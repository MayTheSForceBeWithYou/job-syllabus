// Package queue implements the SQS producer/consumer boundary between
// cmd/ingest and cmd/worker (docs/design.md §5 architecture diagram,
// §9): ingest enqueues a reference to each newly-upserted posting that
// still needs extraction; worker long-polls extract-queue and processes
// them. SQS is at-least-once delivery — a message can be redelivered
// (visibility timeout expiry, worker crash mid-processing) — which is
// exactly why internal/store's write-time counters (docs/design.md §4)
// are built to tolerate the same posting being processed twice.
package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// ExtractMessage is the body of every message on extract-queue: just
// enough for the worker to load the posting's metadata (DynamoDB) and
// body (S3, via rawstore) itself, rather than duplicating either into the
// message.
type ExtractMessage struct {
	PostingID string `json:"postingId"`
	RawS3Key  string `json:"rawS3Key"`
}

// SendExtractMessage enqueues one posting for extraction.
func SendExtractMessage(ctx context.Context, c *sqs.Client, queueURL string, msg ExtractMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal extract message for %s: %w", msg.PostingID, err)
	}
	_, err = c.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("send extract message for %s: %w", msg.PostingID, err)
	}
	return nil
}

// ReceivedMessage pairs a decoded ExtractMessage with the SQS receipt
// handle needed to delete it after successful processing.
type ReceivedMessage struct {
	ExtractMessage
	ReceiptHandle string
}

// receiveVisibilityTimeout matches modules/queues' extract-queue
// visibility timeout (300s — long enough for a Bedrock-fallback call in
// Stage 4, docs/design.md §6, even though Stage 4 itself is Phase 5).
// Keeping the two in sync matters: too short and a normal-length
// extraction gets redelivered to a second worker mid-processing.
const receiveVisibilityTimeout = 300

// ReceiveExtractMessages long-polls extract-queue for up to maxMessages
// (SQS's own cap is 10 per call), waiting up to 20s (SQS's own max) for at
// least one to arrive rather than busy-polling an empty queue.
func ReceiveExtractMessages(ctx context.Context, c *sqs.Client, queueURL string, maxMessages int32) ([]ReceivedMessage, error) {
	out, err := c.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: maxMessages,
		WaitTimeSeconds:     20,
		VisibilityTimeout:   receiveVisibilityTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}

	msgs := make([]ReceivedMessage, 0, len(out.Messages))
	for _, m := range out.Messages {
		var em ExtractMessage
		if err := json.Unmarshal([]byte(aws.ToString(m.Body)), &em); err != nil {
			// Not left for a retry to fix — a redelivery would decode
			// exactly the same way and fail again. Leaving it undeleted
			// means it eventually exhausts maxReceiveCount and lands in
			// the DLQ via redrive, which is the correct outcome for a
			// message that's genuinely malformed, not just unlucky.
			continue
		}
		msgs = append(msgs, ReceivedMessage{ExtractMessage: em, ReceiptHandle: aws.ToString(m.ReceiptHandle)})
	}
	return msgs, nil
}

// DeleteMessage acknowledges successful processing. Only call this after
// the extraction it corresponds to has actually been persisted — deleting
// first and processing second would silently drop work on a crash.
func DeleteMessage(ctx context.Context, c *sqs.Client, queueURL, receiptHandle string) error {
	_, err := c.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}

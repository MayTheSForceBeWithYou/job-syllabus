# Two queues per docs/design.md §3: ingest-queue (manual URL submissions,
# §5 Tier 3) and extract-queue (Stage 1-5 pipeline work, §6). Both follow
# the identical main-queue + DLQ + redrive pattern.
locals {
  queues = {
    ingest  = { visibility_timeout = 60, retention_days = 4 }
    extract = { visibility_timeout = 300, retention_days = 4 } # longer: Bedrock calls in Stage 4
  }
}

resource "aws_sqs_queue" "dlq" {
  for_each = local.queues

  name                      = "${var.project}-${each.key}-dlq"
  message_retention_seconds = 14 * 24 * 60 * 60 # 14 days - DLQ depth > 0 is the alarm that matters (§9); give time to investigate
}

resource "aws_sqs_queue" "main" {
  for_each = local.queues

  name                       = "${var.project}-${each.key}-queue"
  visibility_timeout_seconds = each.value.visibility_timeout
  message_retention_seconds  = each.value.retention_days * 24 * 60 * 60
  receive_wait_time_seconds  = 10 # long polling - fewer empty ReceiveMessage calls, cheaper

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq[each.key].arn
    maxReceiveCount     = var.max_receive_count
  })
}

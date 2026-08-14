resource "aws_sns_topic" "alerts" {
  name = "${var.project}-alerts"
}

resource "aws_sns_topic_subscription" "alerts_email" {
  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = var.alert_email
}

# "DLQ depth > 0 is the one alarm that will actually save you. Everything
# else is nice to have." — docs/design.md §9. The other alarms it lists
# (ingest task failure, API 5xx > 1%, Bedrock throttling) need resources
# that don't exist until Phase 3+/4+/5 (service-api, scheduled ingest,
# Bedrock) — added when those land, not stubbed out now. Monthly spend is
# already covered by the bootstrap stack's $60 AWS Budget, not duplicated
# here as a separate CloudWatch billing alarm.
resource "aws_cloudwatch_metric_alarm" "dlq_depth" {
  for_each = var.dlq_names

  alarm_name          = "${var.project}-${each.key}-dlq-depth"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 300
  statistic           = "Maximum"
  threshold           = 0
  alarm_description   = "Messages landed in the ${each.key} DLQ - something is failing repeatedly."
  treat_missing_data  = "notBreaching"

  dimensions = {
    QueueName = each.value
  }

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]
}

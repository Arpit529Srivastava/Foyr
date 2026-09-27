variable "alert_email" {
  description = "Where alarm emails go (set in terraform.tfvars, which is git-ignored)."
  type        = string
}

resource "aws_sns_topic" "alerts" {
  name = "foyr-alerts"
}

resource "aws_sns_topic_subscription" "alerts_email" {
  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = var.alert_email
}

# fck-nat has had no healthy machine for 10 minutes.
resource "aws_cloudwatch_metric_alarm" "nat_down" {
  alarm_name          = "foyr-nat-down"
  namespace           = "AWS/AutoScaling"
  metric_name         = "GroupInServiceInstances"
  dimensions          = { AutoScalingGroupName = local.nat_name }
  statistic           = "Minimum"
  period              = 300
  evaluation_periods  = 2
  comparison_operator = "LessThanThreshold"
  threshold           = 1
  treat_missing_data  = "breaching"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
}

# Five or more failed builds in an hour usually means our side is broken.
resource "aws_cloudwatch_metric_alarm" "builds_failing" {
  alarm_name          = "foyr-builds-failing"
  namespace           = "AWS/CodeBuild"
  metric_name         = "FailedBuilds"
  dimensions          = { ProjectName = aws_codebuild_project.build.name }
  statistic           = "Sum"
  period              = 3600
  evaluation_periods  = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 5
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alerts.arn]
}

resource "aws_cloudwatch_log_group" "builds" {
  name              = "/foyr/builds"
  retention_in_days = 14
}

resource "aws_cloudwatch_log_group" "platform" {
  name              = "/foyr/platform"
  retention_in_days = 30
}

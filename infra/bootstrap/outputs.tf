output "bucket" {
  description = "Name of the state bucket, used in the backend block."
  value       = aws_s3_bucket.state.bucket
}

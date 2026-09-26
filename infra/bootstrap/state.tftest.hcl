mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "123456789012"
    }
  }
}

run "bucket_is_private_versioned_and_encrypted" {
  command = plan

  assert {
    condition     = aws_s3_bucket.state.bucket == "foyr-tfstate-123456789012"
    error_message = "bucket name must include the account id"
  }

  assert {
    condition     = aws_s3_bucket_versioning.state.versioning_configuration[0].status == "Enabled"
    error_message = "versioning must be enabled"
  }

  assert {
    condition = alltrue([
      aws_s3_bucket_public_access_block.state.block_public_acls,
      aws_s3_bucket_public_access_block.state.block_public_policy,
      aws_s3_bucket_public_access_block.state.ignore_public_acls,
      aws_s3_bucket_public_access_block.state.restrict_public_buckets,
    ])
    error_message = "all public access must be blocked"
  }

  assert {
    condition     = alltrue([for r in aws_s3_bucket_server_side_encryption_configuration.state.rule : alltrue([for d in r.apply_server_side_encryption_by_default : d.sse_algorithm == "AES256"])])
    error_message = "encryption must be on"
  }
}

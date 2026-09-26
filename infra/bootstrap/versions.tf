terraform {
  required_version = ">= 1.8"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      project    = "foyr"
      managed_by = "opentofu"
    }
  }
}

# State for this folder lives in the bucket it creates (moved there after the first apply).
terraform {
  backend "s3" {
    bucket       = "foyr-tfstate-899628690655"
    key          = "bootstrap/terraform.tfstate"
    region       = "ap-south-1"
    use_lockfile = true
    encrypt      = true
  }
}

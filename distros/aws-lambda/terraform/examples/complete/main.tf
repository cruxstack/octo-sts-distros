terraform {
  required_version = ">= 1.3"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# -----------------------------------------------------------------------------
# Example: Complete Octo-STS deployment on AWS Lambda
# -----------------------------------------------------------------------------

module "octo_sts" {
  source = "../../"

  name        = "octo-sts"
  environment = var.environment

  # GitHub App configuration
  # These can be direct values or SSM ARNs for runtime resolution
  github_app_config = {
    app_id      = var.github_app_id
    private_key = var.github_app_private_key
    # webhook_secret is auto-generated if not provided
  }

  # STS configuration
  # If domain is empty, it will use the API Gateway endpoint hostname
  sts_config = {
    domain = var.sts_domain
  }

  # Optional: Filter webhooks to specific organizations
  webhook_config = {
    organization_filter = var.github_organization_filter
  }

  # Lambda configuration (optional - uses sensible defaults)
  lambda_config = {
    memory_size  = 256
    timeout      = 30
    architecture = "arm64"
  }

  # Log retention
  lambda_log_retention_days = 30

  # SSM parameter access (required if using SSM ARNs for secrets)
  ssm_parameter_arns = var.ssm_parameter_arns

  # Additional environment variables (optional)
  lambda_environment_variables = var.additional_env_vars
}

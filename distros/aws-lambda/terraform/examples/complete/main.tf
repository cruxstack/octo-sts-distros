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

locals {
  ssm_prefix = "/octo-sts/${var.environment}/"
}

# -----------------------------------------------------------------------------
# Example: Complete Octo-STS deployment on AWS Lambda
# -----------------------------------------------------------------------------

module "octo_sts" {
  source = "../../"

  name        = "octo-sts"
  environment = var.environment

  # GitHub App configuration
  # When using the installer, these point to SSM parameters that will be created
  # by the setup wizard. When not using the installer, provide direct values or
  # existing SSM ARNs.
  github_app_config = {
    app_id         = var.installer_enabled ? "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${local.ssm_prefix}GITHUB_APP_ID" : var.github_app_id
    private_key    = var.installer_enabled ? "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${local.ssm_prefix}GITHUB_APP_PRIVATE_KEY" : var.github_app_private_key
    webhook_secret = var.installer_enabled ? "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${local.ssm_prefix}GITHUB_WEBHOOK_SECRET" : ""
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

  # Setup wizard configuration
  # When enabled, visit the setup_url output to create your GitHub App
  installer_config = {
    enabled              = var.installer_enabled
    ssm_parameter_prefix = local.ssm_prefix
    github_org           = var.github_org
  }

  # Lambda configuration (optional - uses sensible defaults)
  lambda_config = {
    memory_size  = 256
    timeout      = 30
    architecture = "arm64"
  }

  # Log retention
  lambda_log_retention_days = 30

  # SSM parameter access (required for SSM ARNs)
  ssm_parameter_arns = var.installer_enabled ? [
    "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${local.ssm_prefix}*"
  ] : var.ssm_parameter_arns

  # Additional environment variables (optional)
  lambda_environment_variables = var.additional_env_vars
}

data "aws_caller_identity" "current" {}

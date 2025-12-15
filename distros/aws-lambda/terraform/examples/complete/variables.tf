variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (e.g., prod, staging, dev)"
  type        = string
  default     = "prod"
}

variable "installer_enabled" {
  description = "Enable the setup wizard to create GitHub App via web UI. When true, github_app_id and github_app_private_key are ignored."
  type        = bool
  default     = false
}

variable "github_org" {
  description = "GitHub organization to create the GitHub App under. Leave empty for personal account."
  type        = string
  default     = ""
}

variable "github_app_id" {
  description = "GitHub App ID. Can be a direct value or SSM ARN. Ignored when installer_enabled=true."
  type        = string
  default     = ""
}

variable "github_app_private_key" {
  description = "GitHub App private key (PEM format). Can be a direct value or SSM ARN. Ignored when installer_enabled=true."
  type        = string
  sensitive   = true
  default     = ""
}

variable "sts_domain" {
  description = "Custom domain for STS audience validation. Leave empty to use API Gateway endpoint."
  type        = string
  default     = ""
}

variable "github_organization_filter" {
  description = "Comma-separated list of GitHub organizations to process webhooks for. Empty means all."
  type        = string
  default     = ""
}

variable "ssm_parameter_arns" {
  description = "List of SSM Parameter ARNs the Lambda functions need access to. Ignored when installer_enabled=true."
  type        = list(string)
  default     = []
}

variable "additional_env_vars" {
  description = "Additional environment variables for Lambda functions."
  type        = map(string)
  default     = {}
}

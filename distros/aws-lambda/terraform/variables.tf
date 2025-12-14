# ================================================================== general ===

variable "app_version" {
  description = "Version of Octo-STS to deploy. Use 'latest' for main branch or a specific version tag."
  type        = string
  default     = "latest"
}

variable "app_repo" {
  description = "Git repository URL for the Octo-STS application."
  type        = string
  default     = "https://github.com/octo-sts/app.git"
}

variable "distro_repo" {
  description = "Git repository URL for the Octo-STS distro (contains Lambda wrappers)."
  type        = string
  default     = "https://github.com/cruxstack/octo-sts-distros.git"
}

variable "app_force_rebuild_id" {
  description = "ID to force rebuilding Lambda artifacts. Change this value to trigger a rebuild."
  type        = string
  default     = ""
}

# ------------------------------------------------------------------- lambda ---

variable "lambda_config" {
  description = "Configuration for the Lambda functions."
  type = object({
    memory_size                    = optional(number, 256)
    timeout                        = optional(number, 30)
    runtime                        = optional(string, "provided.al2023")
    architecture                   = optional(string, "arm64")
    reserved_concurrent_executions = optional(number, -1)
  })
  default = {}

  validation {
    condition     = var.lambda_config.memory_size >= 128 && var.lambda_config.memory_size <= 10240
    error_message = "Lambda memory_size must be between 128 and 10240 MB."
  }

  validation {
    condition     = var.lambda_config.timeout >= 1 && var.lambda_config.timeout <= 900
    error_message = "Lambda timeout must be between 1 and 900 seconds."
  }

  validation {
    condition     = contains(["arm64", "x86_64"], var.lambda_config.architecture)
    error_message = "Lambda architecture must be 'arm64' or 'x86_64'."
  }
}

variable "lambda_log_retention_days" {
  description = "Number of days to retain Lambda function logs in CloudWatch Logs."
  type        = number
  default     = 30

  validation {
    condition     = contains([0, 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, 3653], var.lambda_log_retention_days)
    error_message = "Lambda log retention days must be a valid CloudWatch Logs retention period."
  }
}

variable "lambda_environment_variables" {
  description = "Additional environment variables for Lambda functions. Values can be SSM ARNs for automatic resolution at runtime."
  type        = map(string)
  default     = {}
}

# ------------------------------------------------------------------- github ---

variable "github_app_config" {
  description = "GitHub App configuration. Private key and webhook secret values can be SSM ARNs for automatic resolution at runtime."
  sensitive   = true
  type = object({
    app_id         = string
    private_key    = string
    webhook_secret = optional(string, "")
  })

  validation {
    condition     = var.github_app_config.app_id != ""
    error_message = "GitHub App ID is required."
  }

  validation {
    condition     = var.github_app_config.private_key != ""
    error_message = "GitHub App private key is required."
  }
}

# ---------------------------------------------------------------------- sts ---

variable "sts_config" {
  description = "STS service configuration."
  type = object({
    domain = optional(string, "") # Custom domain for audience validation. If empty, uses API Gateway endpoint hostname.
  })
  default = {}
}

# ------------------------------------------------------------------ webhook ---

variable "webhook_config" {
  description = "Webhook service configuration."
  type = object({
    organization_filter = optional(string, "") # Comma-separated list of organizations to process webhooks for. Empty means all.
  })
  default = {}
}

# -------------------------------------------------------------- api gateway ---

variable "api_gateway_config" {
  description = "Configuration for the API Gateway HTTP API."
  type = object({
    enabled    = optional(bool, true)
    stage_name = optional(string, "$default")
  })
  default = {}
}

variable "api_gateway_cors_config" {
  description = "CORS configuration for API Gateway. Set allow_origins to restrict cross-origin access."
  type = object({
    allow_origins = optional(list(string), ["*"])
    allow_methods = optional(list(string), ["POST", "GET", "OPTIONS"])
    allow_headers = optional(list(string), ["Content-Type", "Authorization", "X-Hub-Signature-256", "X-GitHub-Event", "X-GitHub-Delivery"])
    max_age       = optional(number, 300)
  })
  default = {}
}

variable "kms_key_arn" {
  description = "KMS key ARN for encrypting CloudWatch Logs. If not provided, AWS managed keys are used."
  type        = string
  default     = null
}

# --------------------------------------------------------------------- ssm ---

variable "ssm_parameter_arns" {
  description = "List of SSM Parameter Store ARNs that Lambda functions can access for secrets resolution at runtime."
  type        = list(string)
  default     = []
}

output "api_gateway_endpoint" {
  description = "Base URL of the API Gateway"
  value       = module.octo_sts.api_gateway_endpoint
}

output "webhook_url" {
  description = "Webhook URL to configure in GitHub App settings"
  value       = module.octo_sts.webhook_url
}

output "sts_url" {
  description = "STS token exchange URL"
  value       = module.octo_sts.sts_url
}

output "sts_domain" {
  description = "STS domain used for audience validation"
  value       = module.octo_sts.sts_domain
}

output "webhook_secret" {
  description = "Webhook secret to configure in GitHub App (generated if not provided)"
  value       = module.octo_sts.webhook_secret
  sensitive   = true
}

output "lambda_sts_function_name" {
  description = "Name of the STS Lambda function"
  value       = module.octo_sts.lambda_sts_function_name
}

output "lambda_webhook_function_name" {
  description = "Name of the Webhook Lambda function"
  value       = module.octo_sts.lambda_webhook_function_name
}

output "lambda_role_arn" {
  description = "ARN of the IAM role used by Lambda functions"
  value       = module.octo_sts.lambda_role_arn
}

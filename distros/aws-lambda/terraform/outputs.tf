# ================================================================== lambda ===

output "lambda_sts_function_arn" {
  description = "ARN of the STS Lambda function"
  value       = try(aws_lambda_function.sts[0].arn, null)
}

output "lambda_sts_function_name" {
  description = "Name of the STS Lambda function"
  value       = try(aws_lambda_function.sts[0].function_name, null)
}

output "lambda_sts_function_qualified_arn" {
  description = "Qualified ARN of the STS Lambda function"
  value       = try(aws_lambda_function.sts[0].qualified_arn, null)
}

output "lambda_sts_function_invoke_arn" {
  description = "Invoke ARN of the STS Lambda function"
  value       = try(aws_lambda_function.sts[0].invoke_arn, null)
}

output "lambda_webhook_function_arn" {
  description = "ARN of the Webhook Lambda function"
  value       = try(aws_lambda_function.webhook[0].arn, null)
}

output "lambda_webhook_function_name" {
  description = "Name of the Webhook Lambda function"
  value       = try(aws_lambda_function.webhook[0].function_name, null)
}

output "lambda_webhook_function_qualified_arn" {
  description = "Qualified ARN of the Webhook Lambda function"
  value       = try(aws_lambda_function.webhook[0].qualified_arn, null)
}

output "lambda_webhook_function_invoke_arn" {
  description = "Invoke ARN of the Webhook Lambda function"
  value       = try(aws_lambda_function.webhook[0].invoke_arn, null)
}

# --------------------------------------------------------------------- iam ---

output "lambda_role_arn" {
  description = "ARN of the IAM role used by Lambda functions"
  value       = try(aws_iam_role.lambda[0].arn, null)
}

output "lambda_role_name" {
  description = "Name of the IAM role used by Lambda functions"
  value       = try(aws_iam_role.lambda[0].name, null)
}

# --------------------------------------------------------------- cloudwatch ---

output "cloudwatch_log_group_sts_name" {
  description = "Name of the CloudWatch Log Group for STS Lambda"
  value       = try(aws_cloudwatch_log_group.sts[0].name, null)
}

output "cloudwatch_log_group_sts_arn" {
  description = "ARN of the CloudWatch Log Group for STS Lambda"
  value       = try(aws_cloudwatch_log_group.sts[0].arn, null)
}

output "cloudwatch_log_group_webhook_name" {
  description = "Name of the CloudWatch Log Group for Webhook Lambda"
  value       = try(aws_cloudwatch_log_group.webhook[0].name, null)
}

output "cloudwatch_log_group_webhook_arn" {
  description = "ARN of the CloudWatch Log Group for Webhook Lambda"
  value       = try(aws_cloudwatch_log_group.webhook[0].arn, null)
}

# -------------------------------------------------------------- api gateway ---

output "api_gateway_id" {
  description = "ID of the API Gateway HTTP API"
  value       = try(aws_apigatewayv2_api.this[0].id, null)
}

output "api_gateway_arn" {
  description = "ARN of the API Gateway HTTP API"
  value       = try(aws_apigatewayv2_api.this[0].arn, null)
}

output "api_gateway_endpoint" {
  description = "Base URL of the API Gateway"
  value       = try(aws_apigatewayv2_api.this[0].api_endpoint, null)
}

output "api_gateway_execution_arn" {
  description = "Execution ARN of the API Gateway HTTP API"
  value       = try(aws_apigatewayv2_api.this[0].execution_arn, null)
}

output "sts_url" {
  description = "Full URL for STS token exchange endpoint"
  value       = try("${aws_apigatewayv2_stage.this[0].invoke_url}/sts/exchange", null)
}

output "webhook_url" {
  description = "Full webhook URL to configure in GitHub App settings"
  value       = try("${aws_apigatewayv2_stage.this[0].invoke_url}/webhook", null)
}

output "webhook_secret" {
  description = "Webhook secret to configure in GitHub App (generated if not provided)"
  value       = try(local.github_webhook_secret, null)
  sensitive   = true
}

# ---------------------------------------------------------------- sts domain ---

output "sts_domain" {
  description = "The STS domain used for audience validation"
  value       = local.sts_domain
}

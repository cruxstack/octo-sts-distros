# =================================================================== locals ===

locals {
  enabled = module.this.enabled

  aws_account_id  = data.aws_caller_identity.current.account_id
  aws_region_name = data.aws_region.current.id
  aws_partition   = data.aws_partition.current.partition

  # Use provided webhook secret or generate one
  github_webhook_secret = var.github_app_config.webhook_secret != "" ? var.github_app_config.webhook_secret : random_password.webhook_secret[0].result

  # STS domain: use custom domain if provided, otherwise extract hostname from API Gateway endpoint
  sts_domain = var.sts_config.domain != "" ? var.sts_config.domain : (
    local.enabled && var.api_gateway_config.enabled ?
    replace(aws_apigatewayv2_api.this[0].api_endpoint, "https://", "") : ""
  )

  # Common environment variables for both Lambdas
  lambda_env_common = {
    PORT                           = "8080"
    GITHUB_APP_ID                  = var.github_app_config.app_id
    APP_SECRET_CERTIFICATE_ENV_VAR = var.github_app_config.private_key
    METRICS                        = "false"
  }

  lambda_env_sts = merge(local.lambda_env_common, {
    STS_DOMAIN = local.sts_domain
  }, var.lambda_environment_variables)

  lambda_env_webhook = merge(local.lambda_env_common, {
    GITHUB_WEBHOOK_SECRET              = local.github_webhook_secret
    GITHUB_WEBHOOK_ORGANIZATION_FILTER = var.webhook_config.organization_filter
  }, var.lambda_environment_variables)
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}
data "aws_partition" "current" {}

# ======================================================== webhook secret ===

resource "random_password" "webhook_secret" {
  count = local.enabled && var.github_app_config.webhook_secret == "" ? 1 : 0

  length  = 32
  special = false
}

# ================================================================ artifacts ===

module "lambda_artifact_sts" {
  source = "github.com/cruxstack/terraform-docker-artifact-packager?ref=v1.4.0"
  count  = local.enabled ? 1 : 0

  attributes             = ["lambda", "sts"]
  artifact_src_path      = "/tmp/package.zip"
  artifact_dst_directory = "${path.module}/dist"
  docker_build_context   = abspath("${path.module}/assets/lambda-functions")
  docker_build_target    = "package-sts"
  force_rebuild_id       = var.app_force_rebuild_id

  docker_build_args = {
    APP_VERSION = var.app_version
    APP_REPO    = var.app_repo
    DISTRO_REPO = var.distro_repo
  }

  context = module.this.context
}

module "lambda_artifact_webhook" {
  source = "github.com/cruxstack/terraform-docker-artifact-packager?ref=v1.4.0"
  count  = local.enabled ? 1 : 0

  attributes             = ["lambda", "webhook"]
  artifact_src_path      = "/tmp/package.zip"
  artifact_dst_directory = "${path.module}/dist"
  docker_build_context   = abspath("${path.module}/assets/lambda-functions")
  docker_build_target    = "package-webhook"
  force_rebuild_id       = var.app_force_rebuild_id

  docker_build_args = {
    APP_VERSION = var.app_version
    APP_REPO    = var.app_repo
    DISTRO_REPO = var.distro_repo
  }

  context = module.this.context
}

# ================================================================== lambda ===

resource "aws_lambda_function" "sts" {
  count = local.enabled ? 1 : 0

  function_name                  = "${module.this.id}-sts"
  description                    = "Octo-STS - Security Token Service for GitHub App token exchange"
  role                           = aws_iam_role.lambda[0].arn
  handler                        = "bootstrap"
  runtime                        = var.lambda_config.runtime
  memory_size                    = var.lambda_config.memory_size
  timeout                        = var.lambda_config.timeout
  reserved_concurrent_executions = var.lambda_config.reserved_concurrent_executions
  architectures                  = [var.lambda_config.architecture]

  filename         = module.lambda_artifact_sts[0].artifact_package_path
  source_code_hash = filebase64sha256(module.lambda_artifact_sts[0].artifact_package_path)

  environment {
    variables = local.lambda_env_sts
  }

  depends_on = [
    aws_cloudwatch_log_group.sts,
    aws_iam_role_policy.lambda,
  ]

  tags = module.this.tags
}

resource "aws_lambda_function" "webhook" {
  count = local.enabled ? 1 : 0

  function_name                  = "${module.this.id}-webhook"
  description                    = "Octo-STS - Webhook validator for trust policy changes"
  role                           = aws_iam_role.lambda[0].arn
  handler                        = "bootstrap"
  runtime                        = var.lambda_config.runtime
  memory_size                    = var.lambda_config.memory_size
  timeout                        = var.lambda_config.timeout
  reserved_concurrent_executions = var.lambda_config.reserved_concurrent_executions
  architectures                  = [var.lambda_config.architecture]

  filename         = module.lambda_artifact_webhook[0].artifact_package_path
  source_code_hash = filebase64sha256(module.lambda_artifact_webhook[0].artifact_package_path)

  environment {
    variables = local.lambda_env_webhook
  }

  depends_on = [
    aws_cloudwatch_log_group.webhook,
    aws_iam_role_policy.lambda,
  ]

  tags = module.this.tags
}

# ============================================================= cloudwatch ===

resource "aws_cloudwatch_log_group" "sts" {
  count = local.enabled ? 1 : 0

  name              = "/aws/lambda/${module.this.id}-sts"
  retention_in_days = var.lambda_log_retention_days
  tags              = module.this.tags
}

resource "aws_cloudwatch_log_group" "webhook" {
  count = local.enabled ? 1 : 0

  name              = "/aws/lambda/${module.this.id}-webhook"
  retention_in_days = var.lambda_log_retention_days
  tags              = module.this.tags
}

resource "aws_cloudwatch_log_group" "api_gateway" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  name              = "/aws/apigateway/${module.this.id}"
  retention_in_days = var.lambda_log_retention_days
  tags              = module.this.tags
}

# ============================================================= api gateway ===

resource "aws_apigatewayv2_api" "this" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  name          = module.this.id
  protocol_type = "HTTP"
  description   = "API Gateway for Octo-STS - Security Token Service for GitHub"

  cors_configuration {
    allow_origins = ["*"]
    allow_methods = ["POST", "GET", "OPTIONS"]
    allow_headers = ["Content-Type", "Authorization", "X-Hub-Signature-256", "X-GitHub-Event", "X-GitHub-Delivery"]
    max_age       = 300
  }

  tags = module.this.tags
}

resource "aws_apigatewayv2_stage" "this" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id      = aws_apigatewayv2_api.this[0].id
  name        = var.api_gateway_config.stage_name
  auto_deploy = true

  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.api_gateway[0].arn
    format = jsonencode({
      requestId         = "$context.requestId"
      ip                = "$context.identity.sourceIp"
      requestTime       = "$context.requestTime"
      httpMethod        = "$context.httpMethod"
      routeKey          = "$context.routeKey"
      status            = "$context.status"
      protocol          = "$context.protocol"
      responseLength    = "$context.responseLength"
      integrationStatus = "$context.integrationStatus"
    })
  }

  tags = module.this.tags
}

# --------------------------------------------------------- sts integration ---

resource "aws_apigatewayv2_integration" "sts" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id                 = aws_apigatewayv2_api.this[0].id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.sts[0].invoke_arn
  payload_format_version = "2.0"
}

# Route: ANY /sts/{proxy+} - STS service routes
resource "aws_apigatewayv2_route" "sts_proxy" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id    = aws_apigatewayv2_api.this[0].id
  route_key = "ANY /sts/{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.sts[0].id}"
}

# Route: GET / - Root handler (documentation redirect)
resource "aws_apigatewayv2_route" "root" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id    = aws_apigatewayv2_api.this[0].id
  route_key = "GET /"
  target    = "integrations/${aws_apigatewayv2_integration.sts[0].id}"
}

# Route: ANY /{proxy+} - Catch-all for STS (fallback)
resource "aws_apigatewayv2_route" "catch_all" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id    = aws_apigatewayv2_api.this[0].id
  route_key = "ANY /{proxy+}"
  target    = "integrations/${aws_apigatewayv2_integration.sts[0].id}"
}

# ----------------------------------------------------- webhook integration ---

resource "aws_apigatewayv2_integration" "webhook" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id                 = aws_apigatewayv2_api.this[0].id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.webhook[0].invoke_arn
  payload_format_version = "2.0"
}

# Route: ANY /webhook - GitHub webhook endpoint
resource "aws_apigatewayv2_route" "webhook" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  api_id    = aws_apigatewayv2_api.this[0].id
  route_key = "ANY /webhook"
  target    = "integrations/${aws_apigatewayv2_integration.webhook[0].id}"
}

# ---------------------------------------------------- lambda permissions ---

resource "aws_lambda_permission" "sts" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.sts[0].function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.this[0].execution_arn}/*/*"
}

resource "aws_lambda_permission" "webhook" {
  count = local.enabled && var.api_gateway_config.enabled ? 1 : 0

  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.webhook[0].function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.this[0].execution_arn}/*/*"
}

# ---------------------------------------------------------------------- iam ---

resource "aws_iam_role" "lambda" {
  count = local.enabled ? 1 : 0

  name        = module.this.id
  description = "IAM role for Octo-STS Lambda functions"

  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [{
      Effect    = "Allow"
      Principal = { "Service" : "lambda.amazonaws.com" }
      Action    = ["sts:AssumeRole", "sts:TagSession"]
    }]
  })

  tags = module.this.tags
}

data "aws_iam_policy_document" "lambda" {
  count = local.enabled ? 1 : 0

  statement {
    sid    = "CloudWatchLogsAccess"
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = [
      "arn:${local.aws_partition}:logs:${local.aws_region_name}:${local.aws_account_id}:log-group:/aws/lambda/${module.this.id}-*:*"
    ]
  }

  dynamic "statement" {
    for_each = length(var.ssm_parameter_arns) > 0 ? [1] : []

    content {
      sid    = "SSMParameterAccess"
      effect = "Allow"
      actions = [
        "ssm:GetParameter",
        "ssm:GetParameters"
      ]
      resources = var.ssm_parameter_arns
    }
  }
}

resource "aws_iam_role_policy" "lambda" {
  count = local.enabled ? 1 : 0

  name   = module.this.id
  role   = aws_iam_role.lambda[0].id
  policy = data.aws_iam_policy_document.lambda[0].json
}

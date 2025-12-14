# Octo-STS AWS Lambda Distribution

This distribution deploys [Octo-STS](https://github.com/octo-sts/app) to AWS Lambda with API Gateway v2 (HTTP API) for webhook handling and token exchange.

## Directory Structure

```
aws-lambda/
├── terraform/              # Terraform module
│   ├── assets/
│   │   └── lambda-functions/  # Dockerfile for Lambda builds
│   ├── examples/
│   ├── main.tf
│   ├── variables.tf
│   └── outputs.tf
└── README.md

# Lambda code is located in the repository root:
cmd/
├── lambda-sts/             # STS service Lambda entrypoint
└── lambda-webhook/         # Webhook service Lambda entrypoint

internal/
└── ssmresolver/            # SSM ARN resolution utility
```

## Features

- **Serverless Deployment** - Runs on AWS Lambda with API Gateway v2 (HTTP API)
- **Cost Optimized** - Uses ARM64 architecture by default for better price/performance
- **SSM Integration** - Environment variables can reference SSM Parameter Store ARNs for automatic resolution at runtime
- **Separate Functions** - STS and Webhook services run as separate Lambda functions for independent scaling
- **App-Installer Support** - Works with the app-installer tool using `STORAGE_MODE=aws-ssm`

## Architecture

```
                                    +------------------+
                                    |   GitHub App     |
                                    +--------+---------+
                                             |
                                             | Webhooks
                                             v
                                  +----------+---------+
                                  |    API Gateway     |
                                  |    HTTP API (v2)   |
                                  +----------+---------+
                                             |
                     +-----------------------+-----------------------+
                     |                                               |
          /sts/*     |                                               |  /webhook
          /{proxy+}  |                                               |
                     v                                               v
          +----------+---------+                          +----------+---------+
          |   Lambda (STS)     |                          | Lambda (Webhook)   |
          |   Token Exchange   |                          | Policy Validation  |
          +----------+---------+                          +----------+---------+
                     |                                               |
                     +-------------------+---------------------------+
                                         |
                                         v
                                  +------+------+
                                  |  GitHub API |
                                  +-------------+
```

## Usage

### Basic Usage

```hcl
module "octo_sts" {
  source = "github.com/cruxstack/octo-sts-distros//distros/aws-lambda/terraform?ref=v1.0.0"

  name = "octo-sts"

  github_app_config = {
    app_id      = "123456"
    private_key = var.github_app_private_key
    # webhook_secret is auto-generated if not provided
  }

  sts_config = {
    domain = ""  # Empty = use API Gateway endpoint hostname
  }
}
```

### With SSM Parameter Store

Store secrets in SSM Parameter Store and reference them by ARN:

```hcl
module "octo_sts" {
  source = "github.com/cruxstack/octo-sts-distros//distros/aws-lambda/terraform?ref=v1.0.0"

  name = "octo-sts"

  github_app_config = {
    app_id      = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/GITHUB_APP_ID"
    private_key = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/APP_SECRET_CERTIFICATE_ENV_VAR"
  }

  # Grant Lambda access to SSM parameters
  ssm_parameter_arns = [
    "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/*"
  ]
}
```

### With App-Installer

Use the app-installer to create your GitHub App and store credentials in SSM:

```bash
# 1. Configure app-installer for AWS SSM storage
export STORAGE_MODE=aws-ssm
export AWS_SSM_PARAMETER_PREFIX=/octo-sts/prod/
export INSTALLER_WEBHOOK_URL=https://your-api-gateway-url/v1/webhook

# 2. Run app-installer to create GitHub App
./app-installer

# 3. Deploy with Terraform referencing SSM parameters
```

```hcl
module "octo_sts" {
  source = "github.com/cruxstack/octo-sts-distros//distros/aws-lambda/terraform?ref=v1.0.0"

  name = "octo-sts"

  github_app_config = {
    app_id         = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/GITHUB_APP_ID"
    private_key    = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/APP_SECRET_CERTIFICATE_ENV_VAR"
    webhook_secret = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/GITHUB_WEBHOOK_SECRET"
  }

  ssm_parameter_arns = [
    "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/*"
  ]
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `name` | Name for the resources | `string` | n/a | yes |
| `github_app_config` | GitHub App configuration | `object` | n/a | yes |
| `sts_config` | STS service configuration | `object` | `{}` | no |
| `webhook_config` | Webhook service configuration | `object` | `{}` | no |
| `lambda_config` | Lambda function configuration | `object` | `{}` | no |
| `lambda_log_retention_days` | CloudWatch log retention | `number` | `30` | no |
| `lambda_environment_variables` | Additional environment variables | `map(string)` | `{}` | no |
| `api_gateway_config` | API Gateway configuration | `object` | `{}` | no |
| `ssm_parameter_arns` | SSM Parameter ARNs for Lambda access | `list(string)` | `[]` | no |
| `bot_version` | Octo-STS version to deploy | `string` | `"latest"` | no |
| `bot_repo` | Octo-STS repository URL | `string` | `"https://github.com/octo-sts/app.git"` | no |
| `distro_repo` | Distros repository URL | `string` | `"https://github.com/cruxstack/octo-sts-distros.git"` | no |

### GitHub App Config

```hcl
github_app_config = {
  app_id         = string  # GitHub App ID (required) - can be SSM ARN
  private_key    = string  # GitHub App private key PEM (required) - can be SSM ARN
  webhook_secret = string  # Webhook secret (optional) - auto-generated if not provided
}
```

### STS Config

```hcl
sts_config = {
  domain = string  # Custom domain for audience validation (optional)
                   # If empty, uses API Gateway endpoint hostname
}
```

### Webhook Config

```hcl
webhook_config = {
  organization_filter = string  # Comma-separated list of orgs to process (optional)
                                # Empty means process all organizations
}
```

### Lambda Config

```hcl
lambda_config = {
  memory_size                    = number  # Memory in MB (default: 256)
  timeout                        = number  # Timeout in seconds (default: 30)
  runtime                        = string  # Lambda runtime (default: "provided.al2023")
  architecture                   = string  # CPU architecture (default: "arm64")
  reserved_concurrent_executions = number  # Reserved concurrency (default: -1)
}
```

### API Gateway Config

```hcl
api_gateway_config = {
  enabled    = bool    # Enable API Gateway (default: true)
  stage_name = string  # Stage name (default: "v1")
}
```

## Outputs

| Name | Description |
|------|-------------|
| `api_gateway_endpoint` | Base URL of the API Gateway |
| `webhook_url` | Full webhook URL to configure in GitHub App settings |
| `sts_url` | Full URL for STS token exchange endpoint |
| `sts_domain` | The STS domain used for audience validation |
| `webhook_secret` | Webhook secret (generated if not provided) |
| `lambda_sts_function_arn` | ARN of the STS Lambda function |
| `lambda_sts_function_name` | Name of the STS Lambda function |
| `lambda_webhook_function_arn` | ARN of the Webhook Lambda function |
| `lambda_webhook_function_name` | Name of the Webhook Lambda function |
| `lambda_role_arn` | ARN of the IAM role used by Lambda functions |
| `lambda_role_name` | Name of the IAM role used by Lambda functions |

## API Endpoints

| Route | Method | Lambda | Description |
|-------|--------|--------|-------------|
| `/sts/{proxy+}` | ANY | STS | Token exchange service routes |
| `/webhook` | ANY | Webhook | GitHub webhook endpoint |
| `/{proxy+}` | ANY | STS | Catch-all fallback to STS |
| `/` | GET | STS | Root handler (documentation) |

## SSM ARN Resolution

Environment variables that contain SSM Parameter Store ARNs are automatically resolved at Lambda cold start. This allows you to:

1. Store secrets securely in SSM Parameter Store
2. Reference them by ARN in Terraform
3. Lambda automatically fetches the actual values at runtime

ARN format: `arn:aws:ssm:<region>:<account>:parameter/<path>`

Example:
```hcl
github_app_config = {
  app_id      = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/GITHUB_APP_ID"
  private_key = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/APP_SECRET_CERTIFICATE_ENV_VAR"
}
```

## Requirements

| Name | Version |
|------|---------|
| terraform | >= 1.3 |
| aws | >= 5.0 |

## Building Locally

The Lambda functions are built using Docker during Terraform apply. The build process:

1. Clones the Octo-STS app and distros repositories
2. Builds the Lambda wrapper binaries for ARM64
3. Packages them as ZIP files for Lambda deployment

To force a rebuild, change the `bot_force_rebuild_id` variable.

## License

MIT Licensed. See [LICENSE](../../LICENSE) for full details.

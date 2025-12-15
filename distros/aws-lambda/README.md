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
- **Setup Wizard** - Built-in web UI to create and configure your GitHub App automatically
- **Health Checks** - `/healthz` endpoint for load balancer and monitoring integration

## Architecture

```
                                    +------------------+
                                    |   GitHub App     |
                                    +--------+---------+
                                             |
                                             | Webhooks / OAuth
                                             v
                                  +----------+---------+
                                  |    API Gateway     |
                                  |    HTTP API (v2)   |
                                  +----------+---------+
                                             |
        +------------------------------------+------------------------------------+
        |                                    |                                    |
        |  /sts/*                            |  /webhook                          |  /setup/*
        |  /{proxy+}                         |  /healthz                          |  /callback
        |                                    |  /                                 |
        v                                    v                                    |
+-------+--------+                  +--------+--------+                           |
| Lambda (STS)   |                  | Lambda (Webhook)|<--------------------------+
| Token Exchange |                  | + Installer     |
+-------+--------+                  +--------+--------+
        |                                    |
        +----------------+-------------------+
                         |
                         v
                  +------+------+
                  |  GitHub API |
                  +-------------+
                         |
                         v
               +---------+---------+
               | SSM Parameter     |
               | Store (secrets)   |
               +-------------------+
```

## Usage

### Quick Start with Setup Wizard (Recommended)

The easiest way to get started is using the built-in setup wizard. This deploys the
infrastructure first, then guides you through creating the GitHub App via a web UI.

```hcl
module "octo_sts" {
  source = "github.com/cruxstack/octo-sts-distros//distros/aws-lambda/terraform?ref=v1.0.0"

  name = "octo-sts"

  # Point to SSM parameters (will be created by the setup wizard)
  github_app_config = {
    app_id         = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/GITHUB_APP_ID"
    private_key    = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/APP_SECRET_CERTIFICATE_ENV_VAR"
    webhook_secret = "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/GITHUB_WEBHOOK_SECRET"
  }

  # Enable the setup wizard
  installer_config = {
    enabled              = true
    ssm_parameter_prefix = "/octo-sts/prod/"
  }

  # Grant Lambda access to SSM parameters
  ssm_parameter_arns = [
    "arn:aws:ssm:us-east-1:123456789:parameter/octo-sts/prod/*"
  ]
}

output "setup_url" {
  value = module.octo_sts.setup_url
}
```

After `terraform apply`:

1. Open the `setup_url` output in your browser
2. Follow the wizard to create your GitHub App
3. Install the GitHub App on your organization(s)
4. Optionally disable the installer by setting `installer_config.enabled = false` and redeploying

### Basic Usage (Pre-existing GitHub App)

If you already have a GitHub App configured:

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

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| `name` | Name for the resources | `string` | n/a | yes |
| `github_app_config` | GitHub App configuration | `object` | n/a | yes |
| `sts_config` | STS service configuration | `object` | `{}` | no |
| `webhook_config` | Webhook service configuration | `object` | `{}` | no |
| `installer_config` | Setup wizard configuration | `object` | `{}` | no |
| `lambda_config` | Lambda function configuration | `object` | `{}` | no |
| `lambda_log_retention_days` | CloudWatch log retention | `number` | `30` | no |
| `lambda_environment_variables` | Additional environment variables | `map(string)` | `{}` | no |
| `api_gateway_config` | API Gateway configuration | `object` | `{}` | no |
| `ssm_parameter_arns` | SSM Parameter ARNs for Lambda access | `list(string)` | `[]` | no |
| `distro_repo` | Distros repository URL | `string` | `"https://github.com/cruxstack/octo-sts-distros.git"` | no |
| `distro_version` | Distros version to deploy | `string` | `"latest"` | no |
| `force_rebuild_id` | Change this value to force rebuilding Lambda artifacts | `string` | `""` | no |
| `api_gateway_cors_config` | CORS configuration for API Gateway | `object` | `{}` | no |
| `kms_key_arn` | KMS key ARN for CloudWatch Logs encryption | `string` | `null` | no |

### GitHub App Config

```hcl
github_app_config = {
  app_id         = string  # GitHub App ID - can be SSM ARN (required when installer disabled)
  private_key    = string  # GitHub App private key PEM - can be SSM ARN (required when installer disabled)
  webhook_secret = string  # Webhook secret (optional) - auto-generated if not provided
}
```

When `installer_config.enabled = true`, these values can be omitted as credentials will be created via the setup wizard and stored in SSM.

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

### Installer Config

```hcl
installer_config = {
  enabled              = bool    # Enable the setup wizard (default: false)
  ssm_parameter_prefix = string  # SSM parameter prefix, e.g., "/octo-sts/prod/" (required when enabled)
  kms_key_id           = string  # KMS key for SSM parameter encryption (optional)
  github_url           = string  # GitHub URL for GHES (default: "https://github.com")
  github_org           = string  # Organization to create the app under (optional)
}
```

When the installer is enabled:
- `/setup` serves the setup wizard UI
- `/callback` handles GitHub OAuth redirects after app creation
- `/` redirects to `/setup` until the GitHub App is configured
- Credentials are automatically saved to SSM Parameter Store

**Disabling the installer:** After setup is complete, you can disable the installer in two ways:

1. **Via the UI** - Click "Disable Installer" on the success page. This sets `INSTALLER_ENABLED=false`
   in SSM, which immediately hides the setup UI. The API Gateway routes remain but return 404.
   This does not cause Terraform drift since routes are still managed by Terraform.

2. **Via Terraform** - Set `installer_config.enabled = false` and redeploy. This removes the
   installer routes from API Gateway and the SSM write IAM permissions.

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
  stage_name = string  # Stage name (default: "$default")
}
```

### API Gateway CORS Config

```hcl
api_gateway_cors_config = {
  allow_origins = list(string)  # Allowed origins (default: ["*"])
  allow_methods = list(string)  # Allowed HTTP methods (default: ["POST", "GET", "OPTIONS"])
  allow_headers = list(string)  # Allowed headers (default: ["Content-Type", "Authorization", "X-Hub-Signature-256", "X-GitHub-Event", "X-GitHub-Delivery"])
  max_age       = number        # Preflight cache max age in seconds (default: 300)
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
| `setup_url` | URL for the setup wizard (when installer is enabled) |
| `healthz_url` | URL for health check endpoint |
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
| `/healthz` | GET | Webhook | Health check endpoint |
| `/setup` | GET | Webhook | Setup wizard UI (when installer enabled) |
| `/setup/{proxy+}` | ANY | Webhook | Setup wizard sub-routes (when installer enabled) |
| `/callback` | GET | Webhook | GitHub OAuth callback (when installer enabled) |
| `/` | GET | Webhook | Root handler (redirects to /setup or returns 404) |
| `/{proxy+}` | ANY | STS | Catch-all fallback to STS |

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

### SSM Parameters Created by Setup Wizard

When using the setup wizard (`installer_config.enabled = true`), the following SSM
parameters are created automatically under the configured prefix:

| Parameter | Description |
|-----------|-------------|
| `GITHUB_APP_ID` | GitHub App ID |
| `GITHUB_WEBHOOK_SECRET` | Webhook secret for signature validation |
| `GITHUB_CLIENT_ID` | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth client secret |
| `APP_SECRET_CERTIFICATE_ENV_VAR` | GitHub App private key (PEM format) |
| `GITHUB_APP_SLUG` | GitHub App slug (optional) |
| `GITHUB_APP_HTML_URL` | GitHub App URL (optional) |
| `STS_DOMAIN` | STS domain for audience validation (optional) |
| `INSTALLER_ENABLED` | Set to "false" when installer is disabled |

All parameters are stored as `SecureString` type with encryption.

## Requirements

| Name | Version |
|------|---------|
| terraform | >= 1.3 |
| aws | >= 5.0 |

## Building Locally

The Lambda functions are built using Docker during Terraform apply. The build process:

1. Clones the Octo-STS distros repository
2. Fetches the `octo-sts/app` dependency via Go modules
3. Builds the Lambda wrapper binaries for ARM64
4. Packages them as ZIP files for Lambda deployment

To force a rebuild, change the `force_rebuild_id` variable.

## License

MIT Licensed. See [LICENSE](../../LICENSE) for full details.

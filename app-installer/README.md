# App Installer

A web-based tool for creating GitHub Apps via the
[GitHub App Manifest flow](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest).
Pre-configures all permissions required for Octo-STS and saves credentials to
local storage.

For information about the Octo-STS architecture and how GitHub Apps are used,
see the
[upstream architecture documentation](https://github.com/octo-sts/app#architecture).

## Features

- Pre-configured GitHub App manifest with all octo-sts permissions
- Simple web UI for creating the app
- Automatic credential storage after app creation
- Support for GitHub Enterprise Server
- Pluggable storage interface for future backends (AWS SSM, Secrets Manager,
  etc.)

## Usage

```bash
# From the distros/docker directory
docker-compose --profile setup up app-installer

# Visit http://localhost:9000
# 1. Enter your webhook URL (e.g., https://sts.example.com/webhook)
# 2. Click "Create GitHub App"
# 3. Name the app on GitHub and click create
# 4. Credentials are saved to ./secrets/

# Stop the installer
docker-compose --profile setup down
```

## Configuration

| Environment Variable         | Description                                     | Default                |
|------------------------------|-------------------------------------------------|------------------------|
| `PORT`                       | Server port                                     | `8080`                 |
| `REDIRECT_URL`               | Public URL for GitHub OAuth redirect            | `http://localhost:8080`|
| `WEBHOOK_URL`                | Pre-configure webhook URL (optional)            |                        |
| `STORAGE_MODE`               | Storage backend (`envfile`, `files`, `aws-ssm`) | `envfile`              |
| `STORAGE_DIR`                | Directory to save credentials (local modes)     | `./secrets`            |
| `GITHUB_URL`                 | GitHub URL (for GHES)                           | `https://github.com`   |
| `AWS_SSM_PARAMETER_PREFIX`   | SSM parameter prefix (required for `aws-ssm`)   |                        |
| `AWS_SSM_KMS_KEY_ID`         | Custom KMS key ARN (optional)                   | AWS managed key        |
| `AWS_SSM_TAGS`               | JSON object of tags (optional)                  |                        |

## Saved Credentials

After successful app creation, credentials are saved to `STORAGE_DIR`:

| File                | Description                              | Permissions |
|---------------------|------------------------------------------|-------------|
| `app-id`            | GitHub App ID                            | `0644`      |
| `private-key.pem`   | PEM-encoded private key                  | `0600`      |
| `webhook-secret`    | Webhook secret for payload validation    | `0600`      |
| `client-id`         | OAuth client ID                          | `0644`      |
| `client-secret`     | OAuth client secret                      | `0600`      |

## GitHub App Permissions

The manifest includes all permissions required by Octo-STS. For detailed
information about why each permission is needed, see the
[upstream permissions documentation](https://github.com/octo-sts/app#permissions).

**Repository permissions:** actions, administration, checks, contents,
deployments, discussions, environments, issues, packages, pages, pull_requests,
repository_projects, security_events, statuses, workflows

**Organization permissions:** members, organization_administration,
organization_events, organization_projects

## Development

### Running Locally

```bash
cd app-installer
go run cmd/app-installer/main.go
```

Visit http://localhost:8080

### Running Tests

```bash
go test ./...
```

### Storage Backends

The `Store` interface in `pkg/appstore` allows pluggable storage backends.

**Available backends:**
- Local File Store (default) - saves to individual files
- Local Environment File Store - saves to .env file
- AWS SSM Parameter Store - saves to encrypted AWS SSM parameters

**Environment Variables:**

AWS SSM Parameter Store backend can be configured via:

| Environment Variable         | Description                              | Default                |
|------------------------------|------------------------------------------|------------------------|
| `AWS_SSM_PARAMETER_PREFIX`   | SSM parameter prefix/namespace           | Required               |
| `AWS_SSM_KMS_KEY_ID`         | Custom KMS key ARN (optional)            | AWS managed key        |
| `AWS_SSM_TAGS`               | JSON object of tags (optional)           | No tags                |

See `pkg/appstore/` for implementation details and examples.

# octo-sts-distros

Distribution packages and deployment artifacts for
[octo-sts/app](https://github.com/octo-sts/app) - a Security Token Service
(STS) for GitHub that enables OIDC-to-GitHub token federation.

## Overview

Octo-STS allows workloads running anywhere (CI/CD systems, cloud environments,
Kubernetes, etc.) to exchange OIDC tokens for short-lived GitHub access tokens,
eliminating the need for long-lived Personal Access Tokens (PATs).

This repository provides deployment patterns and artifacts for running
octo-sts.

**Note:** For GCP Cloud Run deployment, please refer to the upstream
[octo-sts/app repository](https://github.com/octo-sts/app) which includes
native Cloud Run support.

## Distributions

### Docker (Local Development)

Docker Compose setup for local testing and proof-of-concept deployments.
Includes automated GitHub App installer and ngrok integration.

**Documentation:** [distros/docker/README.md](distros/docker/README.md)

### AWS Lambda

Serverless deployment using API Gateway v2 and Lambda functions.

**Status:** 🚧 Planned

## Components

### App Installer

Web-based tool for creating GitHub Apps via the manifest flow. Automatically
configures all required permissions and saves credentials locally.

**Documentation:** [app-installer/README.md](app-installer/README.md)

## Documentation

- [Architecture Overview](docs/architecture.md) - System design, request flows,
  security model, and API specification
- [Component Breakdown](docs/components.md) - Detailed analysis of binaries,
  packages, and dependencies

## Repository Structure

```
.
├── cmd/                   # Application binaries
├── distros/               # Deployment distributions
│   ├── aws-lambda/        # AWS Lambda deployment
│   └── docker/            # Docker Compose for local development
├── docs/                  # Architecture and component documentation
├── internal/              # Internal packages
└── README.md
```

## Quick Links

- [octo-sts/app](https://github.com/octo-sts/app) - Upstream project
- [Upstream Architecture](https://github.com/octo-sts/app#architecture) - Core
  STS architecture
- [Original Blog Post](https://www.chainguard.dev/unchained/the-end-of-github-pats-you-cant-leak-what-you-dont-have) -
  Background on octo-sts

## Disclaimer

This repository is an independent community project and is not affiliated with,
endorsed by, or associated with [Chainguard](https://www.chainguard.dev/) or
the maintainers of [octo-sts/app](https://github.com/octo-sts/app). All
trademarks belong to their respective owners.

## License

This repository is licensed under the MIT License. See [LICENSE](LICENSE) for
details.

The upstream octo-sts/app project uses the Apache 2.0 License. See
[octo-sts/app LICENSE](https://github.com/octo-sts/app/blob/main/LICENSE).

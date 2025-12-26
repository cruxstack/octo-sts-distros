# octo-sts-distros

Deployment distributions for [octo-sts/app](https://github.com/octo-sts/app) - a
Security Token Service that lets workloads exchange OIDC tokens for short-lived
GitHub access tokens, eliminating long-lived PATs.

**The upstream octo-sts/app works on its own** - this repository adds:

- **Web-based GitHub App installer** - Create your GitHub App via a guided web
  flow that auto-configures permissions and saves credentials to your chosen
  backend
- **Multiple credential storage backends** - Store GitHub App private keys in
  local files, environment variables, or AWS SSM Parameter Store
- **AWS Lambda distribution** - Terraform module for serverless deployment on
  AWS
- **Docker distribution** - Docker Compose setup for local development with
  ngrok

## Distributions

### Docker (Local Development)

Docker Compose setup for local testing and proof-of-concept deployments.
Includes automated GitHub App installer and ngrok integration.

**Documentation:** [distros/docker/README.md](distros/docker/README.md)

### AWS Lambda

Serverless deployment using API Gateway v2 and Lambda functions with Terraform.

**Documentation:**
[distros/aws-lambda/README.md](distros/aws-lambda/README.md)

### GCP Cloud Run

Use [octo-sts/app](https://github.com/octo-sts/app) directly - it has native
Cloud Run support.

## Documentation

- [Architecture Overview](docs/architecture.md) - System design, request flows,
  security model, and API specification
- [Component Breakdown](docs/components.md) - Detailed analysis of binaries,
  packages, and dependencies

## Repository Structure

```
.
├── cmd/                   # Lambda entrypoints and HTTP wrappers
├── distros/               # Deployment distributions
│   ├── aws-lambda/        # AWS Lambda + API Gateway (Terraform)
│   └── docker/            # Docker Compose for local development
└── internal/              # Shared packages (app, sts, configstore)
```

## Quick Links

- [octo-sts/app](https://github.com/octo-sts/app) - Upstream project
- [Trust Policies](https://github.com/octo-sts/app#setting-up-workload-trust) -
  Setup guide and security recommendations
- [Original Blog Post][blog-post] - Background on octo-sts

[blog-post]: https://www.chainguard.dev/unchained/the-end-of-github-pats-you-cant-leak-what-you-dont-have

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

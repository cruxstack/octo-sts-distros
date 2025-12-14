# Octo-STS Architecture

> **Note:** This document describes the architecture of the upstream
> octo-sts/app project. For the most current information, see the
> [upstream documentation](https://github.com/octo-sts/app). For deployment-specific
> information, see the distribution READMEs in the `distros/` directory.

This document provides a comprehensive architecture breakdown of the
octo-sts/app project, a Security Token Service (STS) for GitHub that enables
workloads to exchange OIDC tokens for short-lived GitHub access tokens.

## Overview

Octo-STS is a GitHub App that implements a federated identity model, allowing
workloads running anywhere (CI/CD systems, cloud environments, etc.) to obtain
short-lived GitHub API tokens without requiring long-lived Personal Access
Tokens (PATs). The system validates incoming OIDC tokens against configured
trust policies and issues appropriately-scoped GitHub installation tokens.

## High-Level Architecture

```
+-----------------------------------------------------------------------------+
|                              External Clients                               |
|           (GitHub Actions, Cloud Workloads, CI/CD, Custom OIDC)             |
+-----------------------------------------------------------------------------+
                                      |
                                      | OIDC Token + Exchange Request
                                      v
+-----------------------------------------------------------------------------+
|                              Load Balancer                                  |
|                           (e.g., octo-sts.dev)                              |
+-----------------------------------------------------------------------------+
                                      |
                    +-----------------+-----------------+
                    v                                   v
+-------------------------------+     +---------------------------------------+
|    STS Exchange Service       |     |       Webhook Validator Service       |
|        (cmd/app)              |     |            (cmd/webhook)              |
|                               |     |                                       |
|  * Token Exchange Endpoint    |     |  * Trust Policy Validation            |
|  * OIDC Token Verification    |     |  * GitHub Check Runs                  |
|  * Trust Policy Evaluation    |     |  * PR/Push Event Handling             |
|  * GitHub Token Generation    |     |                                       |
+-------------------------------+     +---------------------------------------+
          |                                           |
          |                                           |
          v                                           v
+-----------------------------------------------------------------------------+
|                           GitHub App Transport                              |
|                            (pkg/ghtransport)                                |
|                                                                             |
|   * App Authentication (KMS/File/Env)                                       |
|   * Installation Token Generation                                           |
|   * JWT Signing (Cloud KMS or Local Key)                                    |
+-----------------------------------------------------------------------------+
                                      |
          +---------------------------+---------------------------+
          v                           v                           v
+-----------------+     +---------------------+     +-------------------------+
| Key Management  |     |   GitHub API        |     |   OIDC Providers        |
|                 |     |                     |     |                         |
| * Cloud KMS     |     | * Apps API          |     | * GitHub Actions        |
| * Local PEM Key |     | * Installation API  |     | * Google Cloud          |
| * Env Var Key   |     | * Repository API    |     | * Any OIDC Provider     |
+-----------------+     +---------------------+     +-------------------------+
```

## Request Flow

### Token Exchange Flow

```
+--------+     +-------------+     +-------------+     +------------+     +----------+
| Client |     | STS Service |     | OIDC        |     | GitHub     |     | GitHub   |
|        |     |             |     | Provider    |     | Repo       |     | API      |
+---+----+     +------+------+     +------+------+     +-----+------+     +----+-----+
    |                 |                   |                  |                 |
    | Exchange Request|                   |                  |                 |
    | (Bearer Token,  |                   |                  |                 |
    | Scope, Identity)|                   |                  |                 |
    |---------------->|                   |                  |                 |
    |                 |                   |                  |                 |
    |                 | Fetch OIDC Config |                  |                 |
    |                 |------------------>|                  |                 |
    |                 |<------------------|                  |                 |
    |                 |                   |                  |                 |
    |                 | Verify JWT        |                  |                 |
    |                 |------------------>|                  |                 |
    |                 |<------------------|                  |                 |
    |                 |                   |                  |                 |
    |                 | Lookup Installation                  |                 |
    |                 |------------------------------------------------------->|
    |                 |<-------------------------------------------------------|
    |                 |                   |                  |                 |
    |                 | Fetch Trust Policy|                  |                 |
    |                 |------------------------------------->|                 |
    |                 |<-------------------------------------|                 |
    |                 |                   |                  |                 |
    |                 | Validate Token vs Policy             |                 |
    |                 | (issuer, subject, claims)            |                 |
    |                 |                   |                  |                 |
    |                 | Generate Installation Token          |                 |
    |                 |------------------------------------------------------->|
    |                 |<-------------------------------------------------------|
    |                 |                   |                  |                 |
    | GitHub Token    |                   |                  |                 |
    |<----------------|                   |                  |                 |
    |                 |                   |                  |                 |
```

### Webhook Validation Flow

```
+--------+     +--------------+     +------------+     +--------------+
| GitHub |     | Webhook      |     | GitHub     |     | Repository   |
|        |     | Service      |     | API        |     |              |
+---+----+     +------+-------+     +-----+------+     +------+-------+
    |                 |                   |                   |
    | Webhook Event   |                   |                   |
    | (PR/Push/Check) |                   |                   |
    |---------------->|                   |                   |
    |                 |                   |                   |
    |                 | Validate Signature|                   |
    |                 |                   |                   |
    |                 | Get Changed Files |                   |
    |                 |------------------>|                   |
    |                 |<------------------|                   |
    |                 |                   |                   |
    |                 | Fetch .sts.yaml   |                   |
    |                 | files             |                   |
    |                 |-------------------------------------->|
    |                 |<--------------------------------------|
    |                 |                   |                   |
    |                 | Parse & Validate  |                   |
    |                 | Trust Policies    |                   |
    |                 |                   |                   |
    |                 | Create CheckRun   |                   |
    |                 |------------------>|                   |
    |                 |<------------------|                   |
    |                 |                   |                   |
    | 200 OK          |                   |                   |
    |<----------------|                   |                   |
    |                 |                   |                   |
```

## Trust Policy Model

Trust policies define the federation rules that map external OIDC identities to GitHub permissions.

### Policy Location

```
Repository: org/repo
\-- .github/
    \-- chainguard/
        \-- {identity}.sts.yaml    # Repository-scoped policy

Organization: org
\-- .github/
    \-- chainguard/
        \-- {identity}.sts.yaml    # Organization-scoped policy
```

### Policy Structure

```yaml
# Basic Trust Policy (TrustPolicy)
issuer: https://token.actions.githubusercontent.com     # Exact match
# OR
issuer_pattern: "https://.*\\.example\\.com"           # Regex pattern

subject: repo:org/repo:ref:refs/heads/main             # Exact match
# OR
subject_pattern: "repo:org/repo:.*"                    # Regex pattern

audience: octo-sts.dev                                 # Optional, defaults to domain
# OR
audience_pattern: ".*"                                 # Optional regex

claim_pattern:                                         # Optional custom claims
  email: ".*@example\\.com"
  workflow: "release\\.yml"

permissions:                                           # GitHub App permissions
  contents: read
  issues: write
  pull_requests: write
```

```yaml
# Organization Trust Policy (OrgTrustPolicy)
# Extends TrustPolicy with repository scoping
issuer: https://token.actions.githubusercontent.com
subject: repo:other-org/repo:ref:refs/heads/main

permissions:
  contents: read

repositories:                                          # Limit to specific repos
  - repo1
  - repo2
```

## Security Model

### Token Validation

1. **Issuer Validation**: OIDC issuer must be HTTPS (except localhost for testing)
2. **Subject Validation**: Max 255 characters, no control characters
3. **Audience Validation**: Must match configured domain or explicit audience
4. **Signature Verification**: JWT signature verified against issuer's JWKS
5. **Expiration Check**: Token must not be expired

### Token Lifecycle

1. **Short-lived tokens**: Generated tokens have limited lifetime
2. **Minimal permissions**: Tokens are scoped to trust policy permissions
3. **Token revocation**: Internal tokens are revoked after trust policy lookup
4. **No refresh tokens**: `ExchangeRefreshToken` is not implemented

### Caching Strategy

**Installation ID Cache** (LRU, 200 entries)
- Key: owner
- Value: GitHub App installation ID

**Trust Policy Cache** (Expirable LRU, 200 entries, 5min TTL)
- Key: (owner, repo, identity)
- Value: raw policy YAML

**OIDC Provider Cache** (LRU, 100 entries)
- Key: issuer URL
- Value: OIDC provider with verifier

## Key Management & JWT Signing

Octo-STS requires the GitHub App's private key to authenticate with GitHub and sign JWTs for obtaining installation tokens. The system supports multiple key storage backends to accommodate different deployment environments and security requirements.

### How JWT Signing Works

When Octo-STS needs to interact with the GitHub API on behalf of an installation:

1. **App Authentication**: The service creates a JWT signed with the GitHub App's private key
2. **JWT Structure**: The token includes the App ID, issued-at time, and expiration (10 minutes max)
3. **Signature Algorithm**: RS256 (RSA with SHA-256) is used for all signing operations
4. **Token Exchange**: GitHub validates the JWT and returns a short-lived installation access token

### GitHub App Private Key Storage

The GitHub App Transport (`pkg/ghtransport`) supports three mutually exclusive methods for storing the GitHub App's private key. Only one method should be configured; if multiple are set, the service will fail to start.

#### 1. Environment Variable (`APP_SECRET_CERTIFICATE_ENV_VAR`)

The GitHub App's private key is passed directly as a PEM-encoded string in an environment variable.

```
APP_SECRET_CERTIFICATE_ENV_VAR="-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA...
-----END RSA PRIVATE KEY-----"
```

**Use cases**:
- Container deployments where secrets are injected at runtime
- Kubernetes deployments using secrets mounted as environment variables
- Serverless platforms with native secrets management

**Considerations**:
- Key is held in memory; ensure the runtime environment is secure
- Rotation requires redeployment or environment variable update

#### 2. File Path (`APP_SECRET_CERTIFICATE_FILE`)

The GitHub App's private key is read from a PEM file on disk.

```
APP_SECRET_CERTIFICATE_FILE=/path/to/github-app-private-key.pem
```

**Use cases**:
- Local development and testing
- VM-based deployments with secure key storage
- Kubernetes deployments using secrets mounted as files

**Considerations**:
- File permissions must restrict access (e.g., `chmod 600`)
- Key must be provisioned to the filesystem before service startup

#### 3. Cloud KMS (`KMS_KEY`)

The GitHub App's private key is stored in a cloud Key Management Service. Signing operations are performed remotely by the KMS; the private key material never leaves the KMS boundary.

```
KMS_KEY=projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key/cryptoKeyVersions/1
```

**Use cases**:
- Production deployments requiring HSM-backed key protection
- Environments with strict key custody requirements
- Multi-region deployments needing centralized key management

**Considerations**:
- Adds latency for each signing operation (network round-trip to KMS)
- Requires appropriate IAM permissions for the service identity
- Key never leaves the KMS boundary (highest security)

### Current KMS Support

Octo-STS currently supports **GCP Cloud KMS** via the `pkg/gcpkms` package.

### Extending KMS Support

The signing architecture uses the `ghinstallation.Signer` interface, which allows adding support for additional KMS providers. To add a new provider:

1. **Implement the `ghinstallation.Signer` interface** to sign JWTs using the new KMS
2. **Update `pkg/ghtransport`** to detect the new KMS configuration and instantiate the appropriate signer

See `pkg/gcpkms/gcpkms.go` for a reference implementation.

### Recommendations by Environment

| Environment | Recommended Method | Rationale |
|-------------|-------------------|-----------|
| Local development | File | Simple setup, easy key rotation |
| CI/CD testing | Environment variable | Secrets injected by CI platform |
| Production (cloud) | Cloud KMS | HSM protection, audit logging, no key extraction |
| Production (on-prem) | File or Env var | Depends on available secrets management |
| High-security | Cloud KMS | Key never leaves secure boundary |

## Infrastructure Architecture (GCP Reference)

The reference deployment uses GCP Cloud Run with the following architecture:

```
+-----------------------------------------------------------------------------+
|                           Google Cloud Platform                             |
+-----------------------------------------------------------------------------+
|                                                                             |
|  +---------------------------------------------------------------------+    |
|  |                    Global Cloud Load Balancer                       |    |
|  |                      (octo-sts.dev, webhook.octo-sts.dev)           |    |
|  +---------------------------------------------------------------------+    |
|                                      |                                      |
|         +----------------------------+----------------------------+         |
|         v                            v                            v         |
|  +--------------+         +--------------+              +--------------+    |
|  | Region 1     |         | Region 2     |              | Region N     |    |
|  |              |         |              |              |              |    |
|  | +----------+ |         | +----------+ |              | +----------+ |    |
|  | |STS Svc   | |         | |STS Svc   | |              | |STS Svc   | |    |
|  | |Cloud Run | |         | |Cloud Run | |              | |Cloud Run | |    |
|  | +----------+ |         | +----------+ |              | +----------+ |    |
|  | +----------+ |         | +----------+ |              | +----------+ |    |
|  | |Webhook   | |         | |Webhook   | |              | |Webhook   | |    |
|  | |Svc       | |         | |Svc       | |              | |Svc       | |    |
|  | +----------+ |         | +----------+ |              | +----------+ |    |
|  +--------------+         +--------------+              +--------------+    |
|                                      |                                      |
|                                      v                                      |
|  +---------------------------------------------------------------------+    |
|  |                        Shared Services                              |    |
|  |                                                                     |    |
|  |  +------------+  +-----------------+  +--------------------------+  |    |
|  |  | Cloud KMS  |  | Secret Manager  |  | CloudEvents Broker       |  |    |
|  |  |            |  |                 |  |                          |  |    |
|  |  | * App Key  |  | * Webhook       |  | * Event Ingress          |  |    |
|  |  |            |  |   Secret        |  | * BigQuery Recording     |  |    |
|  |  +------------+  +-----------------+  +--------------------------+  |    |
|  +---------------------------------------------------------------------+    |
|                                                                             |
+-----------------------------------------------------------------------------+
```

## API Specification

### Exchange Endpoint

**gRPC Service**: `SecurityTokenService.Exchange`

**REST Endpoint**: `POST /sts/exchange` or `GET /sts/exchange`

**Request Parameters**:

| Parameter            | Type         | Required | Description                                          |
|----------------------|--------------|----------|------------------------------------------------------|
| `scope`              | string       | Yes      | Repository or organization (e.g., `org/repo` or `org`) |
| `identity`           | string       | Yes      | Trust policy name (maps to `{identity}.sts.yaml`)    |
| Authorization Header | Bearer token | Yes      | OIDC token to exchange                               |

**Response**:
```json
{
  "token": "ghs_xxxxxxxxxxxxxxxxxxxx"
}
```

**Error Codes**:

| Code                | Description                                |
|---------------------|--------------------------------------------|
| `UNAUTHENTICATED`   | Invalid or missing bearer token            |
| `INVALID_ARGUMENT`  | Invalid scope, identity, or token format   |
| `NOT_FOUND`         | Trust policy not found or cannot be parsed |
| `PERMISSION_DENIED` | Token does not match trust policy          |
| `INTERNAL`          | Token generation failed                    |

## Observability

### Metrics

When metrics are enabled (`METRICS=true`), the service:
- Exposes Prometheus metrics endpoint
- Integrates with OpenTelemetry tracing
- Records CloudEvents for each exchange attempt

### CloudEvents

Each token exchange emits a CloudEvent of type `dev.octo-sts.exchange`:

```json
{
  "actor": {
    "iss": "https://token.actions.githubusercontent.com",
    "sub": "repo:org/repo:ref:refs/heads/main",
    "claims": [
      {"name": "workflow", "value": "deploy.yml"}
    ]
  },
  "trust_policy": { /* policy details */ },
  "installation_id": 12345678,
  "scope": "org/repo",
  "identity": "deploy",
  "token_sha256": "abc123...",
  "error": "",
  "time": "2024-01-15T10:30:00Z"
}
```

### Alerting (GCP Reference)

- **KMS Access Monitoring**: Alerts on unauthorized access to signing keys
- **Error Rate Monitoring**: BigQuery views for error analysis by installation and subject
- **Service Health**: Cloud Run built-in monitoring and alerting

## Deployment Considerations

### Environment Variables

| Variable                             | Required     | Description                  |
|--------------------------------------|--------------|------------------------------|
| `PORT`                               | Yes          | HTTP server port             |
| `GITHUB_APP_ID`                      | Yes          | GitHub App ID                |
| `KMS_KEY`                            | One of three | GCP KMS key reference        |
| `APP_SECRET_CERTIFICATE_FILE`        | One of three | Path to PEM key file         |
| `APP_SECRET_CERTIFICATE_ENV_VAR`     | One of three | PEM key as env var           |
| `STS_DOMAIN`                         | Yes (app)    | Domain for audience check    |
| `GITHUB_WEBHOOK_SECRET`              | Yes (webhook)| Webhook signature secret     |
| `GITHUB_WEBHOOK_ORGANIZATION_FILTER` | No           | Comma-separated org filter   |
| `METRICS`                            | No           | Enable metrics (default true)|
| `EVENT_INGRESS_URI`                  | No           | CloudEvents ingress URI      |

### Scaling Considerations

- **Stateless design**: All state is external (GitHub, KMS, caches are ephemeral)
- **Regional deployment**: Deploy to multiple regions for availability
- **Cache sizing**: LRU caches prevent memory growth
- **Rate limiting**: Subject to GitHub API rate limits per installation

# Octo-STS Component Breakdown

> **Note:** This document describes components from the upstream octo-sts/app
> project. For information about distribution-specific components
> (app-installer, Docker setup, etc.), see the respective README files.

This document provides a detailed breakdown of the octo-sts/app components,
including binaries, packages, and their responsibilities.

## Binary Components

### cmd/app - STS Exchange Service

**Purpose**: Main Security Token Service that handles OIDC-to-GitHub token exchanges.

**Location**: `cmd/app/main.go`

**Responsibilities**:
- Expose gRPC and REST endpoints for token exchange
- Validate incoming OIDC tokens
- Evaluate trust policies
- Generate scoped GitHub installation tokens
- Emit CloudEvents for observability

**Key Dependencies**:
- `chainguard.dev/go-grpc-kit/pkg/duplex` - Dual gRPC/HTTP server
- `chainguard.dev/sdk/proto/platform/oidc/v1` - gRPC service definitions
- `cloud.google.com/go/kms` - GCP KMS client (optional)
- `github.com/cloudevents/sdk-go/v2` - CloudEvents emission

**Configuration**:

| Env Variable                     | Description                       |
|----------------------------------|-----------------------------------|
| `PORT`                           | Server listening port             |
| `GITHUB_APP_ID`                  | GitHub App identifier             |
| `KMS_KEY`                        | GCP KMS key path (optional)       |
| `APP_SECRET_CERTIFICATE_FILE`    | PEM key file path (optional)      |
| `APP_SECRET_CERTIFICATE_ENV_VAR` | PEM key as env var (optional)     |
| `STS_DOMAIN`                     | Service domain for validation     |
| `EVENT_INGRESS_URI`              | CloudEvents endpoint              |
| `METRICS`                        | Enable metrics/tracing            |

---

### cmd/webhook - Trust Policy Validator

**Purpose**: GitHub webhook handler that validates trust policy changes in pull requests and pushes.

**Location**: `cmd/webhook/main.go`

**Responsibilities**:
- Receive and validate GitHub webhook events
- Parse and validate trust policy YAML files
- Create GitHub Check Runs for validation results
- Support for PullRequest, Push, CheckSuite, and CheckRun events

**Key Dependencies**:
- `cloud.google.com/go/secretmanager` - Fetch webhook secrets (GCP)
- `github.com/google/go-github/v75` - GitHub API client
- `github.com/bradleyfalzon/ghinstallation/v2` - GitHub App auth

**Configuration**:

| Env Variable                         | Description                   |
|--------------------------------------|-------------------------------|
| `PORT`                               | Server listening port         |
| `GITHUB_APP_ID`                      | GitHub App identifier         |
| `KMS_KEY`                            | GCP KMS key path (optional)   |
| `GITHUB_WEBHOOK_SECRET`              | Webhook signature secret(s)   |
| `GITHUB_WEBHOOK_ORGANIZATION_FILTER` | Comma-separated org filter    |
| `METRICS`                            | Enable metrics/tracing        |

---

### cmd/prober - Health Prober

**Purpose**: Positive test prober that validates the STS service is functioning correctly.

**Location**: `cmd/prober/main.go`

**Responsibilities**:
- Exchange a GCP ID token for a GitHub token
- Validate returned permissions (contents: read, issues: read)
- Verify permission boundaries (issues: write should fail)
- Validate non-existent identity handling
- Revoke tokens after use

**Configuration**:
- `STS_DOMAIN` - STS service domain

---

### cmd/negative-prober - Negative Test Prober

**Purpose**: Negative test prober that validates the STS service properly rejects invalid requests.

**Location**: `cmd/negative-prober/main.go`

**Responsibilities**:
- Attempt token exchange that should fail
- Verify proper rejection of unauthorized requests

**Configuration**:
- `STS_DOMAIN` - STS service domain

---

### cmd/schemagen - JSON Schema Generator

**Purpose**: Development tool to generate JSON schemas from Go types.

**Location**: `cmd/schemagen/main.go`

**Responsibilities**:
- Generate `TrustPolicy` JSON schema
- Generate `OrgTrustPolicy` JSON schema
- Output to `pkg/octosts/` directory

**Usage**:
```bash
go generate ./cmd/schemagen/...
```

---

## Package Components

### pkg/octosts - Core STS Logic

**Purpose**: Core Security Token Service implementation including trust policy evaluation and token exchange.

**Location**: `pkg/octosts/`

**Files**:

#### octosts.go
Main STS server implementation.

**Key Types**:
```go
type sts struct {
    atr      *ghinstallation.AppsTransport
    ceclient cloudevents.Client
    domain   string
    metrics  bool
}
```

**Key Functions**:
- `NewSecurityTokenServiceServer()` - Factory for STS server
- `Exchange()` - Main token exchange handler
- `lookupInstallAndTrustPolicy()` - Resolve installation and policy
- `lookupInstall()` - Find GitHub App installation by owner
- `lookupTrustPolicy()` - Fetch and parse trust policy from repo

**Caching**:
- `installationIDs` - LRU cache (200 entries) for installation ID lookups
- `trustPolicies` - Expirable LRU cache (200 entries, 5min TTL) for policies

#### trust_policy.go
Trust policy data structures and validation.

**Key Types**:
```go
type TrustPolicy struct {
    Issuer          string                            // Exact issuer match
    IssuerPattern   string                            // Regex issuer match
    Subject         string                            // Exact subject match
    SubjectPattern  string                            // Regex subject match
    Audience        string                            // Exact audience match
    AudiencePattern string                            // Regex audience match
    ClaimPattern    map[string]string                 // Custom claim patterns
    Permissions     github.InstallationPermissions    // Requested permissions
}

type OrgTrustPolicy struct {
    TrustPolicy
    Repositories []string    // Limit to specific repositories
}
```

**Key Functions**:
- `Compile()` - Validate and compile regex patterns
- `CheckToken()` - Validate OIDC token against policy

#### event.go
CloudEvent data structures for observability.

**Key Types**:
```go
type Event struct {
    Actor          Actor           // Token issuer/subject/claims
    TrustPolicy    *OrgTrustPolicy // Matched policy
    InstallationID int64           // GitHub App installation
    Scope          string          // Requested scope
    Identity       string          // Policy identity name
    TokenSHA256    string          // Hash of generated token
    Error          string          // Error message if failed
    Time           time.Time       // Request timestamp
}

type Actor struct {
    Issuer  string  // OIDC issuer
    Subject string  // OIDC subject
    Claims  []Claim // Matched custom claims
}
```

#### revoke.go
Token revocation utility.

**Key Functions**:
- `Revoke()` - Revoke a GitHub installation token via API

---

### pkg/ghtransport - GitHub App Transport

**Purpose**: Factory for creating authenticated GitHub App transports.

**Location**: `pkg/ghtransport/ghtransport.go`

**Supported Authentication Methods**:
1. **Environment Variable**: PEM key in `APP_SECRET_CERTIFICATE_ENV_VAR`
2. **File**: PEM key file at `APP_SECRET_CERTIFICATE_FILE`
3. **GCP KMS**: Signing via Cloud KMS using `KMS_KEY`

**Key Functions**:
```go
func New(ctx context.Context, env *envConfig.EnvConfig, kmsClient *kms.KeyManagementClient) (*ghinstallation.AppsTransport, error)
```

---

### pkg/gcpkms - GCP KMS Signer

**Purpose**: JWT signing implementation using Google Cloud KMS.

**Location**: `pkg/gcpkms/gcpkms.go`

**Key Types**:
```go
type gcpSigner struct {
    client *kms.KeyManagementClient
    key    string
}
```

**Key Functions**:
- `New()` - Create a new KMS-backed signer
- `Sign()` - Sign JWT claims using KMS asymmetric signing

**Algorithm**: RSA_SIGN_PKCS1_2048_SHA256 (RS256)

---

### pkg/oidcvalidate - OIDC Validation

**Purpose**: Input validation for OIDC token fields to prevent injection and security issues.

**Location**: `pkg/oidcvalidate/validate.go`

**Key Functions**:
```go
func IsValidIssuer(iss string) bool    // Validate issuer URL format
func IsValidSubject(sub string) bool   // Validate subject format
func IsValidAudience(aud string) bool  // Validate audience format
```

**Validation Rules**:

- **Issuer**: HTTPS required (HTTP allowed for localhost), no query/fragment, valid hostname, max 255 chars
- **Subject**: Non-empty, max 255 chars, no control characters, printable chars only
- **Audience**: Non-empty, max 255 chars, no control characters, no injection chars

**Security Protections**:
- Homograph attack prevention (ASCII only in hostnames)
- Path traversal prevention
- Control character filtering
- Injection character blocking

---

### pkg/provider - OIDC Provider Cache

**Purpose**: Caching layer for OIDC providers to avoid repeated discovery.

**Location**: `pkg/provider/provider.go`

**Key Types**:
```go
type VerifierProvider interface {
    Verifier(config *oidc.Config) *oidc.IDTokenVerifier
}
```

**Key Functions**:
```go
func Get(ctx context.Context, issuer string) (VerifierProvider, error)
```

**Features**:
- LRU cache (100 entries) for providers
- Exponential backoff retry for discovery
- Response size limiting (100KB max)
- Redirect validation using same rules as issuer

**Retry Configuration**:
- Initial interval: 1 second
- Max interval: 30 seconds
- Multiplier: 2.0
- Jitter: +/- 10%

---

### pkg/webhook - GitHub Webhook Handler

**Purpose**: Process GitHub webhooks for trust policy validation.

**Location**: `pkg/webhook/webhook.go`

**Key Types**:
```go
type Validator struct {
    Transport     *ghinstallation.AppsTransport
    WebhookSecret [][]byte      // Multiple secrets for rolling updates
    Organizations []string      // Optional org filter
}
```

**Supported Events**:

| Event Type         | Handler               | Description               |
|--------------------|-----------------------|---------------------------|
| `PullRequestEvent` | `handlePullRequest()` | Validate policies in PR   |
| `PushEvent`        | `handlePush()`        | Validate policies in push |
| `CheckSuiteEvent`  | `handleCheckSuite()`  | Validate on check suite   |
| `CheckRunEvent`    | `handleCheckSuite()`  | Validate on check run     |

**Validation Process**:
1. Validate webhook signature against secrets
2. Parse event payload
3. Identify changed `.github/chainguard/*.sts.yaml` files
4. Fetch and parse each policy file
5. Create CheckRun with success/failure result

**Key Functions**:
- `ServeHTTP()` - Main HTTP handler
- `validatePayload()` - Verify webhook signature
- `handleSHA()` - Validate policies for a commit
- `validatePolicies()` - Parse and validate policy files

---

### pkg/envconfig - Environment Configuration

**Purpose**: Parse and validate environment configuration.

**Location**: `pkg/envconfig/envconfig.go`

**Key Types**:
```go
type EnvConfig struct {
    Port                       int
    KMSKey                     string
    AppID                      int64
    AppSecretCertificateFile   string
    AppSecretCertificateEnvVar string
    Metrics                    bool
}

type EnvConfigApp struct {
    Domain          string
    EventingIngress string
}

type EnvConfigWebhook struct {
    WebhookSecret      string
    OrganizationFilter string
}
```

**Key Functions**:
```go
func BaseConfig() (*EnvConfig, error)
func AppConfig() (*EnvConfigApp, error)
func WebhookConfig() (*EnvConfigWebhook, error)
```

**Validation**:
- Only one of `KMS_KEY`, `APP_SECRET_CERTIFICATE_FILE`, or `APP_SECRET_CERTIFICATE_ENV_VAR` may be set

---

### pkg/maxsize - Response Size Limiter

**Purpose**: HTTP transport wrapper that limits response body size.

**Location**: `pkg/maxsize/maxsize.go`

**Key Functions**:
```go
func NewRoundTripper(maxSize int64, inner http.RoundTripper) http.RoundTripper
```

**Usage**: Wraps HTTP client to prevent memory exhaustion from large OIDC provider responses.

---

### pkg/prober - Prober Functions

**Purpose**: Health check implementations for positive and negative probing.

**Location**: `pkg/prober/prober.go`

**Key Functions**:
```go
func Func(ctx context.Context) error     // Positive prober
func Negative(ctx context.Context) error // Negative prober
```

**Positive Prober Tests**:
1. Exchange GCP token for GitHub token
2. Read repository contents (should succeed)
3. List issues (should succeed)
4. Create issue (should fail - read-only)
5. Exchange with non-existent identity (should fail)

---

## Data Flow Summary

```
+-----------------------------------------------------------------------------+
|                           Component Interaction                             |
+-----------------------------------------------------------------------------+

cmd/app
    |
    +--> pkg/envconfig        (Load configuration)
    +--> pkg/ghtransport      (Create GitHub App transport)
    |         |
    |         \--> pkg/gcpkms (KMS signing if configured)
    |
    \--> pkg/octosts          (Main STS logic)
              |
              +--> pkg/provider      (OIDC provider discovery)
              |         |
              |         \--> pkg/maxsize (Response limiting)
              |
              \--> pkg/oidcvalidate  (Input validation)

cmd/webhook
    |
    +--> pkg/envconfig        (Load configuration)
    +--> pkg/ghtransport      (Create GitHub App transport)
    |
    \--> pkg/webhook          (Webhook handling)
              |
              \--> pkg/octosts (Trust policy parsing)
```

## Trust Policy File Locations

**Repository-Level Policy**: `{org}/{repo}/.github/chainguard/{identity}.sts.yaml`
- Uses TrustPolicy schema
- Permissions scoped to the repository

**Organization-Level Policy**: `{org}/.github/chainguard/{identity}.sts.yaml`
- Uses OrgTrustPolicy schema
- Can specify repositories list
- Permissions can span multiple repos

## JSON Schemas

Two JSON schemas are provided for IDE autocompletion:

### TrustPolicy Schema
**Location**: `pkg/octosts/octosts.TrustPolicy.json`
**Usage**: Repository-level trust policies

### OrgTrustPolicy Schema
**Location**: `pkg/octosts/octosts.OrgTrustPolicy.json`
**Usage**: Organization-level trust policies with repository scoping

**VS Code Configuration**:
```json
{
  "yaml.schemas": {
    "https://raw.githubusercontent.com/octo-sts/app/refs/heads/main/pkg/octosts/octosts.TrustPolicy.json": "/.github/chainguard/*"
  }
}
```

## External Dependencies

### Core Dependencies

| Package                                      | Version  | Purpose                   |
|----------------------------------------------|----------|---------------------------|
| `chainguard.dev/go-grpc-kit`                 | v0.17.15 | gRPC/HTTP duplex server   |
| `chainguard.dev/sdk`                         | v0.1.44  | STS proto definitions     |
| `github.com/bradleyfalzon/ghinstallation/v2` | v2.17.0  | GitHub App authentication |
| `github.com/google/go-github/v75`            | v75.0.0  | GitHub API client         |
| `github.com/coreos/go-oidc/v3`               | v3.17.0  | OIDC verification         |
| `github.com/cloudevents/sdk-go/v2`           | v2.16.2  | CloudEvents emission      |
| `github.com/hashicorp/golang-lru/v2`         | v2.0.7   | LRU caching               |

### GCP Dependencies (Optional)

| Package                            | Version  | Purpose          |
|------------------------------------|----------|------------------|
| `cloud.google.com/go/kms`          | v1.23.2  | KMS signing      |
| `cloud.google.com/go/secretmanager`| v1.16.0  | Secret retrieval |

### Infrastructure Dependencies

| Package                                          | Version | Purpose           |
|--------------------------------------------------|---------|-------------------|
| `github.com/chainguard-dev/terraform-infra-common`| v0.9.7  | Terraform modules |
| `github.com/kelseyhightower/envconfig`           | v1.4.0  | Env var parsing   |

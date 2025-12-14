# Octo-STS Docker Deployment

> **For Local Development Only**
>
> This Docker distribution is designed for local testing and proof-of-concept
> deployments. It is **not production-ready** and should not be exposed to the
> public internet without additional security hardening.

Run Octo-STS locally using Docker Compose with ngrok for GitHub webhook
connectivity.

## Prerequisites

- Docker Engine 24.0+ and Docker Compose v2.20+
- [ngrok](https://ngrok.com/) account and CLI
- GitHub account with permission to create GitHub Apps

## Architecture

```
+---------------+
|    ngrok      |  Provides public HTTPS endpoint
+-------+-------+
        |
        v
+-------+--------------------------------------+
| Caddy (Reverse Proxy)                        |
+-------+-------------------+------------------+
        |                   |
        v                   v
+-------+-------+   +-------+----------------+
| STS Service   |   | App Service            |
| Port 8080     |   | Port 8080              |
+---------------+   | - /webhook (webhooks)  |
                    | - /setup (installer)   |
                    | - /healthz (health)    |
                    +------------------------+
```

For detailed architecture information, see the
[upstream architecture documentation](https://github.com/octo-sts/app#architecture).

## Setup

### 1. Configure Environment

```bash
cp .env.example .env
```

Edit `.env` and set `GITHUB_ORG` to the organization where the GitHub App will
be created (leave empty for personal account).

### 2. Start ngrok

In a terminal, run:

```bash
ngrok http 9000
```

For free tier users, ngrok no longer displays the forwarding URL directly in
the CLI. Instead:

1. Open `http://localhost:4040` in your browser (ngrok web interface)
2. Find and copy the forwarding URL (e.g., `https://abc123.ngrok-free.app`)
3. This URL will be used for both the installer and webhook configuration

### 3. Start Docker Services

```bash
docker compose up --build
```

This starts all services with the installer enabled by default. The installer
is available at `/setup` on your ngrok URL.

### 4. Create the GitHub App

Open your ngrok URL with `/setup` path (e.g., `https://abc123.ngrok-free.app/setup`)
in your browser.

Follow the prompts to create your GitHub App. When prompted for the webhook
URL, enter your ngrok URL with `/webhook` path (e.g.,
`https://abc123.ngrok-free.app/webhook`).

The installer automatically saves the GitHub App credentials to your `.env`
file.

### 5. Restart Services

After the GitHub App is created, restart the services to load the new
credentials:

```bash
docker compose down
docker compose up --build
```

Optionally, disable the installer by setting `INSTALLER_ENABLED=false` in
`.env` (recommended for security after setup is complete).

Your Octo-STS instance is now running at your ngrok URL.

## Endpoints

| Path | Description |
|------|-------------|
| `/` | STS token exchange endpoint |
| `/webhook` | GitHub webhook receiver |
| `/setup` | Installer UI (when enabled) |
| `/setup/callback` | OAuth callback (when enabled) |
| `/healthz` | Health check |

## Next Steps

- [Create trust policies](https://octo-sts.dev) to define which identities can
  request tokens
- [Configure token exchange](https://octo-sts.dev) in your CI/CD workflows

## Troubleshooting

### Services won't start

Check logs:
```bash
docker compose logs sts
docker compose logs app
```

### Port already in use

Change the HTTP_PORT in `.env`:
```bash
HTTP_PORT=9001
```

### GitHub App webhook not receiving events

1. Check ngrok is still running at same URL
2. Update webhook URL in GitHub App settings if ngrok URL changed
3. Check webhook deliveries in GitHub App settings for error details

### Can't access ngrok URL

Free ngrok URLs expire. Restart ngrok and update the GitHub App webhook URL.

## See Also

- [Upstream Documentation](https://github.com/octo-sts/app)
- [Architecture Overview](../../docs/architecture.md)

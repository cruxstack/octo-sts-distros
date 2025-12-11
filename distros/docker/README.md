# Octo-STS Docker Deployment

> **⚠️ For Local Development Only**
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
| STS Service   |   | Webhook Service        |
| Port 8080     |   | Port 8080              |
+---------------+   +------------------------+

Setup Phase:
+---------------------------------------------+
| App Installer (Profile: setup)              |
| Creates GitHub App via manifest flow        |
+---------------------------------------------+
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

### 2. Start Docker Setup

```bash
docker compose --profile setup up app-installer --build
```

This starts the app-installer service which will be accessible through
ngrok.

### 3. Start ngrok

In a separate terminal, run:

```bash
ngrok http 9000
```

For free tier users, ngrok no longer displays the forwarding URL directly in
the CLI. Instead:

1. Open `http://localhost:4040` in your browser (ngrok web interface)
2. Find and copy the forwarding URL (e.g., `https://abc123.ngrok-free.app`)
3. This URL will be used for both the app installer and webhook configuration

### 4. Create the GitHub App

Open your ngrok forwarding URL (from step 3) in your browser to access the
app-installer interface.

Follow the prompts to create your GitHub App. When prompted for the webhook
URL, enter your ngrok URL with `/webhook` path (e.g.,
`https://abc123.ngrok-free.app/webhook`).

The installer automatically saves the GitHub App credentials to your `.env`
file.

### 5. Start the Services

Stop the app-installer (Ctrl+C), then start the services:

```bash
docker compose up --build
```

This starts:
- **Caddy** - Reverse proxy (port 9000)
- **STS** - Token exchange service
- **Webhook** - PR validation service

Your Octo-STS instance is now running at your ngrok URL.

## Next Steps

- [Create trust policies](https://octo-sts.dev) to define which identities can
  request tokens
- [Configure token exchange](https://octo-sts.dev) in your CI/CD workflows

## Troubleshooting

### Services won't start

Check logs:
```bash
docker compose logs sts
docker compose logs webhook
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

- [App Installer Documentation](../../app-installer/README.md)
- [Upstream Documentation](https://github.com/octo-sts/app)
- [Architecture Overview](../../docs/architecture.md)

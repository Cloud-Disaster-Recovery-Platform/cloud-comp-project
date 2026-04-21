# Cloudflare Tunnel Integration Guide

This guide describes how to configure and deploy a Cloudflare Tunnel to securely connect your local application to the Cloud Mirror failsafe infrastructure.

## Architecture

Cloudflare Tunnel provides a secure way to connect your local resources to Cloudflare without opening public inbound ports. The `cloudflared` daemon runs alongside your application and establishes outbound-only connections to Cloudflare's edge.

```
[Local App] <--> [cloudflared] <--> [Cloudflare Edge] <--> [Internet]
```

## Prerequisites

- A Cloudflare account with a domain added
- Cloudflare Zero Trust enabled
- `cloudflared` installed on the local server

## Setup Instructions

### 1. Authenticate cloudflared

Log in to your Cloudflare account:

```bash
cloudflared tunnel login
```

This will open a browser window for authentication and download a certificate file (usually to `~/.cloudflared/cert.pem`).

### 2. Create the Tunnel

Create a new tunnel for your application:

```bash
cloudflared tunnel create <TUNNEL_NAME>
```

This command generates a tunnel ID and a credentials JSON file. **Keep this JSON file secure**; it is required to run the tunnel.

### 3. Configure the Tunnel

Create a configuration file `config.yaml` (usually in `~/.cloudflared/` or your project root):

```yaml
tunnel: <TUNNEL_ID>
credentials-file: /path/to/<TUNNEL_ID>.json

ingress:
  - hostname: app.yourdomain.com
    service: http://localhost:3000  # Path to your local Node.js app
  - hostname: status.yourdomain.com
    service: http://localhost:8080  # Path to State Sync Engine status
  - service: http_status:404
```

### 4. Route Traffic to the Tunnel

Create a DNS CNAME record that points to your tunnel:

```bash
cloudflared tunnel route dns <TUNNEL_NAME> app.yourdomain.com
```

### 5. Run the Tunnel

Start the tunnel daemon:

```bash
cloudflared tunnel run <TUNNEL_NAME>
```

For production, it is recommended to run `cloudflared` as a system service.

## Integration with Cloud Mirror Failover

In the Cloud Mirror architecture, Cloudflare acts as the primary entry point. When GCP Cloud DNS health checks detect a failure, they update the DNS records.

1. **Normal Path**: `User -> Cloudflare -> Tunnel -> Local App`
2. **Failover Path**: `User -> Cloudflare -> Cloud Run (GCP)`

### Health Check Requirements

The local application MUST expose a `/health` endpoint that returns a `200 OK` status when functional. This endpoint is used by GCP Cloud DNS to monitor the tunnel's health.

Example `curl` validation:
```bash
curl -f https://app.yourdomain.com/health
```

## Security Best Practices

- **Least Privilege**: Ensure the machine running `cloudflared` only has access to the required local ports.
- **Secret Management**: Do not commit the tunnel credentials JSON file to source control. Use environment variables or a secret manager.
- **Tunnel Permissions**: Use Cloudflare Access policies to restrict who can reach your tunnel endpoints if they are not meant to be public.

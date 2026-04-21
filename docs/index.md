# Cloud Mirror Documentation Index

Welcome to the Cloud Mirror documentation. This system provides a headless failsafe infrastructure with automatic failover to GCP.

## Core Documentation
- [Architecture & Design](design.md): Detailed system overview and component responsibilities.
- [State Sync Engine README](../state-sync-engine/README.md): Setup and configuration for the Go replication service.
- [Requirements](requirements.md): System requirements and constraints.

## Integration & Deployment Guides
- [Terraform Deployment](terraform-deployment.md): How to provision the GCP infrastructure.
- [Cloudflare Tunnel Integration](cloudflare-tunnel.md): Securely connecting local apps to the edge.
- [Container Packaging](container-packaging.md): Docker templates for Node.js, Go, and Python apps.

## Operations & Monitoring
- [Operational Runbook](operational-runbook.md): Monitoring, troubleshooting, and manual procedures.
- [Cloud Monitoring Setup](cloud-monitoring.md): Configuring alerts and dashboards in GCP.

## Project Management
- [Implementation Tasks](tasks.md): Track progress and pending items.

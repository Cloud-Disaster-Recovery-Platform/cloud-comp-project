# Terraform Deployment Guide

This guide describes how to provision the Cloud Mirror backup infrastructure on Google Cloud Platform using Terraform.

## Prerequisites

- [Terraform](https://www.terraform.io/downloads.html) 1.0+
- [Google Cloud SDK (gcloud)](https://cloud.google.com/sdk/docs/install)
- A GCP Project with billing enabled
- Owner or Editor permissions on the GCP project

## Setup Instructions

### 1. Authenticate with GCP

```bash
gcloud auth login
gcloud auth application-default login
gcloud config set project <YOUR_PROJECT_ID>
```

### 2. Enable Required APIs

Run the following command to enable the necessary GCP services:

```bash
gcloud services enable \
  compute.googleapis.com \
  sqladmin.googleapis.com \
  run.googleapis.com \
  dns.googleapis.com \
  storage.googleapis.com \
  monitoring.googleapis.com \
  logging.googleapis.com \
  vpcaccess.googleapis.com
```

### 3. Configure Variables

Create a `terraform.tfvars` file in the `terraform/` directory. You can use `terraform.tfvars.example` as a template:

```hcl
project_id          = "your-project-id"
project_name        = "cloud-mirror"
region              = "us-central1"
database_name       = "myapp"
replicator_password = "your-secure-password"
app_container_image = "gcr.io/your-project/your-app:latest"
domain              = "example.com"
local_app_ip        = "1.2.3.4"
lock_bucket_name    = "cloud-mirror-locks"
```

### 4. Initialize and Apply

```bash
cd terraform
terraform init
terraform plan  # Review the changes
terraform apply
```

### 5. Verify Outputs

After a successful apply, Terraform will display important outputs:
- `cloud_sql_private_ip`: Use this in your State Sync Engine config.
- `cloud_run_service_uri`: The URL of your backup application.
- `dns_nameservers`: Update your domain's nameservers if using the managed zone.

## Infrastructure Components

The Terraform configuration provisions:
- **VPC Network**: A private network for secure communication.
- **Cloud SQL**: A PostgreSQL 15 instance with private IP only.
- **Cloud Run**: A serverless container service for your backup application.
- **Cloud DNS**: A managed zone with weighted routing and health checks.
- **GCS Bucket**: For distributed locking and split-brain prevention.
- **Cloud Monitoring**: Alert policies for replication lag and failover events.

## Customization

- **Scaling**: Adjust `max_instance_count` in the compute module for Cloud Run.
- **Database Tier**: Change `database_tier` (default: `db-f1-micro`) for better performance.
- **Alerting**: Provide `monitoring_notification_email` to receive alert notifications.

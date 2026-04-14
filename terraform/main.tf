terraform {
  required_version = ">= 1.5.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

module "network" {
  source = "./modules/network"

  project_name = var.project_name
  region       = var.region
}

module "database" {
  source = "./modules/database"

  project_name         = var.project_name
  region               = var.region
  network_id           = module.network.network_id
  database_name        = var.database_name
  replicator_password  = var.replicator_password
  backup_start_time    = var.backup_start_time
  tier                 = var.database_tier
  deletion_protection  = var.database_deletion_protection
}

locals {
  database_url = "postgresql://${module.database.replicator_username}:${var.replicator_password}@${module.database.private_ip_address}:5432/${module.database.database_name}"
}

module "compute" {
  source = "./modules/compute"

  project_name        = var.project_name
  region              = var.region
  app_container_image = var.app_container_image
  database_url        = local.database_url
  network_name        = module.network.network_name
  subnet_name         = module.network.subnet_name
  service_account_email = google_service_account.cloud_run.email
}

module "dns" {
  source = "./modules/dns"

  project_name  = var.project_name
  domain        = var.domain
  local_app_ip  = var.local_app_ip
  backup_app_ip = var.backup_app_ip
}

module "storage" {
  source = "./modules/storage"

  bucket_name   = var.lock_bucket_name
  location      = var.region
  storage_class = var.lock_bucket_storage_class
  force_destroy = var.lock_bucket_force_destroy
}

resource "google_service_account" "state_sync_engine" {
  account_id   = "${replace(var.project_name, "_", "-")}-state-sync"
  display_name = "State Sync Engine Service Account"
}

resource "google_project_iam_member" "state_sync_cloud_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.state_sync_engine.email}"
}

resource "google_project_iam_member" "state_sync_storage_object_admin" {
  project = var.project_id
  role    = "roles/storage.objectAdmin"
  member  = "serviceAccount:${google_service_account.state_sync_engine.email}"
}

resource "google_service_account" "cloud_run" {
  account_id   = "${replace(var.project_name, "_", "-")}-cloud-run"
  display_name = "Cloud Run Backup Service Account"
}

resource "google_project_iam_member" "cloud_run_cloud_sql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.cloud_run.email}"
}

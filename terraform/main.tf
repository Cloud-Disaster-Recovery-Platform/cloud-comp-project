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

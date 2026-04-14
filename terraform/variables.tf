variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "project_name" {
  description = "Project name prefix for resource naming."
  type        = string
}

variable "region" {
  description = "Primary GCP region."
  type        = string
}

variable "database_name" {
  description = "Application database name."
  type        = string
}

variable "replicator_password" {
  description = "Password for Cloud SQL replicator user."
  type        = string
  sensitive   = true
}

variable "backup_start_time" {
  description = "Cloud SQL backup start time in HH:MM format (UTC)."
  type        = string
  default     = "03:00"
}

variable "database_tier" {
  description = "Cloud SQL machine tier."
  type        = string
  default     = "db-f1-micro"
}

variable "database_deletion_protection" {
  description = "Whether Cloud SQL deletion protection is enabled."
  type        = bool
  default     = true
}

variable "app_container_image" {
  description = "Container image to deploy on Cloud Run."
  type        = string
}

variable "domain" {
  description = "DNS domain for failover managed zone, without trailing dot."
  type        = string
}

variable "local_app_ip" {
  description = "IPv4 address of the local application endpoint."
  type        = string
}

variable "backup_app_ip" {
  description = "IPv4 address of the cloud backup endpoint."
  type        = string
}

variable "lock_bucket_name" {
  description = "Name of the GCS bucket used for distributed locks."
  type        = string
}

variable "lock_bucket_storage_class" {
  description = "Storage class for the lock bucket."
  type        = string
  default     = "STANDARD"
}

variable "lock_bucket_force_destroy" {
  description = "Whether lock bucket objects should be force-deleted."
  type        = bool
  default     = false
}

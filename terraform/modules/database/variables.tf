variable "project_name" {
  description = "Project name prefix for resource naming."
  type        = string
}

variable "region" {
  description = "GCP region where Cloud SQL is created."
  type        = string
}

variable "network_id" {
  description = "VPC network self_link/id used for private IP connectivity."
  type        = string
}

variable "database_name" {
  description = "Application database name."
  type        = string
}

variable "replicator_password" {
  description = "Password for the replicator database user."
  type        = string
  sensitive   = true
}

variable "backup_start_time" {
  description = "Backup start time in HH:MM format (UTC)."
  type        = string
  default     = "03:00"
}

variable "tier" {
  description = "Cloud SQL machine tier."
  type        = string
  default     = "db-f1-micro"
}

variable "deletion_protection" {
  description = "Whether deletion protection is enabled for the Cloud SQL instance."
  type        = bool
  default     = true
}

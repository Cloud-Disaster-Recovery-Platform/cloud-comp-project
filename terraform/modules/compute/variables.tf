variable "project_name" {
  description = "Project name prefix for resource naming."
  type        = string
}

variable "region" {
  description = "GCP region where Cloud Run is created."
  type        = string
}

variable "app_container_image" {
  description = "Container image for the backup application."
  type        = string
}

variable "database_url" {
  description = "Database URL passed to the container as DATABASE_URL."
  type        = string
  sensitive   = true
}

variable "network_name" {
  description = "VPC network name used for Cloud Run VPC access."
  type        = string
}

variable "subnet_name" {
  description = "Subnet name used for Cloud Run VPC access."
  type        = string
}

variable "service_account_email" {
  description = "Service account email for Cloud Run runtime identity."
  type        = string
}

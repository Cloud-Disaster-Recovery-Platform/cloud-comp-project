variable "project_name" {
  description = "Project name prefix for resource naming."
  type        = string
}

variable "domain" {
  description = "DNS domain for the managed zone, without trailing dot."
  type        = string
}

variable "local_app_ip" {
  description = "IPv4 address of the local application endpoint."
  type        = string
}

variable "backup_app_ip" {
  description = "IPv4 address of the backup cloud endpoint."
  type        = string
}

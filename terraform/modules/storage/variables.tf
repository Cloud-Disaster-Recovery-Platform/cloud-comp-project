variable "bucket_name" {
  description = "Name of the GCS bucket used for distributed locks."
  type        = string
}

variable "location" {
  description = "Bucket location/region."
  type        = string
}

variable "storage_class" {
  description = "GCS storage class."
  type        = string
  default     = "STANDARD"
}

variable "force_destroy" {
  description = "Whether to force destroy bucket objects on delete."
  type        = bool
  default     = false
}

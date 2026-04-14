resource "google_storage_bucket" "locks" {
  name     = var.bucket_name
  location = var.location

  uniform_bucket_level_access = true
  force_destroy               = var.force_destroy
  storage_class               = var.storage_class
}

output "bucket_name" {
  description = "Name of the lock bucket."
  value       = google_storage_bucket.locks.name
}

output "bucket_url" {
  description = "URL of the lock bucket."
  value       = google_storage_bucket.locks.url
}

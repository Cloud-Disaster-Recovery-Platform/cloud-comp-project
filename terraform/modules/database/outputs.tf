output "instance_name" {
  description = "Cloud SQL instance name."
  value       = google_sql_database_instance.backup.name
}

output "private_ip_address" {
  description = "Private IP address of the Cloud SQL instance."
  value       = google_sql_database_instance.backup.private_ip_address
}

output "connection_name" {
  description = "Cloud SQL connection name (<project>:<region>:<instance>)."
  value       = google_sql_database_instance.backup.connection_name
}

output "database_name" {
  description = "Application database name."
  value       = google_sql_database.app_db.name
}

output "replicator_username" {
  description = "Replicator database username."
  value       = google_sql_user.replicator.name
}

output "network_id" {
  description = "VPC network ID."
  value       = module.network.network_id
}

output "subnet_id" {
  description = "Subnet ID."
  value       = module.network.subnet_id
}

output "cloud_sql_private_ip_address" {
  description = "Cloud SQL private IP address."
  value       = module.database.private_ip_address
}

output "cloud_sql_connection_name" {
  description = "Cloud SQL connection name."
  value       = module.database.connection_name
}

output "cloud_run_service_uri" {
  description = "Cloud Run backup service URI."
  value       = module.compute.service_uri
}

output "dns_nameservers" {
  description = "Managed zone authoritative nameservers."
  value       = module.dns.nameservers
}

output "lock_bucket_name" {
  description = "Distributed lock bucket name."
  value       = module.storage.bucket_name
}

output "lock_bucket_url" {
  description = "Distributed lock bucket URL."
  value       = module.storage.bucket_url
}

output "replication_lag_alert_policy_id" {
  description = "Cloud Monitoring alert policy ID for replication lag threshold."
  value       = google_monitoring_alert_policy.replication_lag.id
}

output "failover_events_alert_policy_id" {
  description = "Cloud Monitoring alert policy ID for failover event detection."
  value       = google_monitoring_alert_policy.failover_events.id
}

output "monitoring_notification_channel_ids" {
  description = "Notification channels attached to monitoring alert policies."
  value       = local.monitoring_notification_channel_ids
}

output "state_sync_service_account_email" {
  description = "Service account email for the State Sync Engine."
  value       = google_service_account.state_sync_engine.email
}

output "cloud_run_service_account_email" {
  description = "Service account email used by Cloud Run backup service."
  value       = google_service_account.cloud_run.email
}

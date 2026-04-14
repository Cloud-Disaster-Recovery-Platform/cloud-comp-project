output "zone_name" {
  description = "Managed DNS zone name."
  value       = google_dns_managed_zone.failover.name
}

output "nameservers" {
  description = "Authoritative nameservers for the managed zone."
  value       = google_dns_managed_zone.failover.name_servers
}

output "health_check_id" {
  description = "Health check resource ID."
  value       = google_compute_health_check.local_app.id
}

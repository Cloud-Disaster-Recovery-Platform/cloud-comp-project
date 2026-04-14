resource "google_dns_managed_zone" "failover" {
  name        = "${var.project_name}-zone"
  dns_name    = "${var.domain}."
  description = "Failover DNS zone"
}

resource "google_compute_health_check" "local_app" {
  name                = "${var.project_name}-health"
  check_interval_sec  = 10
  timeout_sec         = 5
  healthy_threshold   = 2
  unhealthy_threshold = 3

  https_health_check {
    port         = 443
    request_path = "/health"
  }
}

resource "google_dns_record_set" "app" {
  name         = "app.${var.domain}."
  type         = "A"
  ttl          = 60
  managed_zone = google_dns_managed_zone.failover.name

  routing_policy {
    wrr {
      weight  = 1.0
      rrdatas = [var.local_app_ip]
    }

    wrr {
      weight  = 0.0
      rrdatas = [var.backup_app_ip]
    }
  }
}

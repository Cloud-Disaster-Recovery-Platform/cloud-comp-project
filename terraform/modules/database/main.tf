resource "google_sql_database_instance" "backup" {
  name             = "${var.project_name}-db"
  database_version = "POSTGRES_15"
  region           = var.region

  settings {
    tier = var.tier

    ip_configuration {
      ipv4_enabled    = false
      private_network = var.network_id
    }

    backup_configuration {
      enabled                        = true
      start_time                     = var.backup_start_time
      point_in_time_recovery_enabled = true
    }
  }

  deletion_protection = var.deletion_protection
}

resource "google_sql_database" "app_db" {
  name     = var.database_name
  instance = google_sql_database_instance.backup.name
}

resource "google_sql_user" "replicator" {
  name     = "replicator"
  instance = google_sql_database_instance.backup.name
  password = var.replicator_password
}

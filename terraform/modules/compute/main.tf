resource "google_cloud_run_v2_service" "backup_app" {
  name     = "${var.project_name}-app"
  location = var.region

  template {
    scaling {
      min_instance_count = 0
      max_instance_count = 10
    }

    containers {
      image = var.app_container_image

      env {
        name  = "DATABASE_URL"
        value = var.database_url
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "512Mi"
        }
      }
    }

    vpc_access {
      network_interfaces {
        network    = var.network_name
        subnetwork = var.subnet_name
      }
    }
  }
}

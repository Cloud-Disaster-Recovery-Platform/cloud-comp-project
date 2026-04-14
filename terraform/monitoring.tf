resource "google_monitoring_notification_channel" "alert_email" {
  count = var.monitoring_notification_email == "" ? 0 : 1

  display_name = "${var.project_name}-alerts-email"
  type         = "email"

  labels = {
    email_address = var.monitoring_notification_email
  }
}

locals {
  monitoring_notification_channel_ids = concat(
    var.monitoring_notification_channel_ids,
    [for ch in google_monitoring_notification_channel.alert_email : ch.id]
  )
}

resource "google_monitoring_alert_policy" "replication_lag" {
  display_name = "${var.project_name} - Replication Lag High"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "Replication lag above threshold"

    condition_threshold {
      filter          = "metric.type=\"prometheus.googleapis.com/state_sync_replication_lag_seconds/gauge\" AND resource.type=\"prometheus_target\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.replication_lag_alert_threshold_seconds
      duration        = "120s"

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MAX"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.monitoring_notification_channel_ids

  documentation {
    mime_type = "text/markdown"
    content   = "Replication lag exceeded configured threshold. Validate local replication stream health and cloud database connectivity."
  }
}

resource "google_monitoring_alert_policy" "failover_events" {
  display_name = "${var.project_name} - Failover Event Detected"
  combiner     = "OR"
  enabled      = true

  conditions {
    display_name = "Failover events increased"

    condition_threshold {
      filter          = "metric.type=\"prometheus.googleapis.com/state_sync_failover_events_total/counter\" AND resource.type=\"prometheus_target\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"

      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.monitoring_notification_channel_ids

  documentation {
    mime_type = "text/markdown"
    content   = "A failover event was detected. Check active node state, lock ownership, and application health endpoints."
  }
}

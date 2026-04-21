# GCP Cloud Monitoring Setup

Cloud Mirror integrates with GCP Cloud Monitoring to provide visibility into replication health and automatic failover events.

## Metrics Integration

The State Sync Engine exports Prometheus-compatible metrics. These are ingested into Google Cloud Monitoring via the **Managed Service for Prometheus**.

### Key Dashboards to Create

It is recommended to create a custom dashboard in the GCP Console with the following charts:

1. **Replication Lag**:
   - Metric: `prometheus.googleapis.com/state_sync_replication_lag_seconds/gauge`
   - Goal: Keep under 30 seconds.
2. **Failover Status**:
   - Metric: `prometheus.googleapis.com/state_sync_active_node/gauge`
   - 0 = Local, 1 = Cloud.
3. **Event Throughput**:
   - Metric: `prometheus.googleapis.com/state_sync_events_processed_total/counter`
   - Use Rate (e.g., `rate(5m)`) to see events per second.

## Alert Policies

The Terraform configuration automatically sets up the following alert policies:

### 1. High Replication Lag
- **Condition**: Triggers when `replication_lag_seconds` > 30s (configurable) for more than 2 minutes.
- **Action**: Check local database load and network connectivity between local and GCP.

### 2. Failover Event
- **Condition**: Triggers when the `failover_events_total` counter increments.
- **Action**: Immediately investigate why the local application failed. Verify that the Cloud Backup Node is serving traffic successfully.

## Notification Channels

To receive alerts, you must configure notification channels:

1. **Email**: Provide the `monitoring_notification_email` variable in Terraform.
2. **Other (Slack, PagerDuty)**: Manually create these in the GCP Console and provide their IDs in the `monitoring_notification_channel_ids` list variable.

## Viewing Metrics in GCP Console

1. Go to **Monitoring** > **Metrics Explorer**.
2. Search for `state_sync_` in the metric filter.
3. Group by `resource.type = "prometheus_target"` to see metrics from different engine instances.

## Troubleshooting Monitoring

- **No Metrics Visible**: Ensure the State Sync Engine can reach the Google Cloud Monitoring API (requires `roles/monitoring.metricWriter` permission on the service account).
- **Missing Alerts**: Verify that the alert policies are "Enabled" in the Monitoring > Alerting section of the GCP Console.

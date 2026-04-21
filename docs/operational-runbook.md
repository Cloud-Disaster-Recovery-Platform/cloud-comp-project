# Operational Runbook: Cloud Mirror

This runbook provides procedures for monitoring, troubleshooting, and managing the Cloud Mirror failsafe infrastructure.

## System Monitoring

### 1. Key Metrics

Monitor the following Prometheus metrics (exposed by the State Sync Engine on port 9090):

- `state_sync_replication_lag_seconds`: High lag (> 60s) indicates replication bottlenecks or connectivity issues.
- `state_sync_events_failed_total`: Non-zero values indicate errors in publishing to the cloud database.
- `state_sync_active_node`: 0 = Local active, 1 = Cloud active. Monitor for unexpected transitions.
- `state_sync_lock_held`: 1 = Current instance holds the writer lock.

### 2. Status Endpoint

Check the JSON status endpoint for a high-level overview:
```bash
curl http://localhost:8080/status
```

### 3. Log Analysis

The State Sync Engine logs in JSON format. Key messages to search for:
- `Failover detected`: Transition to cloud active.
- `Recovery detected`: Transition back to local active.
- `Split-brain detected`: Critical error where multiple nodes attempt to write.
- `Circuit breaker opened`: Cloud database is unreachable.

## Troubleshooting

### Replication Lag Increasing

1. **Check Connectivity**: Verify the local engine can reach the Cloud SQL private IP.
2. **Check Cloud DB Load**: High CPU or memory usage on Cloud SQL can slow down ingestion.
3. **Database Locks**: Ensure no long-running transactions on the local database are blocking the replication slot.
4. **Log Errors**: Check for `Publish failed` messages in the engine logs.

### Failover Not Triggering

1. **Check Health Check**: Verify the local `/health` endpoint is returning `200 OK`.
2. **DNS Propagation**: Check Cloud DNS logs for health check status transitions.
3. **Weights**: Verify that the DNS record weights are correctly configured (Local: 1.0, Cloud: 0.0 by default).

### Split-Brain Condition

If `state_sync_split_brain_events_total` increases:
1. **AUTHORITATIVE ACTION**: The Cloud Backup Node is considered authoritative.
2. **Read-Only Local**: Manually set the local database to read-only:
   ```sql
   ALTER DATABASE myapp SET default_transaction_read_only = on;
   ```
3. **Analyze Logs**: Compare timestamps of writes on both local and cloud databases.
4. **Manual Reconciliation**: Manually sync any records that were written to local but not replicated before the split-brain.

## Manual Procedures

### Triggering Manual Failover

To force traffic to the cloud:
1. Stop the local application or block its health check port.
2. Alternatively, update the Cloud DNS record weights via Terraform or GCP Console.

### Resuming After Failover (Recovery)

1. Ensure the local application is healthy.
2. The State Sync Engine should automatically detect recovery and sync changes back.
3. **Verification**: Confirm `state_sync_replication_lag_seconds` is near zero before releasing the lock.
4. Release the lock if stuck: Delete the `active-writer` object in the GCS lock bucket.

### Database Maintenance

When performing maintenance on the local database:
1. Stop the State Sync Engine to prevent replication errors.
2. Perform maintenance.
3. Restart the engine; it will resume from the last saved LSN in the replication slot.

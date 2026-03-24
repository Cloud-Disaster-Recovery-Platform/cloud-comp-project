# Implementation Plan: Local-First Distributed System

## Overview

This implementation plan creates a headless failsafe infrastructure that provides 100% uptime with zero cloud costs during idle states. The system consists of three main components: a State Sync Engine (Go service), Cloud Backup Node (GCP infrastructure), and Health Monitoring/Failover (DNS-based). The implementation follows an incremental approach, building core functionality first, then adding observability, error handling, and operational features.

## Tasks

- [x] 1. Project setup and core interfaces
  - Create Go module structure with proper directory layout
  - Define core interfaces: ReplicationSubscriber, ReplicationPublisher, DistributedLock, StateCoordinator
  - Set up dependency management (go.mod) with required libraries: pgx/v5, zap, viper, prometheus client
  - Create configuration schema structs matching design specification
  - _Requirements: 10.1, 10.2, 10.5_

- [ ] 2. Configuration management implementation
  - [ ] 2.1 Implement configuration loading from YAML/JSON and environment variables
    - Use viper library to load config.yaml and override with env vars
    - Implement Config struct unmarshaling with validation
    - Support nested configuration for database, replication, lock, health, failover sections
    - _Requirements: 10.1, 10.2, 10.3, 10.4_
  
  - [ ] 2.2 Implement configuration validation at startup
    - Validate required fields: local_database, cloud_database, replication.tables
    - Check for missing credentials and connection parameters
    - Exit with descriptive error messages when validation fails
    - _Requirements: 10.5, 10.6_
  
  - [ ]* 2.3 Write unit tests for configuration loading
    - Test YAML parsing, env var overrides, validation errors
    - Test missing required fields produce correct error messages
    - _Requirements: 10.1, 10.2, 10.5, 10.6_

- [ ] 3. PostgreSQL logical replication subscriber
  - [ ] 3.1 Implement ReplicationSubscriber interface using pgx/v5
    - Implement Connect() to establish replication connection
    - Create replication slot if it doesn't exist
    - Handle SSL/TLS connection modes per configuration
    - _Requirements: 3.2, 3.3, 8.6, 13.2_
  
  - [ ] 3.2 Implement Subscribe() to consume change events
    - Start logical replication stream using pgoutput plugin
    - Parse BEGIN, RELATION, INSERT, UPDATE, DELETE, COMMIT messages
    - Convert pgoutput messages to ChangeEvent structs
    - Send ChangeEvents to output channel
    - _Requirements: 3.3, 3.4, 8.1, 8.2, 8.3_
  
  - [ ] 3.3 Implement Acknowledge() for LSN confirmation
    - Send standby status update with confirmed LSN
    - Track last acknowledged LSN for resume capability
    - _Requirements: 3.3, 14.6_
  
  - [ ] 3.4 Implement graceful Close() with connection cleanup
    - Stop consuming replication stream
    - Close database connection properly
    - _Requirements: 12.4_
  
  - [ ]* 3.5 Write unit tests for replication subscriber
    - Test connection establishment, slot creation
    - Test message parsing for INSERT/UPDATE/DELETE operations
    - Test LSN acknowledgment and tracking
    - _Requirements: 3.2, 3.3, 3.4, 8.1, 8.2_

- [ ] 4. Cloud database replication publisher
  - [ ] 4.1 Implement ReplicationPublisher interface
    - Implement Publish() to batch-insert changes to cloud database
    - Use pgx connection pool for cloud database
    - Execute INSERT/UPDATE/DELETE statements based on ChangeEvent.Operation
    - Handle transaction batching per configuration batch_size
    - _Requirements: 3.5, 8.1, 8.2, 8.3, 8.4_
  
  - [ ] 4.2 Implement HealthCheck() for cloud connectivity
    - Execute simple SELECT 1 query to verify connection
    - Return error if cloud database unreachable
    - _Requirements: 3.5, 14.1_
  
  - [ ]* 4.3 Write unit tests for replication publisher
    - Test batch publishing with mock database
    - Test transaction handling and error cases
    - Test health check connectivity validation
    - _Requirements: 3.5, 8.1, 8.2_

- [ ] 5. Distributed lock implementation using GCS
  - [ ] 5.1 Implement DistributedLock interface with GCS backend
    - Implement Acquire() using GCS object creation with DoesNotExist precondition
    - Store lock metadata: holder, acquired timestamp, expiration time
    - Return false if lock already held by another node
    - _Requirements: 6.1_
  
  - [ ] 5.2 Implement Renew() for lock lease extension
    - Update GCS object metadata with new expiration time
    - Verify current holder matches before renewal
    - Return error if not lock holder
    - _Requirements: 6.1_
  
  - [ ] 5.3 Implement Release() to give up lock
    - Delete GCS object to release lock
    - Handle case where lock already expired
    - _Requirements: 6.1_
  
  - [ ] 5.4 Implement GetHolder() to query current lock owner
    - Read GCS object metadata
    - Return holder identifier and expiration time
    - _Requirements: 6.1_
  
  - [ ]* 5.5 Write unit tests for distributed lock
    - Test acquire/renew/release cycle
    - Test concurrent acquisition attempts
    - Test lock expiration and TTL handling
    - _Requirements: 6.1_

- [ ] 6. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 7. State coordinator for read-only mode transitions
  - [ ] 7.1 Implement StateCoordinator interface
    - Implement SetReadOnly() to execute ALTER DATABASE SET default_transaction_read_only = on
    - Terminate active write transactions using pg_terminate_backend
    - _Requirements: 6.2_
  
  - [ ] 7.2 Implement SetReadWrite() to enable writes
    - Execute ALTER DATABASE SET default_transaction_read_only = off
    - Verify state transition completed
    - _Requirements: 6.3_
  
  - [ ] 7.3 Implement GetState() to query current database mode
    - Execute SHOW default_transaction_read_only
    - Parse result and return DatabaseState enum
    - _Requirements: 6.2, 6.3_
  
  - [ ]* 7.4 Write unit tests for state coordinator
    - Test read-only mode activation
    - Test read-write mode restoration
    - Test state query accuracy
    - _Requirements: 6.2, 6.3_

- [ ] 8. Replication state persistence
  - [ ] 8.1 Implement ReplicationState struct serialization
    - Marshal ReplicationState to JSON format
    - Write to /var/lib/state-sync/replication.state file
    - Create directory if it doesn't exist
    - _Requirements: 12.6, 14.6_
  
  - [ ] 8.2 Implement state loading on startup
    - Read replication.state file if exists
    - Unmarshal JSON to ReplicationState struct
    - Resume replication from last LSN
    - _Requirements: 12.6, 14.6_
  
  - [ ]* 8.3 Write unit tests for state persistence
    - Test state save and load cycle
    - Test handling of missing state file (fresh start)
    - Test LSN resume after restart
    - _Requirements: 12.6, 14.6_

- [ ] 9. Main replication engine orchestration
  - [ ] 9.1 Implement main replication loop
    - Subscribe to local database change events
    - Queue events in memory buffer (configurable size)
    - Flush batches to cloud database per flush_interval or batch_size
    - Acknowledge LSN after successful flush
    - Track replication lag by comparing event timestamp to current time
    - _Requirements: 3.3, 3.4, 3.5, 3.6, 3.7, 9.1, 9.2_
  
  - [ ] 9.2 Implement graceful shutdown handler
    - Listen for SIGTERM and SIGINT signals
    - Stop consuming new change events
    - Flush all queued events to cloud database
    - Persist replication state to disk
    - Close all database connections
    - Complete shutdown within 30 seconds or log warning
    - _Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6_
  
  - [ ]* 9.3 Write integration tests for replication engine
    - Test end-to-end replication of INSERT/UPDATE/DELETE
    - Test graceful shutdown with in-flight events
    - Test state persistence and resume
    - _Requirements: 3.3, 3.4, 3.5, 12.1, 12.2, 12.3_

- [ ] 10. Error recovery and retry logic
  - [ ] 10.1 Implement exponential backoff for database connections
    - Retry failed connections with exponential backoff (1s, 2s, 4s, 8s, max 60s)
    - Log connection failures with error details
    - _Requirements: 14.1, 3.8_
  
  - [ ] 10.2 Implement circuit breaker for cloud database
    - Track consecutive failures to cloud database
    - Open circuit after 5 consecutive failures
    - Reduce retry frequency when circuit open (every 5 minutes)
    - Close circuit on successful connection
    - _Requirements: 14.3, 14.4_
  
  - [ ] 10.3 Implement retry logic for replication errors
    - Retry failed Publish() operations with backoff
    - Queue events when cloud unavailable
    - Resume normal operation when cloud restored
    - _Requirements: 3.6, 14.2, 14.5_
  
  - [ ]* 10.4 Write unit tests for error recovery
    - Test exponential backoff timing
    - Test circuit breaker state transitions
    - Test event queuing during cloud outage
    - _Requirements: 14.1, 14.2, 14.3, 14.4, 14.5_

- [ ] 11. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 12. Observability: structured logging
  - [ ] 12.1 Implement JSON structured logging with zap
    - Configure zap logger with JSON encoder
    - Log all state transitions: failover events, active node changes, replication status
    - Log errors with context: connection failures, replication errors, lock conflicts
    - Include correlation IDs for request tracing
    - _Requirements: 11.1, 11.2_
  
  - [ ]* 12.2 Write unit tests for logging
    - Test log output format is valid JSON
    - Test log levels and message content
    - _Requirements: 11.1, 11.2_

- [ ] 13. Observability: Prometheus metrics
  - [ ] 13.1 Implement Prometheus metrics endpoint
    - Expose /metrics endpoint on configured metrics_port
    - Implement all metrics from design: replication_lag_seconds, events_processed_total, events_failed_total, batch_size, flush_duration_seconds
    - Implement connection metrics: db_connections_active, db_connection_errors_total
    - Implement failover metrics: active_node, failover_events_total, split_brain_events_total
    - Implement lock metrics: lock_held, lock_acquire_duration_seconds
    - _Requirements: 11.3, 11.4_
  
  - [ ] 13.2 Update replication engine to record metrics
    - Increment counters on events processed/failed
    - Record histogram for flush duration
    - Update gauge for replication lag
    - _Requirements: 11.4, 3.7_
  
  - [ ]* 13.3 Write unit tests for metrics
    - Test metric registration and updates
    - Test metric values reflect actual operations
    - _Requirements: 11.3, 11.4_

- [ ] 14. Observability: status endpoint
  - [ ] 14.1 Implement /status HTTP endpoint
    - Expose endpoint on configured status_port
    - Return JSON with current ReplicationState
    - Include: active_node, replication_lag, lock_holder, last_failover_time, events_processed
    - Return 200 OK if healthy, 503 if unhealthy
    - _Requirements: 11.5_
  
  - [ ]* 14.2 Write unit tests for status endpoint
    - Test JSON response format
    - Test health status codes
    - _Requirements: 11.5_

- [ ] 15. Failover coordination logic
  - [ ] 15.1 Implement failover detection and coordination
    - Periodically check distributed lock holder
    - Detect when cloud becomes active (lock holder changes)
    - Coordinate setting local database to read-only when cloud active
    - Handle case where local database unreachable during failover
    - _Requirements: 5.4, 5.5, 6.2, 6.4_
  
  - [ ] 15.2 Implement recovery coordination
    - Detect when local application recovers (health checks pass)
    - Synchronize cloud-side changes back to local database
    - Verify local database is current before switching traffic
    - Release distributed lock after recovery complete
    - _Requirements: 5.7, 6.3_
  
  - [ ] 15.3 Implement split-brain detection and logging
    - Detect when both nodes think they are active
    - Log split-brain events with timestamps and lock holder info
    - Designate cloud as authoritative per policy
    - Increment split_brain_events_total metric
    - _Requirements: 6.4, 6.5_
  
  - [ ]* 15.4 Write integration tests for failover coordination
    - Test failover from local to cloud
    - Test recovery from cloud to local
    - Test split-brain detection and resolution
    - _Requirements: 5.4, 5.5, 5.7, 6.2, 6.3, 6.4, 6.5_

- [ ] 16. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 17. Terraform modules for GCP infrastructure
  - [ ] 17.1 Create network module
    - Define google_compute_network resource for VPC
    - Define google_compute_subnetwork with 10.0.0.0/24 CIDR
    - Create variables.tf for project_name, region
    - Create outputs.tf for network ID and subnet ID
    - _Requirements: 4.2_
  
  - [ ] 17.2 Create database module
    - Define google_sql_database_instance with POSTGRES_15
    - Configure private IP in VPC (no public IP)
    - Define google_sql_database for application database
    - Define google_sql_user for replicator user
    - Configure backup settings: enabled, start_time, point_in_time_recovery
    - Create variables.tf for database_name, replicator_password
    - Create outputs.tf for private_ip_address, connection_name
    - _Requirements: 4.3, 8.1, 8.2, 8.3, 8.4_
  
  - [ ] 17.3 Create compute module for Cloud Run
    - Define google_cloud_run_v2_service resource
    - Configure scaling: min_instances=0, max_instances=10
    - Configure container with app_container_image variable
    - Set DATABASE_URL environment variable from database module output
    - Configure VPC access to connect to Cloud SQL
    - Set resource limits: 1 CPU, 512Mi memory
    - Create variables.tf for app_container_image, database_url
    - Create outputs.tf for service_uri
    - _Requirements: 4.4, 4.6, 4.7_
  
  - [ ] 17.4 Create DNS module for health checks and failover
    - Define google_dns_managed_zone resource
    - Define google_dns_record_set with routing_policy for weighted routing
    - Configure primary route to local application (weight 1.0)
    - Configure backup route to Cloud Run (weight 0.0)
    - Define google_compute_health_check for local application
    - Configure health check: interval 10s, timeout 5s, path /health
    - Set healthy_threshold=2, unhealthy_threshold=3
    - Create variables.tf for domain, local_app_ip
    - Create outputs.tf for nameservers
    - _Requirements: 4.5, 5.1, 5.2, 5.3, 5.4, 5.8_
  
  - [ ] 17.5 Create storage module for distributed locks
    - Define google_storage_bucket for lock storage
    - Configure bucket location and storage class
    - Set uniform bucket-level access
    - Create variables.tf for bucket_name
    - Create outputs.tf for bucket_name, bucket_url
    - _Requirements: 6.1_
  
  - [ ] 17.6 Create root module composition
    - Create main.tf that instantiates all modules
    - Wire module outputs to inputs (e.g., network ID to database module)
    - Create variables.tf for all required inputs
    - Create outputs.tf aggregating all module outputs
    - Create terraform.tfvars.example with sample values
    - _Requirements: 4.1, 4.8_

- [ ] 18. Documentation and integration guides
  - [ ] 18.1 Create README for State Sync Engine
    - Document installation and setup instructions
    - Provide configuration examples for common scenarios
    - Document required PostgreSQL permissions for replication user
    - Explain how to create replication slot manually if needed
    - _Requirements: 1.4, 3.2, 8.6_
  
  - [ ] 18.2 Create Cloudflare Tunnel integration guide
    - Provide cloudflared configuration template
    - Document how to route tunnel traffic to local application
    - Specify health check endpoint requirements for local application
    - Document DNS configuration for failover routing
    - _Requirements: 2.1, 2.2, 2.3, 2.4_
  
  - [ ] 18.3 Create container packaging guide
    - Provide Dockerfile templates for Node.js, Python, Go applications
    - Document environment variable requirements for cloud deployment
    - Provide examples for connecting to Cloud SQL from containers
    - Document Cloud Run deployment requirements
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_
  
  - [ ] 18.4 Create Terraform deployment guide
    - Document prerequisites: GCP project, service account, APIs to enable
    - Provide step-by-step deployment instructions
    - Document how to customize terraform.tfvars for specific deployments
    - Explain output values and how to use them
    - _Requirements: 4.1, 4.8_
  
  - [ ] 18.5 Create operational runbook
    - Document how to monitor replication lag and system health
    - Provide troubleshooting guide for common issues
    - Document split-brain resolution procedures
    - Explain how to perform manual failover if needed
    - Document backup and recovery procedures
    - _Requirements: 11.1, 11.2, 11.3, 11.4, 11.5, 6.5, 6.6_

- [ ] 19. Integration with GCP Cloud Monitoring
  - [ ] 19.1 Configure Cloud Monitoring integration in Terraform
    - Add google_monitoring_alert_policy for replication lag threshold
    - Add alert policy for failover events
    - Configure notification channels for alerts
    - _Requirements: 11.6, 9.5_
  
  - [ ]* 19.2 Write documentation for Cloud Monitoring setup
    - Document how to view metrics in GCP console
    - Explain alert policies and notification configuration
    - _Requirements: 11.6_

- [ ] 20. Security hardening
  - [ ] 20.1 Implement TLS for cloud database connections
    - Configure pgx connection with sslmode=require
    - Validate TLS certificates for cloud connections
    - _Requirements: 13.1, 13.5_
  
  - [ ] 20.2 Configure IAM roles in Terraform
    - Create service account for State Sync Engine
    - Grant minimal permissions: Cloud SQL Client, Storage Object Admin (for locks)
    - Create service account for Cloud Run
    - Grant minimal permissions: Cloud SQL Client
    - _Requirements: 13.6_
  
  - [ ] 20.3 Document credential management
    - Document using environment variables for database passwords
    - Provide examples for GCP Secret Manager integration
    - Document Cloudflare Tunnel authentication
    - _Requirements: 13.3, 13.4_

- [ ] 21. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 22. Build and deployment artifacts
  - [ ] 22.1 Create Dockerfile for State Sync Engine
    - Multi-stage build with Go 1.21+ base image
    - Copy compiled binary and configuration
    - Set proper entrypoint and health check
    - _Requirements: 1.1, 1.2, 1.3_
  
  - [ ] 22.2 Create build scripts and CI configuration
    - Create Makefile for building Go binary
    - Create script for building and pushing Docker image
    - Provide GitHub Actions or GitLab CI example
    - _Requirements: 1.1, 1.2, 1.3_
  
  - [ ] 22.3 Create deployment scripts
    - Create script for deploying Terraform infrastructure
    - Create script for deploying State Sync Engine
    - Create script for initial database setup (replication slot creation)
    - _Requirements: 4.1, 1.4_

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation at key milestones
- The implementation uses Go for State Sync Engine and Terraform/HCL for infrastructure
- Testing tasks validate correctness of core functionality
- Documentation tasks ensure the system is usable by developers integrating with their existing applications

# Infrastructure Recommendations: 5 000 Hosts

Baseline measurements taken from a single-node deployment with 5 connected agents.
All projections are extrapolations — validate with load testing before production.

---

## Baseline (5 agents, measured)

| Metric | Value |
|---|---|
| CP process RSS | 31.5 MB |
| CP CPU (idle) | 3m cores |
| Go goroutines | 49 |
| gRPC active streams | 5 |
| DB pool (idle/acquired) | 3 / 0 |
| `/hosts` avg latency | 6 ms |
| `/audit-log` avg latency | 7.3 ms |
| `/hosts/:id/events` avg latency | 26 ms |
| Policy bundle push count | 0 (no pushes during observation) |

---

## Projections @ 5 000 Hosts

### Control Plane process

| Resource | Estimate | Notes |
|---|---|---|
| RSS | ~220 MB | ~40 KB per stream (goroutines + channels + TLS state) |
| Goroutines | ~15 000 | 3 per stream; Go scheduler handles this without issue |
| CPU (idle streams) | 0.5–1 core | Heartbeat writes dominate (167 writes/s) |
| CPU (1 000 events/s ingest) | 2–3 cores | pgx.CopyFrom batching, proto decode |
| CPU (policy fan-out burst) | 3–4 cores | Compile + sign + gRPC send for all hosts |
| File descriptors | 5 000+ | 1 per mTLS stream + HTTP + DB pool |

### PostgreSQL

| Workload | Rate | Notes |
|---|---|---|
| Heartbeat writes | 167 writes/s | `UPDATE hosts SET last_heartbeat_at` |
| Counter snapshots | 83 writes/s | INSERT into partitioned table |
| Log events (moderate) | 1 000 rows/s | `pgx.CopyFrom` batch, tolerable |
| Log events (storm) | 10 000 rows/s | Requires NVMe, WAL tuning |
| WAL throughput (moderate) | ~10–20 MB/s | |
| Expected CPU | 1.5–3 cores | |
| Expected RAM | 600–900 MB | shared_buffers + work_mem × connections |

---

## Minimum Hardware

### Control Plane node

```
CPU:  4 cores (x86_64)
RAM:  4 GB
Disk: 20 GB (binary + PKI + logs)
Net:  1 Gbps
```

A single replica handles 5 000 agents comfortably at this size.
For HA, run 2 replicas behind a TCP load balancer (see note on statefulness below).

### PostgreSQL node

```
CPU:  4–8 cores
RAM:  8 GB  (shared_buffers=2GB, effective_cache_size=6GB)
Disk: NVMe SSD, 200+ GB, separate volume for WAL
Net:  1 Gbps
```

Single PostgreSQL instance is sufficient for 5 000 hosts at moderate event rates.
At sustained >5 000 events/s consider read replicas or Citus for log_events partitions.

---

## Required Tuning

### OS — all CP nodes

```ini
# /etc/security/limits.d/qf.conf
qf soft nofile 65536
qf hard nofile 65536

# or systemd unit override
[Service]
LimitNOFILE=65536
```

Without this the default ulimit (1024) causes EMFILE at ~900 concurrent agents.

### PostgreSQL — postgresql.conf

```ini
# Memory
shared_buffers            = 2GB        # 25% of RAM
effective_cache_size      = 6GB
work_mem                  = 16MB
maintenance_work_mem      = 256MB

# WAL
wal_level                 = replica
max_wal_size              = 4GB
checkpoint_completion_target = 0.9
synchronous_commit        = off        # safe for telemetry; heartbeats tolerate loss

# Connections
max_connections           = 200
# Add PgBouncer in front when running 2+ CP replicas

# Autovacuum — critical for partitioned telemetry tables
autovacuum_vacuum_cost_delay  = 2ms
autovacuum_max_workers        = 4

# Logging (disable for perf, enable for debugging)
log_min_duration_statement    = 100    # ms; catches slow queries
```

### PostgreSQL — partition retention

Defaults in `cp/internal/ingest/partitions.go`:
```
log_events      7 days
flow_events    14 days
counter_snapshots  30 days
```

At 1 000 events/s sustained, `log_events` accumulates ~86 GB/day.
Tune TTL constants **before** first production deployment:

```go
// cp/internal/ingest/partitions.go
const (
    logRetentionDays     = 3   // reduce if disk is limited
    flowRetentionDays    = 7
    counterRetentionDays = 30
)
```

---

## Bundle Fan-Out Latency

When a policy changes, the cascade recompiler rebuilds and pushes bundles to all matching hosts.

| Hosts affected | Estimated propagation time | Notes |
|---|---|---|
| 50 | < 1 s | |
| 500 | 5–15 s | |
| 5 000 | 30–90 s | sequential compile; agents apply cached bundle in the meantime |

Agents continue enforcing the previous bundle until the new one arrives — firewall is never open.

To reduce propagation time at scale:
1. Scope policies narrowly (label selectors) so fewer hosts are affected per change.
2. Future: parallel compile worker pool in `CascadeRecompiler` (current implementation is sequential per host).

---

## High Availability

### CP replicas

gRPC agent streams are stateful (per-stream goroutine, registry entry). There is no shared stream state in the current implementation — a replica restart disconnects agents, which reconnect via `RunWithReconnect` (exponential backoff 1–60 s).

For 2-replica HA:
- Use a TCP load balancer with **source-IP hash** (not round-robin) to avoid per-request switching.
- Both replicas share the same PostgreSQL — no additional coordination needed.
- The PKI PVC must be `ReadWriteMany` (NFS or Longhorn) so both replicas access the same CA key.

```yaml
# values.yaml
replicaCount: 2
pki:
  persistentVolume:
    storageClass: longhorn    # or nfs
    accessMode: ReadWriteMany
    size: 1Gi
```

### PostgreSQL HA

For production use Patroni or CloudNativePG instead of Bitnami single-instance:
```yaml
# CloudNativePG example
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: qf-pg
spec:
  instances: 3
  storage:
    size: 200Gi
    storageClass: local-path
```

---

## Connection Pooling

At 2+ CP replicas each maintaining a pgxpool (default max 10 connections), PostgreSQL sees up to `replicas × 10 = 20` connections under load, which is fine.

If adding more replicas or microservices, add PgBouncer in transaction mode:

```yaml
# PgBouncer values
pgbouncer:
  poolMode: transaction
  maxClientConn: 500
  defaultPoolSize: 20
```

Update `QF_DB_DSN` to point at PgBouncer instead of PostgreSQL directly.

---

## Kubernetes Resource Requests and Limits

```yaml
# deploy/helm/qf-cp/values.yaml
resources:
  requests:
    cpu: "500m"
    memory: "256Mi"
  limits:
    cpu: "4000m"
    memory: "512Mi"
```

Do not set a tight memory limit — Go GC works best with headroom.
The limit prevents OOM on the node in pathological cases (event storm + concurrent fan-out).

---

## Monitoring Alerts (Prometheus)

Key metrics exported at `GET /metrics`:

| Metric | Alert threshold | Meaning |
|---|---|---|
| `qf_grpc_active_streams` | < expected × 0.9 | Mass agent disconnect |
| `qf_db_pool_acquired_conns` | > 8 (of 10) sustained | Pool exhaustion approaching |
| `qf_events_ingested_total` rate | drop to 0 for > 60s | Ingest pipeline stalled |
| `qf_bundle_push_duration_seconds` p99 | > 30s | Fan-out too slow |
| `go_goroutines` | > 20 000 | Goroutine leak |
| `process_resident_memory_bytes` | > 400 MB | Memory growth anomaly |

---

## Scaling Thresholds

| Agent count | Recommended configuration |
|---|---|
| < 500 | 1 CP replica, 2 CPU / 2 GB RAM, single PG node 4 CPU / 4 GB |
| 500–2 000 | 1–2 CP replicas, 4 CPU / 4 GB, PG 4 CPU / 8 GB NVMe |
| 2 000–5 000 | 2 CP replicas, 4 CPU / 4 GB each, PG 8 CPU / 16 GB NVMe, PgBouncer |
| > 5 000 | Horizontal CP sharding by tenant, PG read replicas for telemetry queries |

---

## What Does Not Scale (Known Limitations)

1. **Sequential bundle fan-out** — `CascadeRecompiler` iterates hosts one by one. At 5 000 hosts a global policy change takes 30–90 s.
2. **Single PostgreSQL** — write bottleneck at >10 000 events/s sustained. The partitioned schema is prepared for sharding but not implemented.
3. **In-process login rate limiter** — sliding window is per-replica in-memory. With 2+ replicas each allows 5 attempts/min independently (effective 10/min). Add Redis-backed limiter for strict enforcement.
4. **WebSocket hub is per-replica** — UI clients on replica A do not receive invalidation events triggered by a gRPC stream on replica B. Acceptable for current scale; fix with Redis pub/sub fan-out.

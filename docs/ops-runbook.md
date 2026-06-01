# qf Ops Runbook

Operational procedures for running qf in production.

---

## Table of Contents

1. [Backup and Restore](#1-backup-and-restore)
2. [Secret Rotation](#2-secret-rotation)
3. [Certificate Management](#3-certificate-management)
4. [PostgreSQL Maintenance](#4-postgresql-maintenance)
5. [Scaling qf-cp](#5-scaling-qf-cp)
6. [Debug Checklist](#6-debug-checklist)

---

## 1. Backup and Restore

### 1.1 Backup PKI directory

The PKI directory (`QF_PKI_DIR`, default `/etc/qf/pki`) contains:

| File | Description |
|---|---|
| `ca.crt` | CA certificate (public, safe to distribute) |
| `ca.key` | CA private key (AES-256-GCM encrypted with `QF_MASTER_KEY`) |
| `server.crt` / `server.key` | CP TLS server certificate and key |
| `bundle_signing.crt` / `bundle_signing.key` | Ed25519 bundle signing key pair |

```bash
# Backup
PKI_DIR=/etc/qf/pki
BACKUP_FILE="qf-pki-$(date +%Y%m%d-%H%M%S).tar.gz"

tar -czf "$BACKUP_FILE" -C "$(dirname $PKI_DIR)" "$(basename $PKI_DIR)"
chmod 600 "$BACKUP_FILE"

# Store backup in secure location (encrypted at rest)
# The CA private key is already encrypted with QF_MASTER_KEY, but store backup securely anyway
```

> **Important:** Always back up PKI together with `QF_MASTER_KEY`. A PKI backup without the corresponding master key is unusable.

### 1.2 Restore PKI directory

```bash
BACKUP_FILE="qf-pki-20260101-120000.tar.gz"
TARGET_DIR=/etc/qf

tar -xzf "$BACKUP_FILE" -C "$TARGET_DIR"
chown -R root:root "$TARGET_DIR/pki"
chmod 700 "$TARGET_DIR/pki"
chmod 600 "$TARGET_DIR/pki"/*.key

# Restart CP with same QF_MASTER_KEY used when backup was taken
systemctl restart qf-cp
```

### 1.3 PostgreSQL backup

```bash
# Full dump
pg_dump -h localhost -U qf -d qf -F c -f "qf-db-$(date +%Y%m%d-%H%M%S).pgdump"

# Restore
pg_restore -h localhost -U qf -d qf --clean "qf-db-20260101-120000.pgdump"
```

---

## 2. Secret Rotation

### 2.1 Rotate `QF_MASTER_KEY`

`QF_MASTER_KEY` encrypts the CA private key at rest (`ca.key` file, AES-256-GCM). Rotation requires re-encrypting the CA key.

**Procedure:**

1. Generate new master key:
   ```bash
   NEW_KEY=$(openssl rand -hex 32)
   echo "New key: $NEW_KEY"   # store securely before proceeding
   ```

2. Stop qf-cp (prevents CA key reads mid-rotation):
   ```bash
   systemctl stop qf-cp
   # Or: kubectl -n qf scale deployment qf-cp --replicas=0
   ```

3. Re-encrypt CA key with new master key using the Go tool (build from source):
   ```bash
   # Decrypt with old key, re-encrypt with new key
   go run ./cp/cmd/rekey/ \
     --pki-dir /etc/qf/pki \
     --old-key "$OLD_MASTER_KEY" \
     --new-key "$NEW_KEY"
   ```
   > **Note:** `rekey` tool re-reads `ca.key`, decrypts with old key, re-encrypts with new key, writes back atomically.

4. Update the secret and restart:
   ```bash
   # Systemd: edit /etc/qf/cp.env or environment file
   sed -i "s/QF_MASTER_KEY=.*/QF_MASTER_KEY=$NEW_KEY/" /etc/qf/cp.env
   systemctl start qf-cp

   # Kubernetes: update Secret
   kubectl -n qf create secret generic qf-cp-secrets \
     --from-literal=QF_MASTER_KEY="$NEW_KEY" \
     --from-literal=QF_DB_DSN="..." \
     --dry-run=client -o yaml | kubectl apply -f -
   kubectl -n qf rollout restart deployment/qf-cp
   ```

5. Verify startup:
   ```bash
   journalctl -u qf-cp -n 20
   # Look for: "PKI initialized" without errors
   ```

**Impact:** Zero agent downtime. Agents use mTLS with certs already issued; CA key is only needed to sign new certs (enrollment). Enrollment is briefly unavailable during restart (~seconds).

### 2.2 Rotate `QF_JWT_SECRET`

JWT secret signs access and refresh tokens (HMAC-SHA256). Rotation invalidates all active sessions — users must log in again.

```bash
NEW_JWT=$(openssl rand -hex 32)

# Update and restart
sed -i "s/QF_JWT_SECRET=.*/QF_JWT_SECRET=$NEW_JWT/" /etc/qf/cp.env
systemctl restart qf-cp

# Kubernetes
kubectl -n qf patch secret qf-cp-secrets \
  --patch="{\"stringData\":{\"QF_JWT_SECRET\":\"$NEW_JWT\"}}"
kubectl -n qf rollout restart deployment/qf-cp
```

**Impact:** All browser sessions and API clients using JWT Bearer tokens are invalidated immediately. API tokens (opaque tokens stored in DB) are not affected.

### 2.3 Rotate bundle signing key

The bundle signing key (Ed25519) signs policy bundles delivered to agents. Rotation requires agents to receive the new public key before the old key is decommissioned.

**Procedure:**

1. Generate new key pair:
   ```bash
   openssl genpkey -algorithm ed25519 -out /etc/qf/pki/bundle_signing_new.key
   openssl pkey -in /etc/qf/pki/bundle_signing_new.key -pubout -out /etc/qf/pki/bundle_signing_new.pub
   ```

2. Configure CP to dual-sign with both keys (not yet implemented — workaround: rolling restart with new key, agents will re-enroll to get updated `bundle_signing.pub`).

3. Replace key files atomically:
   ```bash
   mv /etc/qf/pki/bundle_signing.key /etc/qf/pki/bundle_signing_old.key
   mv /etc/qf/pki/bundle_signing_new.key /etc/qf/pki/bundle_signing.key
   mv /etc/qf/pki/bundle_signing_new.pub /etc/qf/pki/bundle_signing.pub
   ```

4. Restart CP. New bundles will be signed with new key.

5. Re-enroll agents (they receive updated `bundle_signing.pub` during enrollment):
   ```bash
   # On each agent host: remove old PKI material and restart
   rm /etc/qf/agent.crt /etc/qf/agent.key /etc/qf/ca.crt /etc/qf/bundle_signing.pub
   # Set enrollment token in /etc/qf/agent.conf
   systemctl restart qf-agent
   ```

---

## 3. Certificate Management

### 3.1 Check certificate expiry

Agent certificates are issued during enrollment. Default validity: **1 year** (set in `pki/ca.go`).

```bash
# Check a specific agent cert
openssl x509 -in /etc/qf/agent.crt -noout -dates -subject

# List all certs and their expiry in the database
psql "$QF_DB_DSN" -c "
  SELECT h.name, c.serial, c.not_after, c.status
  FROM certificates c
  JOIN hosts h ON h.id = c.host_id
  WHERE c.status = 'active'
  ORDER BY c.not_after ASC;"
```

### 3.2 Agent re-enrollment (cert expired or revoked)

When an agent's certificate expires or is revoked, the CP rejects its mTLS connection. The agent enters **degraded** mode and must re-enroll.

**On the agent host:**

```bash
# 1. Create a new enrollment token via CP API (or UI)
TOKEN=$(curl -s -X POST http://cp.example.com:8080/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H 'Content-Type: application/json' \
  -d '{"label_template":{},"ttl_seconds":3600,"max_uses":1}' | jq -r .token)

# 2. On the agent: remove old cert material and set token
rm /etc/qf/agent.crt /etc/qf/agent.key
echo "QF_ENROLL_TOKEN=$TOKEN" >> /etc/qf/agent.conf

# 3. Restart agent — it will re-enroll and receive new cert
systemctl restart qf-agent
```

After re-enrollment, remove `QF_ENROLL_TOKEN` from `agent.conf` (the agent clears it automatically on success, but verify):

```bash
grep QF_ENROLL_TOKEN /etc/qf/agent.conf  # should be empty or absent
```

### 3.3 Revoke a certificate

```bash
# Via API: mark host as revoked
curl -s -X PATCH http://cp.example.com:8080/hosts/$HOST_ID \
  -H "Authorization: Bearer $JWT" \
  -H "X-Tenant-ID: $TENANT_ID" \
  -H 'Content-Type: application/json' \
  -d '{"status":"revoked"}'

# In the database (direct)
psql "$QF_DB_DSN" -c "
  UPDATE certificates SET status='revoked', revoked_at=NOW()
  WHERE host_id='<host-uuid>' AND status='active';"
```

The revocation blocklist is reloaded by CP automatically every 5 minutes; no restart required.

### 3.4 Rotate CP server TLS certificate

The CP server TLS cert (`server.crt`/`server.key`) secures HTTPS and gRPC connections.

```bash
# Stop CP, generate new cert signed by the existing CA
systemctl stop qf-cp

# The cert is regenerated automatically by LoadOrInit if missing
mv /etc/qf/pki/server.crt /etc/qf/pki/server.crt.bak
mv /etc/qf/pki/server.key /etc/qf/pki/server.key.bak

systemctl start qf-cp
# CP regenerates server.crt/server.key on startup if files are absent
```

---

## 4. PostgreSQL Maintenance

### 4.1 Partition management

Partitions are managed automatically by the built-in `PartitionManager` (runs in CP every 24 hours). Default retention:

| Table | Partition type | Retention |
|---|---|---|
| `log_events` | daily | 7 days |
| `flow_events` | daily | 14 days |
| `counter_snapshots` | weekly | 30 days |
| `system_events` | no partition (DELETE) | 30 days |

PartitionManager creates 7 days of future daily partitions and 2 weeks of future weekly partitions on each cycle.

**To manually extend partitions:**

```bash
psql "$QF_DB_DSN" <<'SQL'
-- Create next-day partition manually
DO $$
DECLARE
  d date := CURRENT_DATE + 8;  -- one day beyond normal lookahead
BEGIN
  EXECUTE format(
    'CREATE TABLE IF NOT EXISTS log_events_%s PARTITION OF log_events
     FOR VALUES FROM (''%s'') TO (''%s'')',
    to_char(d, 'YYYY_MM_DD'), d, d + 1
  );
END $$;
SQL
```

**To change retention:** The TTL constants are hardcoded in `cp/internal/ingest/partitions.go`. To change them, modify and rebuild qf-cp:

```go
const (
    ttlLog     = 14 * 24 * time.Hour  // was 7 days
    ttlFlow    = 30 * 24 * time.Hour  // was 14 days
    ttlCounter = 90 * 24 * time.Hour  // was 30 days
)
```

### 4.2 Monitor partition health

```bash
# List all partitions with row counts
psql "$QF_DB_DSN" -c "
  SELECT
    c.relname AS partition,
    pg_size_pretty(pg_relation_size(c.oid)) AS size,
    pg_stat_get_live_tuples(c.oid) AS live_rows
  FROM pg_inherits i
  JOIN pg_class c ON c.oid = i.inhrelid
  JOIN pg_class p ON p.oid = i.inhparent
  WHERE p.relname IN ('log_events','flow_events','counter_snapshots')
  ORDER BY c.relname;"
```

### 4.3 PostgreSQL tuning

For high-throughput deployments:

```sql
-- In postgresql.conf
shared_buffers = 256MB            -- 25% of RAM
work_mem = 16MB                   -- for complex queries
maintenance_work_mem = 128MB      -- for VACUUM / index builds
max_connections = 100             -- qf-cp uses pgxpool (default max 10)
wal_level = replica               -- if using streaming replication
checkpoint_completion_target = 0.9
random_page_cost = 1.1            -- for SSD storage
```

Run `VACUUM ANALYZE` weekly on high-write tables:

```bash
psql "$QF_DB_DSN" -c "VACUUM ANALYZE log_events, flow_events, system_events;"
```

---

## 5. Scaling qf-cp

qf-cp is stateless (state in PostgreSQL + PKI dir). Multiple replicas can run concurrently.

### 5.1 Horizontal scaling (Kubernetes)

```bash
kubectl -n qf scale deployment qf-cp --replicas=3
```

Requirements for multi-replica setup:
- **Shared PKI volume**: all replicas must mount the same `QF_PKI_DIR` (ReadWriteMany PVC, or pre-provisioned NFS/CephFS). The CA key is read-only after initial generation.
- **Same `QF_MASTER_KEY`**: must be identical across all replicas.
- **Same `QF_JWT_SECRET`**: must be identical for JWT validation to work across replicas.
- **Load balancer**: distribute gRPC (8443) and enrollment (8444) traffic across replicas; agents reconnect automatically on disconnect.

```yaml
# values.yaml override for 3 replicas
replicaCount: 3
pki:
  persistentVolume:
    storageClass: nfs-client   # must support ReadWriteMany
```

### 5.2 Vertical scaling

qf-cp is CPU-bound during bundle compilation (policy rule compilation). If `BundleApplied` latency is high:

- Increase CP CPU limits (Helm: `resources.limits.cpu`)
- Increase PostgreSQL connection pool: set `QF_DB_POOL_MAX` env var (not currently exposed; default pgxpool size is 10 — rebuild to change)

### 5.3 Monitoring key metrics

| Metric | Where | Alert threshold |
|---|---|---|
| `GET /healthz` response | HTTP | Non-200 → down |
| Agent connection count | gRPC port 8443 active connections | Drop >20% in 5min |
| Bundle delivery latency | CP logs: `"bundle pushed"` | p99 > 5s |
| PostgreSQL connections | `pg_stat_activity` | > 80% of `max_connections` |
| Partition missing | CP logs: `"partition manager: create failed"` | Any error |
| Disk usage (PKI dir) | Host / PVC | > 80% |

---

## 6. Debug Checklist

### Host stuck in `enrolling`

```bash
# 1. Agent logs
journalctl -u qf-agent -n 50 --no-pager

# 2. Network: enrollment port reachable from agent
nc -zv <cp-host> 8444

# 3. Token not expired
curl -s http://cp.example.com:8080/tokens \
  -H "Authorization: Bearer $JWT" -H "X-Tenant-ID: $TENANT_ID" | jq '.[] | {id, expires_at, uses_count, max_uses}'

# 4. TLS SAN mismatch: QF_CP_ENDPOINT on agent must match QF_CP_HOST on CP
# Agent connects to cp.example.com:8444 but cert SAN is for localhost → handshake failure
grep QF_CP_ENDPOINT /etc/qf/agent.conf
# Must match the hostname in CP's QF_CP_HOST env var

# 5. Firewall: check port 8444 open on CP host
ss -tlnp | grep 8444
```

### Bundle not arriving on agent

```bash
# 1. Agent gRPC connected?
journalctl -u qf-agent | grep -E "connected|bundle|stream"

# 2. Agent cert valid
openssl x509 -in /etc/qf/agent.crt -noout -dates
# if expired → re-enroll (see section 3.2)

# 3. CP pusher logs
# On CP host or pod
journalctl -u qf-cp | grep -E "bundle|push|stream" | tail -20
# kubectl -n qf logs deployment/qf-cp | grep bundle

# 4. Host status in DB
psql "$QF_DB_DSN" -c "SELECT id, name, status FROM hosts WHERE name='<hostname>';"
# If status='stale' or 'needs_rebootstrap' → re-enroll

# 5. Policy exists and compiles
curl -s http://cp.example.com:8080/policies \
  -H "Authorization: Bearer $JWT" -H "X-Tenant-ID: $TENANT_ID" | jq '.[] | {id, name}'
```

### Events not appearing in UI

```bash
# 1. Agent status active?
curl -s http://cp.example.com:8080/hosts \
  -H "Authorization: Bearer $JWT" -H "X-Tenant-ID: $TENANT_ID" | jq '.[] | {name, status}'

# 2. Agent sending events?
journalctl -u qf-agent | grep -E "event|batch|ingest"

# 3. CP ingest logs
journalctl -u qf-cp | grep -E "ingest|log_event|insert" | tail -20

# 4. PostgreSQL: rows in today's partition
psql "$QF_DB_DSN" -c "
  SELECT COUNT(*) FROM log_events
  WHERE created_at >= CURRENT_DATE AND host_id='<host-uuid>';"

# 5. Today's partition exists
psql "$QF_DB_DSN" -c "
  SELECT relname FROM pg_class
  WHERE relname = 'log_events_' || to_char(CURRENT_DATE, 'YYYY_MM_DD');"
# If missing: PartitionManager hasn't run yet; see section 4.1 to create manually
```

### TLS / mTLS errors

| Error | Likely cause | Fix |
|---|---|---|
| `certificate signed by unknown authority` | Agent has wrong `ca.crt` | Re-enroll agent |
| `certificate has expired` | Agent cert expired | Re-enroll agent (section 3.2) |
| `connection refused :8443` | CP gRPC not listening | Check CP startup, port binding |
| `remote error: tls: certificate required` | Agent didn't send cert | Check `agent.crt`/`agent.key` exist in `QF_PKI_DIR` |
| `no such host` | DNS for CP endpoint unresolvable | Check `QF_CP_ENDPOINT` in agent config |

### CP fails to start

```bash
journalctl -u qf-cp -n 30 --no-pager
```

Common errors:

| Error message | Cause | Fix |
|---|---|---|
| `QF_MASTER_KEY must be 32-byte hex` | Key missing or wrong length | Set correct 64-char hex key |
| `migrations: ...` | DB unreachable or permissions | Check `QF_DB_DSN`, PostgreSQL up |
| `listen ...: address already in use` | Port conflict | Check `ss -tlnp`, adjust `QF_HTTP_ADDR` / `QF_GRPC_ADDR` |
| `PKI initialized` not in logs | CA init failed | Check `QF_PKI_DIR` writable, disk space |

### High memory or CPU on CP

```bash
# Profile CP via pprof (if exposed)
curl http://cp.example.com:8080/debug/pprof/goroutine?debug=1

# Check goroutine count (should be stable under load)
# Check DB connection pool saturation: look for "acquire timeout" in logs
```

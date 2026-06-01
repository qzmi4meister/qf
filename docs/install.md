# qf Installation Guide

## Overview

qf consists of two components:

- **qf-cp** — control plane (Go binary or container); requires PostgreSQL
- **qf-agent** — firewall agent (Go binary, eBPF); runs on each protected Linux host

Three deployment scenarios are covered below.

---

## Scenario 1: Standalone Linux (binary)

Run qf-cp directly on a Linux server with PostgreSQL, then install qf-agent on managed hosts.

### 1.1 Prerequisites

- Linux x86_64 or arm64
- PostgreSQL 14+
- Go 1.22+ (to build from source) or pre-built binaries from GitHub Releases

### 1.2 Set up PostgreSQL

```bash
# Create database and user
psql -U postgres <<'SQL'
CREATE USER qf WITH PASSWORD 'changeme';
CREATE DATABASE qf OWNER qf;
SQL
```

### 1.3 Generate master key

```bash
QF_MASTER_KEY=$(openssl rand -hex 32)
echo "QF_MASTER_KEY=$QF_MASTER_KEY"  # save this securely
```

### 1.4 Build and run qf-cp

```bash
# Build
go build -o qf-cp ./cp/cmd/qf-cp/

# Run
export QF_DB_DSN="postgres://qf:changeme@localhost:5432/qf"
export QF_MASTER_KEY="<64-char hex from step 1.3>"
export QF_JWT_SECRET="$(openssl rand -hex 32)"
export QF_CP_HOST="cp.example.com"   # hostname agents will use in TLS SAN
export QF_HTTP_ADDR=":8080"
export QF_GRPC_ADDR=":8443"
export QF_ENROLL_ADDR=":8444"

./qf-cp
```

qf-cp auto-runs database migrations on startup. Logs go to stderr in JSON format.

**Environment variables:**

| Variable | Default | Description |
|---|---|---|
| `QF_DB_DSN` | — | PostgreSQL DSN (required) |
| `QF_MASTER_KEY` | — | 32-byte hex key for token encryption (required) |
| `QF_JWT_SECRET` | ephemeral | JWT signing key; set to persist sessions across restarts |
| `QF_CP_HOST` | `localhost` | Hostname included in CP TLS certificate SAN |
| `QF_CP_ENDPOINT` | `localhost:8444` | Enrollment endpoint advertised to agents |
| `QF_HTTP_ADDR` | `:8080` | REST API + UI listen address |
| `QF_GRPC_ADDR` | `:8443` | gRPC (mTLS) listen address for agents |
| `QF_ENROLL_ADDR` | `:8444` | Enrollment gRPC listen address (plain TLS) |
| `QF_PKI_DIR` | `/etc/qf/pki` | Directory where CA, server certs, bundle signing key are stored |
| `QF_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `QF_FORWARDER_DSN` | — | Optional syslog-compatible DSN for event forwarding |

### 1.5 Create enrollment token

Open the UI at `http://cp.example.com:8080` (redirects to `/app/`).

Or via API:

```bash
# Login to get JWT
TOKEN=$(curl -s -X POST http://cp.example.com:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | jq -r .token)

# Create bulk enrollment token (valid 24h, unlimited uses)
curl -s -X POST http://cp.example.com:8080/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: <tenant-uuid>" \
  -H 'Content-Type: application/json' \
  -d '{"label_template":{"env":"prod"},"ttl_seconds":86400,"max_uses":0}' | jq .
```

Save the returned `token` value — this is the enrollment token for agents.

### 1.6 Install qf-agent via deb/rpm

```bash
# On the managed host — Debian/Ubuntu
sudo dpkg -i qf-agent_1.0.0_amd64.deb

# Or RPM-based (RHEL/Fedora/Rocky)
sudo rpm -i qf-agent-1.0.0-1.x86_64.rpm
```

Configure the agent:

```bash
sudo nano /etc/qf/agent.conf
```

```ini
QF_CP_ENDPOINT=cp.example.com:8444
QF_IFACE=eth0
QF_PKI_DIR=/etc/qf
QF_LOG_LEVEL=info
```

### 1.7 Enroll the agent

The agent enrolls automatically on first start using the enrollment token:

```bash
# Set token for first run (written to PKI dir after enrollment)
echo "QF_ENROLL_TOKEN=<token-from-step-1.5>" | sudo tee -a /etc/qf/agent.conf

sudo systemctl start qf-agent
sudo systemctl status qf-agent
```

After successful enrollment, `QF_ENROLL_TOKEN` is no longer needed — the agent uses mTLS certificates stored in `QF_PKI_DIR`.

```bash
# Verify enrollment in CP
curl -s http://cp.example.com:8080/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: <tenant-uuid>" | jq '.[] | {id, name, status}'
```

The host status changes from `enrolling` → `active` once the first policy bundle is received.

---

## Scenario 2: k8s node — agent on node, CP in Kubernetes (Helm)

Run qf-cp in your cluster via Helm; install qf-agent as a systemd unit on each Kubernetes node.

### 2.1 Deploy qf-cp via Helm

```bash
# Add image (if using local build)
# Or pull from GHCR: ghcr.io/qf/qf-cp:<version>

helm install qf-cp deploy/helm/qf-cp/ \
  --namespace qf \
  --create-namespace \
  --set secrets.dbDSN="postgres://qf:changeme@postgres:5432/qf" \
  --set secrets.masterKey="$(openssl rand -hex 32)" \
  --set secrets.jwtSecret="$(openssl rand -hex 32)" \
  --set env.QF_CP_HOST="qf-cp.qf.svc.cluster.local" \
  --set env.QF_CP_ENDPOINT="qf-cp.qf.svc.cluster.local:8444"
```

Expose the enrollment port (8444) so nodes can reach it:

```bash
# Option A: NodePort (simple)
helm upgrade qf-cp deploy/helm/qf-cp/ \
  --set service.type=NodePort \
  --set service.enrollNodePort=30444

# Option B: LoadBalancer (cloud)
helm upgrade qf-cp deploy/helm/qf-cp/ --set service.type=LoadBalancer
```

Wait for pod to be ready:

```bash
kubectl -n qf rollout status deployment/qf-cp
```

### 2.2 Install qf-agent on each node

SSH to each Kubernetes node:

```bash
ssh user@node1.example.com

# Install package
sudo dpkg -i qf-agent_1.0.0_amd64.deb

# Configure
sudo tee /etc/qf/agent.conf <<'EOF'
QF_CP_ENDPOINT=<node-ip>:30444   # NodePort, or LoadBalancer IP:8444
QF_IFACE=eth0
QF_PKI_DIR=/etc/qf
QF_ENROLL_TOKEN=<enrollment-token>
QF_LOG_LEVEL=info
EOF

sudo systemctl enable --now qf-agent
```

### 2.3 Enable Ingress for UI (optional)

```bash
helm upgrade qf-cp deploy/helm/qf-cp/ \
  --set ingress.enabled=true \
  --set ingress.host=qf.example.com \
  --set ingress.tls=true
```

---

## Scenario 3: All-in-Kubernetes (CP + agent DaemonSet)

> **Note:** qf-agent as a Kubernetes DaemonSet requires privileged pods and host network access. This mode is suitable for cluster-internal network policy enforcement.

### 3.1 Deploy qf-cp

Same as Scenario 2, Step 2.1.

### 3.2 Deploy agent as DaemonSet (stub)

```yaml
# agent-daemonset.yaml — example stub, adapt for your environment
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: qf-agent
  namespace: qf
spec:
  selector:
    matchLabels:
      app: qf-agent
  template:
    metadata:
      labels:
        app: qf-agent
    spec:
      hostNetwork: true
      hostPID: true
      tolerations:
        - operator: Exists
      containers:
        - name: qf-agent
          image: <your-registry>/qf-agent:1.0.0  # build separately (eBPF needs CGO)
          securityContext:
            privileged: true
            capabilities:
              add: [NET_ADMIN, BPF, SYS_ADMIN]
          env:
            - name: QF_CP_ENDPOINT
              value: "qf-cp.qf.svc.cluster.local:8444"
            - name: QF_IFACE
              value: "eth0"
            - name: QF_ENROLL_TOKEN
              valueFrom:
                secretKeyRef:
                  name: qf-agent-token
                  key: token
          volumeMounts:
            - name: pki
              mountPath: /etc/qf
      volumes:
        - name: pki
          hostPath:
            path: /etc/qf
            type: DirectoryOrCreate
```

```bash
kubectl create secret generic qf-agent-token \
  --namespace qf \
  --from-literal=token=<enrollment-token>

kubectl apply -f agent-daemonset.yaml
```

---

## Troubleshooting

### Host stuck in `enrolling` status

1. Check agent logs: `journalctl -u qf-agent -n 50`
2. Verify CP reachable from agent host: `nc -zv <cp-host> 8444`
3. Check enrollment token not expired: `GET /tokens` via API
4. Verify `QF_CP_ENDPOINT` matches `QF_CP_HOST` set on CP (TLS SAN mismatch → handshake failure)

### Bundle not arriving on agent

1. Check agent gRPC connection: agent logs should show `"bundle received"` periodically
2. Check CP logs for errors on gRPC port 8443
3. Verify mTLS: agent certs in `QF_PKI_DIR` (`agent.crt`, `agent.key`, `ca.crt`) must exist and not be expired
4. Check host status in UI — if `stale` or `needs_rebootstrap`, re-enroll the agent

### Events not appearing in UI

1. Confirm agent `status = active` in hosts list
2. Check agent logs for `"event batch sent"` messages
3. Verify `QF_GRPC_ADDR` (port 8443) is reachable from agent host
4. Check CP event ingester logs for PostgreSQL write errors

### TLS handshake errors

- `certificate signed by unknown authority`: agent uses wrong `ca.crt` or CP re-generated CA; re-enroll agent
- `certificate has expired`: check cert expiry with `openssl x509 -in /etc/qf/agent.crt -noout -dates`; re-enroll to get new cert
- `connection refused` on port 8444: enrollment server not running; check CP startup logs

### systemd unit fails to start

```bash
journalctl -u qf-agent --no-pager -n 20
```

- `BPF load failed`: kernel too old (need 5.10+) or missing `CAP_BPF`; check kernel version with `uname -r`
- `config load failed`: check `/etc/qf/agent.conf` syntax (KEY=VALUE, no spaces around `=`)

### PostgreSQL connection errors (CP)

- Verify `QF_DB_DSN` is correct and PostgreSQL is reachable
- Check if migrations ran: CP logs `"migrations applied"` on startup
- Verify PostgreSQL user has `CREATE` permissions (needed for first migration)

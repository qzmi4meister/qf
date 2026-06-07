# qf — Deployment Guide

## Overview

| Component | What it is |
|---|---|
| **qf-cp** | Control plane — REST API, Web UI, gRPC server, PKI, policy compiler. Requires PostgreSQL. |
| **qf-agent** | Firewall agent — eBPF/TC datapath, runs on each protected Linux host. Requires kernel ≥ 5.15. |

---

## 1. Control Plane in Kubernetes (Helm)

### Prerequisites

- Kubernetes 1.24+
- PostgreSQL 14+ accessible from the cluster
- Helm 3.10+
- The CP needs a **PersistentVolume** for PKI storage (CA, certs, bundle signing key)

### 1.1 Prepare values file

Create `values-prod.yaml` (do not commit to git):

```yaml
image:
  repository: ghcr.io/qzmi4meister/qf-cp
  pullPolicy: Always

secrets:
  dbDSN: "postgres://qf:PASSWORD@postgres.qf.svc.cluster.local:5432/qf"
  masterKey: "GENERATE_WITH: openssl rand -hex 32"
  jwtSecret: "GENERATE_WITH: openssl rand -hex 32"
  adminEmail: "admin@example.com"
  adminPassword: "CHANGE_ME"

env:
  QF_CP_HOST: "qf.example.com"          # hostname in TLS SAN; agents connect using this name
  QF_CP_ENDPOINT: "qf.example.com:31444" # enrollment address advertised to agents
  QF_GRPC_ADDR: ":8443"
  QF_ENROLL_ADDR: ":8444"
  QF_HTTP_ADDR: ":8080"
  QF_PKI_DIR: /etc/qf/pki
  QF_LOG_LEVEL: info

pki:
  persistentVolume:
    storageClass: "longhorn"   # or your storage class; "" = cluster default
    size: 1Gi

service:
  type: ClusterIP   # or NodePort / LoadBalancer — see section 1.3

ingress:
  enabled: true
  className: "traefik"          # or nginx
  host: qf.example.com
  tls:
    - secretName: qf-cp-tls
      hosts:
        - qf.example.com
```

Generate secrets:

```bash
echo "masterKey: $(openssl rand -hex 32)"
echo "jwtSecret: $(openssl rand -hex 32)"
```

### 1.2 Install / upgrade

```bash
helm upgrade --install qf-cp \
  oci://ghcr.io/qzmi4meister/helm/qf-cp \
  --version 0.8.12 \
  --namespace qf --create-namespace \
  -f values-prod.yaml
```

Wait for the pod:

```bash
kubectl -n qf rollout status deployment/qf-cp
kubectl -n qf get pods
```

CP runs database migrations automatically on first start. Check logs:

```bash
kubectl -n qf logs -l app.kubernetes.io/name=qf-cp --tail=50
```

### 1.3 Expose agent ports

Agents need to reach **port 8444** (enrollment) and **port 8443** (gRPC stream) on the CP.
The Web UI (port 8080) is typically exposed via Ingress only.

**Option A — NodePort** (simplest for on-prem / bare-metal):

Add to values:

```yaml
service:
  type: NodePort
  grpcNodePort: 31443
  enrollNodePort: 31444
```

Then set in `env`:

```yaml
env:
  QF_CP_ENDPOINT: "<any-node-ip>:31444"
  QF_CP_HOST: "<any-node-ip-or-hostname>"
```

**Option B — LoadBalancer** (cloud):

```yaml
service:
  type: LoadBalancer
```

After `EXTERNAL-IP` is assigned, set `QF_CP_HOST` and `QF_CP_ENDPOINT` to that IP/hostname.

**Option C — Separate Ingress for gRPC** (advanced):
Use a TCP passthrough Ingress/Gateway for ports 8443 and 8444 alongside the HTTP Ingress for the UI.

### 1.4 Verify

```bash
# Health check
curl https://qf.example.com/healthz

# Login
curl -c cookies.txt -X POST https://qf.example.com/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"CHANGE_ME"}'

# List hosts (should be empty initially)
curl -b cookies.txt -H 'X-Tenant-ID: <tenant-id>' \
  https://qf.example.com/hosts | jq .
```

### 1.5 Upgrade CP

```bash
helm upgrade qf-cp \
  oci://ghcr.io/qzmi4meister/helm/qf-cp \
  --version NEW_VERSION \
  --namespace qf \
  -f values-prod.yaml \
  --set image.tag=NEW_VERSION
```

---

## 2. Control Plane on a Standalone Linux Machine

Run qf-cp directly as a systemd service on any Linux x86_64/arm64 server.

### 2.1 Prerequisites

- Linux x86_64 or arm64
- PostgreSQL 14+ (local or remote)
- Binary from GitHub Releases or built from source

### 2.2 Install PostgreSQL and create database

```bash
# Debian/Ubuntu
sudo apt install -y postgresql

sudo -u postgres psql <<'SQL'
CREATE USER qf WITH PASSWORD 'CHANGE_ME';
CREATE DATABASE qf OWNER qf;
SQL
```

### 2.3 Download binary

```bash
VERSION=0.8.12
curl -LO https://github.com/qzmi4meister/qf/releases/download/v${VERSION}/qf-cp_${VERSION}_linux_amd64.tar.gz
tar xzf qf-cp_${VERSION}_linux_amd64.tar.gz
sudo mv qf-cp /usr/local/bin/qf-cp
sudo chmod +x /usr/local/bin/qf-cp
```

Or build from source:

```bash
# Requires Go 1.22+, Node.js 18+
git clone https://github.com/qzmi4meister/qf && cd qf
make ui-build
go build -o /usr/local/bin/qf-cp ./cp/cmd/qf-cp/
```

### 2.4 Create configuration

```bash
sudo mkdir -p /etc/qf/pki
sudo tee /etc/qf/cp.env <<'EOF'
QF_DB_DSN=postgres://qf:CHANGE_ME@localhost:5432/qf
QF_MASTER_KEY=GENERATE_WITH_openssl_rand_-hex_32
QF_JWT_SECRET=GENERATE_WITH_openssl_rand_-hex_32
QF_CP_HOST=qf.example.com
QF_CP_ENDPOINT=qf.example.com:8444
QF_HTTP_ADDR=:8080
QF_GRPC_ADDR=:8443
QF_ENROLL_ADDR=:8444
QF_PKI_DIR=/etc/qf/pki
QF_LOG_LEVEL=info
QF_ADMIN_EMAIL=admin@example.com
QF_ADMIN_PASSWORD=CHANGE_ME
EOF
sudo chmod 600 /etc/qf/cp.env
```

Generate keys before editing:

```bash
openssl rand -hex 32   # paste as QF_MASTER_KEY
openssl rand -hex 32   # paste as QF_JWT_SECRET
```

### 2.5 Create systemd unit

```bash
sudo tee /etc/systemd/system/qf-cp.service <<'EOF'
[Unit]
Description=qf Control Plane
After=network.target postgresql.service
Requires=network.target

[Service]
Type=simple
User=root
EnvironmentFile=/etc/qf/cp.env
ExecStart=/usr/local/bin/qf-cp
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now qf-cp
sudo systemctl status qf-cp
```

### 2.6 Verify

```bash
# Check migrations ran
sudo journalctl -u qf-cp -n 30

# Health endpoint
curl http://localhost:8080/healthz

# Web UI
# http://qf.example.com:8080/app
```

### 2.7 Firewall

Open ports so agents can reach the CP:

```bash
# ufw (Debian/Ubuntu)
sudo ufw allow 8443/tcp comment "qf gRPC"
sudo ufw allow 8444/tcp comment "qf enrollment"
sudo ufw allow 8080/tcp comment "qf UI"
```

---

## 3. Agent: Deploy via Ansible

The Ansible role downloads the package from GitHub Releases, installs it, and starts/enables the systemd service. It supports both apt (Debian/Ubuntu) and dnf/yum (RHEL/Fedora/Rocky).

### 3.1 Configure inventory

Edit `deploy/ansible/inventory.yml`:

```yaml
all:
  children:
    qf_agents:
      hosts:
        web1:
          ansible_host: 192.168.1.10
        web2:
          ansible_host: 192.168.1.11
        db1:
          ansible_host: 192.168.1.20
      vars:
        ansible_user: root
        ansible_ssh_private_key_file: ~/.ssh/id_ed25519
```

### 3.2 Configure agent before deployment

The role installs the package but does **not** write `/etc/qf/agent.conf` — configure it on each host before or after deployment, or add a task to the role.

Minimal `/etc/qf/agent.conf` on each target host:

```ini
QF_CP_ENDPOINT=qf.example.com:8443
QF_ENROLL_ENDPOINT=qf.example.com:8444
QF_ENROLL_TOKEN=<token-from-cp-ui>
QF_IFACE=eth0
QF_PKI_DIR=/etc/qf
QF_LOG_LEVEL=info
```

To get an enrollment token: open the CP Web UI → **Tokens** → **New token** → copy the value.

Or via API:

```bash
curl -s -b cookies.txt -X POST https://qf.example.com/tokens \
  -H 'Content-Type: application/json' \
  -H 'X-Tenant-ID: <tenant-id>' \
  -d '{"label_template":{"env":"prod"},"ttl_seconds":86400,"max_uses":0}' | jq -r .token
```

### 3.3 Deploy

```bash
# Install pip dependencies (once)
pip install ansible

# Deploy to all hosts in qf_agents group
make deploy-agent
# or directly:
ansible-playbook -i deploy/ansible/inventory.yml deploy/ansible/deploy-agent.yml

# Deploy to a single host
make deploy-agent target=web1
# or:
ansible-playbook -i deploy/ansible/inventory.yml deploy/ansible/deploy-agent.yml --limit web1
```

The role:
1. Checks kernel version ≥ 5.15, aborts if too old
2. Downloads `.deb` or `.rpm` from GitHub Releases (version auto-detected from `version/version.go`)
3. Installs with `apt` or `dnf` (preserves existing config with `confold`)
4. Starts and enables `qf-agent.service`
5. Verifies service is active

### 3.4 Override package version

```bash
ansible-playbook -i deploy/ansible/inventory.yml deploy/ansible/deploy-agent.yml \
  -e qf_agent_version=0.8.12
```

---

## 4. Agent: Manual Install on Linux

Works on any Linux x86_64 host — bare metal, VM, or Kubernetes node. Kernel ≥ 5.15 required.

### 4.1 Install package

```bash
VERSION=0.8.12

# Debian / Ubuntu
curl -LO https://github.com/qzmi4meister/qf/releases/download/v${VERSION}/qf-agent_${VERSION}_amd64.deb
sudo dpkg -i qf-agent_${VERSION}_amd64.deb

# RHEL / Fedora / Rocky
curl -LO https://github.com/qzmi4meister/qf/releases/download/v${VERSION}/qf-agent-${VERSION}.x86_64.rpm
sudo rpm -i qf-agent-${VERSION}.x86_64.rpm
```

The package installs:
- `/usr/local/bin/qf-agent` — binary
- `/etc/qf/agent.conf` — config template
- `/etc/systemd/system/qf-agent.service` — systemd unit

### 4.2 Configure

```bash
sudo nano /etc/qf/agent.conf
```

```ini
# gRPC stream endpoint (post-enrollment)
QF_CP_ENDPOINT=qf.example.com:8443

# Enrollment endpoint (used only on first start)
QF_ENROLL_ENDPOINT=qf.example.com:8444

# Bootstrap token — get from CP UI (Tokens page) or API
# Removed automatically after successful enrollment
QF_ENROLL_TOKEN=tok_xxxxxxxxxxxx

# Network interface to attach eBPF program
QF_IFACE=eth0

# PKI directory — stores agent.crt, agent.key, ca.crt after enrollment
QF_PKI_DIR=/etc/qf

QF_LOG_LEVEL=info
```

To find the correct interface name:

```bash
ip route get 8.8.8.8 | awk '{print $5; exit}'
```

### 4.3 Start agent

```bash
sudo systemctl enable --now qf-agent
sudo systemctl status qf-agent
```

On first start the agent:
1. Connects to `QF_ENROLL_ENDPOINT` and presents the bootstrap token
2. Sends a CSR; CP signs it and returns mTLS certificates
3. Stores certs in `QF_PKI_DIR` — subsequent starts skip enrollment
4. Connects to `QF_CP_ENDPOINT` over mTLS and awaits policy bundles

### 4.4 Verify enrollment

```bash
# Agent logs
journalctl -u qf-agent -f

# On the CP — host should appear with status "active"
curl -b cookies.txt -H 'X-Tenant-ID: <tenant-id>' \
  https://qf.example.com/hosts | jq '.[] | {hostname, status}'
```

### 4.5 Upgrade agent

```bash
VERSION=NEW_VERSION

# Debian / Ubuntu — preserves /etc/qf/agent.conf
sudo dpkg -i qf-agent_${VERSION}_amd64.deb

# RHEL
sudo rpm -U qf-agent-${VERSION}.x86_64.rpm

sudo systemctl restart qf-agent
```

---

## Troubleshooting

### Host stuck in `enrolling`

1. `journalctl -u qf-agent -n 50` — look for TLS or connection errors
2. `nc -zv qf.example.com 8444` — verify enrollment port reachable from the host
3. Check token is valid and not expired: CP UI → Tokens
4. Verify `QF_CP_HOST` on CP matches the hostname/IP in `QF_ENROLL_ENDPOINT` — TLS SAN mismatch causes handshake failure

### Bundle not arriving

1. Agent logs should show `bundle received` and `bundle applied`
2. Check mTLS certs exist: `ls -la /etc/qf/` — `agent.crt`, `agent.key`, `ca.crt` must be present
3. Verify port 8443 reachable: `nc -zv qf.example.com 8443`
4. Check cert expiry: `openssl x509 -in /etc/qf/agent.crt -noout -dates`
5. If cert expired: stop agent, delete `/etc/qf/agent.crt` and `/etc/qf/agent.key`, add `QF_ENROLL_TOKEN` back to config, restart

### Agent fails to start — BPF errors

```bash
journalctl -u qf-agent -n 20
```

- `BPF load failed: permission denied` — missing capabilities; check the systemd unit has `AmbientCapabilities=CAP_BPF CAP_NET_ADMIN CAP_SYS_ADMIN`
- `kernel version too old` — need ≥ 5.15; check `uname -r`
- `interface not found` — `QF_IFACE` does not match actual interface name; check `ip link`

### CP fails to start — database errors

- `migrations failed` — PostgreSQL user lacks `CREATE TABLE` permissions; grant: `GRANT ALL ON DATABASE qf TO qf;`
- `connection refused` — verify `QF_DB_DSN` host/port and that PostgreSQL is running
- `master key must be 32 bytes` — `QF_MASTER_KEY` must be a 64-character hex string (`openssl rand -hex 32`)

### TLS errors

| Error | Cause | Fix |
|---|---|---|
| `certificate signed by unknown authority` | Agent has wrong `ca.crt` or CP re-generated CA | Re-enroll agent |
| `certificate has expired` | Agent cert TTL elapsed (default 365d) | Delete old certs, re-enroll |
| `x509: certificate is valid for X, not Y` | `QF_CP_HOST` mismatch | Set `QF_CP_HOST` to the hostname agents use |

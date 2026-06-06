# qf — eBPF host firewall

qf is a centrally-managed, eBPF-based host firewall. A lightweight agent runs on each Linux host and enforces firewall rules pushed from a central control plane. Rules are expressed as policies with label selectors — the control plane compiles them to BPF maps and pushes bundles over mTLS gRPC.

## Architecture

```
┌─────────────────────────────────────────────┐
│  qf-cp  (Control Plane)                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐ │
│  │ REST API │  │  gRPC    │  │  Web UI   │ │
│  │ :8080    │  │  :8443   │  │  React    │ │
│  └──────────┘  └──────────┘  └───────────┘ │
│         │             │                     │
│         └─────────────┴──── PostgreSQL      │
└─────────────────────────────────────────────┘
           │ mTLS gRPC (policy bundles)
    ┌──────┴──────────────────────┐
    ▼                             ▼
┌────────────┐            ┌────────────┐
│  qf-agent  │            │  qf-agent  │
│  (host A)  │            │  (host B)  │
│  eBPF/TC   │            │  eBPF/TC   │
└────────────┘            └────────────┘
```

## Features

- **eBPF TC datapath** — ingress/egress packet filtering at the TC hook; no iptables dependency
- **Policy-as-code** — rules with label selectors, CIDR match, port ranges, protocol filters
- **Object groups** — reusable IP sets, port sets, host sets referenced across rules
- **Dry-run preview** — diff policies before applying
- **Flow events** — conntrack-based per-connection telemetry (TCP/UDP/ICMP)
- **Log events** — per-rule hit logging with rate limiting
- **Audit log** — all CP mutations recorded
- **mTLS** — mutual TLS between agents and CP; PKI managed by CP
- **OIDC** — optional SSO via any OIDC provider
- **Web UI** — built-in React UI for policy management, host overview, flow explorer

## Components

| Component | Language | Description |
|---|---|---|
| `cp/` | Go + React | Control plane: REST API, gRPC server, Web UI, PKI, policy compiler |
| `agent/` | Go + C (eBPF) | Firewall agent: BPF loader, conntrack, log/flow collector |
| `proto/` | Protobuf | gRPC service definitions |
| `deploy/` | Helm, Dockerfile, nfpm | Deployment artifacts |

## Quick start

See [docs/install.md](docs/install.md) for full installation instructions covering three scenarios:

1. **Standalone binary** — qf-cp + PostgreSQL on a single Linux server
2. **Kubernetes (Helm)** — qf-cp deployed via Helm chart
3. **All-in-K8s** — both CP and agents in Kubernetes

### Minimal local run

```bash
# PostgreSQL required
export QF_DB_DSN="postgres://qf:password@localhost:5432/qf"
export QF_MASTER_KEY="$(openssl rand -hex 32)"
export QF_JWT_SECRET="$(openssl rand -hex 32)"

# Build UI
make ui-build

# Run CP
go run ./cp/cmd/qf-cp/
# UI at http://localhost:8080/app
```

### Kubernetes (Helm)

```bash
helm upgrade --install qf-cp oci://ghcr.io/YOUR_ORG/helm/qf-cp \
  --namespace qf --create-namespace \
  --set secrets.dbDSN="postgres://qf:password@postgres:5432/qf" \
  --set secrets.masterKey="$(openssl rand -hex 32)" \
  --set secrets.adminPassword="$(openssl rand -base64 16)" \
  --set env.QF_CP_HOST="cp.example.com" \
  --set env.QF_CP_ENDPOINT="cp.example.com:8444"
```

### Install agent

```bash
# Debian/Ubuntu
curl -LO https://github.com/YOUR_ORG/qf/releases/latest/download/qf-agent_VERSION_amd64.deb
dpkg -i qf-agent_VERSION_amd64.deb

# RHEL/Fedora
curl -LO https://github.com/YOUR_ORG/qf/releases/latest/download/qf-agent-VERSION.x86_64.rpm
rpm -i qf-agent-VERSION.x86_64.rpm
```

Configure `/etc/qf/agent.conf` with the enrollment token from the UI, then:

```bash
systemctl enable --now qf-agent
```

## Building from source

**Prerequisites:** Go 1.22+, clang 14+, Linux kernel 5.4+ (for BPF compilation), Node.js 18+ (for UI)

```bash
# Generate BPF and proto stubs (requires Linux + clang)
make generate
make proto

# Build UI
make ui-build

# Build CP binary
make build-cp

# Build agent binary (run on Linux)
make build

# Build container image
make docker-cp QF_IMAGE=your-registry/qf-cp

# Package agent (.deb + .rpm)
make pkg-agent
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `QF_DB_DSN` | — | PostgreSQL DSN **(required)** |
| `QF_MASTER_KEY` | — | 32-byte hex key for token encryption **(required)** |
| `QF_JWT_SECRET` | ephemeral | JWT signing key (ephemeral = sessions lost on restart) |
| `QF_CP_HOST` | `localhost` | Hostname in CP TLS certificate SAN |
| `QF_CP_ENDPOINT` | `localhost:8444` | Enrollment endpoint advertised to agents |
| `QF_HTTP_ADDR` | `:8080` | REST API + UI listen address |
| `QF_GRPC_ADDR` | `:8443` | gRPC (mTLS) listen address |
| `QF_ENROLL_ADDR` | `:8444` | Enrollment gRPC listen address |
| `QF_PKI_DIR` | `/etc/qf/pki` | PKI directory (CA, certs, signing keys) |
| `QF_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `QF_ADMIN_EMAIL` | `admin@localhost` | Bootstrap admin email |
| `QF_ADMIN_PASSWORD` | `changeme` | Bootstrap admin password **(change in production)** |
| `QF_FORWARDER_DSN` | — | Syslog forwarding DSN |

## Agent configuration

`/etc/qf/agent.conf`:

```ini
QF_CP_ENDPOINT=cp.example.com:8443
QF_ENROLL_ENDPOINT=cp.example.com:8444
QF_ENROLL_TOKEN=<bootstrap-token-from-ui>
QF_PKI_DIR=/etc/qf/pki
QF_IFACE=eth0
```

## API

REST API documentation: `GET /openapi.yaml` or `GET /docs` (Swagger UI) on a running CP instance.

## Requirements

- **CP:** Linux or container; PostgreSQL 14+
- **Agent:** Linux x86_64 or arm64; kernel 5.4+ (5.10+ recommended); `CAP_BPF`, `CAP_NET_ADMIN`, `CAP_SYS_ADMIN`

## License

[MIT](LICENSE)

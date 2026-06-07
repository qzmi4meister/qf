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
- **Flow events** — conntrack-based per-connection telemetry (TCP/UDP/ICMP)
- **Log events** — per-rule hit logging with rate limiting
- **Audit log** — all CP mutations recorded with before/after state
- **mTLS** — mutual TLS between agents and CP; PKI managed by CP
- **OIDC** — optional SSO via any OIDC provider
- **Web UI** — built-in React UI for policy management, host overview, flow explorer

## Components

| Component | Language | Description |
|---|---|---|
| `cp/` | Go + React | Control plane: REST API, gRPC server, Web UI, PKI, policy compiler |
| `agent/` | Go + C (eBPF) | Firewall agent: BPF loader, conntrack, log/flow collector |
| `proto/` | Protobuf | gRPC service definitions |
| `deploy/` | Helm, Dockerfile, nfpm, Ansible | Deployment artifacts |

## Deployment

Full instructions: **[docs/install.md](docs/install.md)**

| Scenario | Guide |
|---|---|
| Control plane in **Kubernetes** (Helm) | [§1](docs/install.md#1-control-plane-in-kubernetes-helm) |
| Control plane on **standalone Linux** (systemd) | [§2](docs/install.md#2-control-plane-on-a-standalone-linux-machine) |
| Agent deploy via **Ansible** | [§3](docs/install.md#3-agent-deploy-via-ansible) |
| Agent **manual install** on Linux | [§4](docs/install.md#4-agent-manual-install-on-linux) |

### Quick: CP in Kubernetes

```bash
helm upgrade --install qf-cp \
  oci://ghcr.io/qzmi4meister/helm/qf-cp \
  --version 0.8.12 \
  --namespace qf --create-namespace \
  --set secrets.dbDSN="postgres://qf:PASSWORD@postgres:5432/qf" \
  --set secrets.masterKey="$(openssl rand -hex 32)" \
  --set secrets.jwtSecret="$(openssl rand -hex 32)" \
  --set secrets.adminPassword="CHANGE_ME" \
  --set env.QF_CP_HOST="qf.example.com" \
  --set env.QF_CP_ENDPOINT="qf.example.com:31444"
```

### Quick: Agent install

```bash
VERSION=0.8.12

# Debian/Ubuntu
curl -LO https://github.com/qzmi4meister/qf/releases/download/v${VERSION}/qf-agent_${VERSION}_amd64.deb
sudo dpkg -i qf-agent_${VERSION}_amd64.deb

# RHEL/Fedora/Rocky
curl -LO https://github.com/qzmi4meister/qf/releases/download/v${VERSION}/qf-agent-${VERSION}.x86_64.rpm
sudo rpm -i qf-agent-${VERSION}.x86_64.rpm
```

Edit `/etc/qf/agent.conf`, then:

```bash
sudo systemctl enable --now qf-agent
```

### Quick: Agent via Ansible

```bash
# Edit deploy/ansible/inventory.yml with your hosts
ansible-playbook -i deploy/ansible/inventory.yml deploy/ansible/deploy-agent.yml
# or: make deploy-agent
```

## Building from source

**Prerequisites:** Go 1.22+, clang 14+, Linux kernel 5.15+ (for BPF), Node.js 18+ (for UI)

```bash
make generate    # BPF + proto codegen (requires Linux + clang)
make ui-build    # build React UI
make build-cp    # build CP binary
make build       # cross-compile agent for linux/amd64
make docker-cp   # build CP container image
make pkg-agent   # package agent as .deb + .rpm
```

## Environment variables (CP)

| Variable | Default | Description |
|---|---|---|
| `QF_DB_DSN` | — | PostgreSQL DSN **(required)** |
| `QF_MASTER_KEY` | — | 32-byte hex key for token encryption **(required)** |
| `QF_JWT_SECRET` | ephemeral | JWT signing key (ephemeral = sessions lost on restart) |
| `QF_CP_HOST` | `localhost` | Hostname in CP TLS certificate SAN — must match what agents use |
| `QF_CP_ENDPOINT` | `localhost:8444` | Enrollment endpoint advertised to agents |
| `QF_HTTP_ADDR` | `:8080` | REST API + UI listen address |
| `QF_GRPC_ADDR` | `:8443` | gRPC (mTLS) listen address for connected agents |
| `QF_ENROLL_ADDR` | `:8444` | Enrollment gRPC listen address |
| `QF_PKI_DIR` | `/etc/qf/pki` | Directory for CA, certs, bundle signing key |
| `QF_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `QF_ADMIN_EMAIL` | `admin@localhost` | Bootstrap admin email (first run only) |
| `QF_ADMIN_PASSWORD` | `changeme` | Bootstrap admin password **(change immediately)** |

## Agent configuration (`/etc/qf/agent.conf`)

```ini
QF_CP_ENDPOINT=qf.example.com:8443
QF_ENROLL_ENDPOINT=qf.example.com:8444
QF_ENROLL_TOKEN=<bootstrap-token-from-ui>   # removed after first successful enrollment
QF_IFACE=eth0
QF_PKI_DIR=/etc/qf
QF_LOG_LEVEL=info
```

## Requirements

| | Minimum | Recommended |
|---|---|---|
| **CP** | Any Linux / container; PostgreSQL 14+ | PostgreSQL 15+, 1 vCPU, 256 MB RAM |
| **Agent** | Linux x86_64/arm64; kernel 5.15+; `CAP_BPF`, `CAP_NET_ADMIN`, `CAP_SYS_ADMIN` | Kernel 6.1+ for bpf_loop support |

## API

REST API: `GET /openapi.yaml` or `GET /docs` (Swagger UI) on a running CP instance.

## License

[MIT](LICENSE)

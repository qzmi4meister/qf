# syntax=docker/dockerfile:1

# ── Builder ───────────────────────────────────────────────────────────────────
FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /qf-cp \
    ./cp/cmd/qf-cp/

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot

# Required:
#   QF_DB_DSN         PostgreSQL DSN
#   QF_MASTER_KEY     32-byte hex CA master key
#
# Optional (defaults shown):
#   QF_PKI_DIR        /etc/qf/pki
#   QF_GRPC_ADDR      :8443   (mTLS gRPC for agents)
#   QF_ENROLL_ADDR    :8444   (enrollment gRPC, plain)
#   QF_HTTP_ADDR      :8080   (REST API + Web UI)
#   QF_CP_ENDPOINT    localhost:8444  (advertised to agents)
#   QF_CP_HOST        localhost       (server cert SAN)
#   QF_JWT_SECRET     (ephemeral if unset; tokens invalidated on restart)
#   QF_LOG_LEVEL      info    (debug|info|warn|error)
#   QF_ADMIN_EMAIL    admin@localhost
#   QF_ADMIN_PASSWORD changeme
#   QF_FORWARDER_DSN  (syslog://host:port — forward log events externally)
#   QF_OIDC_ISSUER / QF_OIDC_CLIENT_ID / QF_OIDC_CLIENT_SECRET / QF_OIDC_REDIRECT_URL
#
# Health: GET /healthz → 200 OK  (use as k8s livenessProbe httpGet)

COPY --from=builder /qf-cp /qf-cp

EXPOSE 8080 8443 8444

ENTRYPOINT ["/qf-cp"]

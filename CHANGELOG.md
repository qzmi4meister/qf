# Changelog

All notable changes to qf are documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning: [Semantic Versioning](https://semver.org/).

---

## [0.3.0] — 2026-05-31

### Added
- P3-API-01..06: observability REST + Prometheus — `events.go`: GET /hosts/{id}/events|flows|counters|counters/latest; optional query params (start/end RFC3339, rule_id, action, limit max 1000); `audit.go`: GET /audit-log with actor/object/time filters; `middleware_audit.go`: captures POST/PUT/PATCH/DELETE → response body as `after`, actor from X-Actor-ID/X-Actor-Type headers, async InsertAuditLog, extractObjectFromPath UUID regex; `metrics_handler.go`: GET /metrics → promhttp.Handler(); `cp/internal/metrics/`: ActiveStreams gauge, BundlePushDuration histogram, EventsIngested counter vec (log/flow/counter/system), AgentCPU/Mem/ConntrackUtil gauge vecs, DBPoolAcquired/Idle; registry.go: Inc/Dec ActiveStreams on register/deregister; ingester.go: EventsIngested.Add after each flush; main.go: 15s ticker → pool.Stat() → DBPool metrics
- P3-INGEST-01: Ingester — `cp/internal/ingest/`; 4 workers (log/flow/counter/system), buffered channels (10k/1k), flush on maxBatch=2000 or 1s tick; bulk-insert via `pgx.CopyFrom` (log/flow/counter), single-row for system_events; `convert.go`: proto→storegen params, `bytesToAddr` (4/16 bytes→`*netip.Addr`), enum→string helpers; drop on channel full with slog.Warn
- P3-DB-02: sqlc telemetry queries — `InsertLogEventsBatch`, `InsertFlowEventsBatch`, `InsertCounterSnapshotsBatch` (`:copyfrom` → `pgx.CopyFrom`), `InsertSystemEvent :one`; list queries with optional time-range + filter params: `ListLogEvents`, `ListFlowEvents`, `ListCounterSnapshots`, `GetLatestCounterSnapshotsForHost` (DISTINCT ON rule_id), `ListSystemEvents`, `DeleteOldSystemEvents`; `InsertAuditLog` + `ListAuditLog` with actor/object filters; sqlc.yaml: inet override → `*netip.Addr` (pgx/v5 native)
- P3-DB-01: observability migrations — `log_events` (partitioned daily), `flow_events` (partitioned daily), `counter_snapshots` (partitioned weekly), `system_events` (regular table, low-volume); indexes on `(host_id, created_at)`, `(rule_id, created_at)`; DO-block seeds initial partitions (today +30d for daily, current ISO week +8w for weekly); TTL/extension handled by partition manager (P3-INGEST-03)

---

## [0.2.0] — 2026-05-31

### Added
- P2-PKI-06: CP cert rotation — `pki.RandSerial()` exported; `AgentServer` gets `ca *pki.CA` + `rc *pki.RevocationChecker`; `handleCertRenewal`: verify active cert not revoked → parse CSR PEM → `SignHostCSR(365d)` → `InsertCertificate` → `UpdateCertificateStatus("rotated")` → `CertRenewalResponse{success,cert_pem,not_after}`; errors send failure response without killing stream; wired in `handleMessage`
- P2-MAIN-02: agent config — `agent/internal/config/config.go`; `Config{CPEndpoint,Interface,PKIDir,LogLevel}`; `Load(path)` parses KEY=VALUE file (optional) then applies env overrides (QF_CP_ENDPOINT, QF_IFACE, QF_PKI_DIR, QF_LOG_LEVEL); `LoadDefault()` uses `/etc/qf/agent.conf`
- P2-MAIN-01: CP main — `cp/cmd/qf-cp/main.go`; pgxpool + stdlib.OpenDBFromPool for goose; PKI init (CA, serverCert, bundleSigner, tokenStore, revocationChecker); BundleBuilder + StreamRegistry; mTLS gRPC server (:8443) + plain enrollment gRPC (:8444) + REST HTTP (:8080); signal shutdown → DisconnectAll → graceful stop all servers; config from env (QF_DB_DSN, QF_MASTER_KEY, QF_PKI_DIR, QF_GRPC_ADDR, QF_ENROLL_ADDR, QF_HTTP_ADDR, QF_CP_ENDPOINT, QF_LOG_LEVEL)
- P2-API-06: DefaultPolicy — `cp/internal/api/defaultpolicy.go`; `GET /default-policy` → GetDefaultPolicy; `PUT /default-policy` → UpsertDefaultPolicy; defaults deny/deny when fields empty
- P2-API-05: Bootstrap tokens — `cp/internal/api/tokens.go`; `GET/POST/DELETE /tokens`; uses `pki.TokenStore`; POST creates single_host or bulk token, returns plaintext token once; `NewRouter` now accepts `*pki.TokenStore`
- P2-API-04: ObjectGroups CRUD — `cp/internal/api/objectgroups.go`; `GET/POST/PUT/DELETE /objectgroups`; `?type=` filter via `ListObjectGroupsByType`; type validated (ipset/portset/hostset); spec as `json.RawMessage`; PUT updates spec only (type+name immutable)
- P2-API-03: Policies + Rules CRUD — `cp/internal/api/policies.go`; `GET/POST/PUT/DELETE /policies`, nested `GET/POST/PUT/DELETE /policies/{id}/rules`; selector+match as `json.RawMessage`; `resolvePolicy` verifies tenant ownership before rule ops; rule ownership check via `rule.PolicyID == policyUUID`
- P2-API-02: Hosts CRUD — `cp/internal/api/hosts.go`; `GET/POST /hosts`, `GET/PATCH /hosts/{id}`; tenant from `X-Tenant-ID` header; `HostResponse` with labels as `map[string]string`; PATCH supports labels+status independently; shared helpers: `writeJSON`, `apiError`, `decodeJSON`, `uuidToStr`; `NewRouter` now accepts `*storegen.Queries`
- P2-API-01: Chi router — `cp/internal/api/router.go`; `NewRouter()`: RequestID + structuredLogger (slog) + Recoverer; `GET /healthz` → `{"status":"ok"}`; dep: go-chi/chi v5.3.0
- P2-AGT-06: cert rotation — `agent/internal/grpcclient/certrotate.go`; `CertRotator{certFile,keyFile,sendFn,responseCh}`; `Run(ctx)`: load cert → threshold=notBefore+50%lifetime → sleep → renew; `renew`: ECDSA P-256 keygen → CSR → send CertRenewalRequest → wait 30s for response via `DeliverResponse` channel → atomic write cert+key → return nil (triggers reconnect); `atomicWrite` temp+rename, mode 0600
- P2-AGT-05: reconnect with backoff — `agent/internal/grpcclient/reconnect.go`; `BackoffConfig{InitialMs,MaxMs,Multiplier,JitterFrac}`; `DefaultBackoff()` = 1s→60s ×2 ±20%; `RunWithReconnect(ctx, cfg, dialFn, sessionFn)`: dial → session → on error: isFatal check (codes.Unauthenticated or "tls:"/"revoked" in msg) → stop; else backoff+retry; reset delay on clean session exit
- P2-AGT-04: heartbeat sender — `agent/internal/grpcclient/heartbeat.go`; `HeartbeatSender{stream, intervalMs, genFn, healthFn}`; `Run(ctx)` loops `time.After(interval)` → send; `SetIntervalMs` updates interval for next beat; `genFn` returns current applied generation; `healthFn` optional (nil → empty AgentHealth)
- P2-AGT-03: receive PolicyBundle — `agent/internal/grpcclient/bundle.go`; `HandleBundle(stream, bundle, pubKey, applyFn)`: verify sig → BundleAck(sig_verified); bad/no sig → BundleApplied(success=false), no apply; good sig → applyFn → BundleApplied(success, duration_ms)
- P2-AGT-02: Hello/Welcome + initial bundle — `agent/internal/grpcclient/handshake.go`; `Handshake(stream, hello)` sends Hello → recvs Welcome → if rejected returns error; if behind (hello.current_generation < server_generation) recvs next ServerMessage expecting PolicyBundle; returns `HandshakeResult{Welcome, Bundle}`; caller applies Bundle
- P2-AGT-01: gRPC mTLS dial — `agent/internal/grpcclient/client.go`; `Config{CertFile,KeyFile,CAFile,CPEndpoint}`; `DefaultConfig` uses `/etc/qf/` paths; `Dial` loads X.509 keypair + CA pool → TLS1.3 creds → `grpc.NewClient`; `Stream()` opens `AgentService.Stream`; `Close()` closes conn
- P2-GRPC-06: DisconnectRequest + graceful shutdown — `registry.go` extended: `disconnectSignal`, `Disconnect(hostID,reason,ms)`, `DisconnectAll`; `register` returns `chan disconnectSignal`; `session.go` main recv loop moved to goroutine → select between `recvCh` and `disconnectCh`; on signal: sends `ServerMessage{DisconnectRequest}`, returns nil
- P2-GRPC-05: push on policy change — `agentsrv/registry.go`; `StreamRegistry` tracks active streams (hostID→mutex-wrapped sender); `Dispatch(BundleUpdate)` implements `policy.Dispatcher` — RLock lookup → serialize send with per-stream mutex; `Stream()` registers on connect, defers deregister; `NewMTLSServer`/`NewAgentServer` accept `*StreamRegistry`
- P2-COMP-05: cascade recompiler — `cp/internal/policy/cascade.go`; `CascadeRecompiler.OnObjectGroupChanged`: og→`rule_objectgroup_refs`→rules→policies→selector→hosts→rebuild+dispatch; `OnPolicyChanged`: policy selector→hosts→rebuild+dispatch; `Dispatcher` interface decouples from GRPC-05; partial failures logged, continue remaining hosts
- P2-COMP-04: bundle assembly — `cp/internal/policy/bundle.go`; `BundleBuilder.Build(ctx,tenantID,hostID,generation)` compiles ruleset → `PolicyBundle` proto → Ed25519 sign via `BundleSigner`; `GetBundle` implements `agentsrv.BundleProvider` (looks up host generation+1); `buildProtoBundle` maps `ResolvedRule→EffectiveRule`, auto-enables log on DENY unless silent, sets `requires_conntrack` if any rule has state predicate
- P2-COMP-02: effective ruleset — `cp/internal/policy/ruleset.go`; `RulesetCompiler.Compile(ctx, tenantID, hostID)`: loads all policies, filters by selector against host labels, resolves ObjectGroup refs (src/dst CIDR+port via `Resolver`), merges and sorts by `(policy.priority, rule.priority, rule.id)`; `RuleMatchSpec` JSONB shape; `protoProtocol/Action/Direction/State` helpers
- P2-COMP-03: ObjectGroup resolver — `cp/internal/policy/resolver.go`; `Resolver.Resolve(ctx, tenantID, ogID)`; IPSet: parses `spec.cidrs[]` → `[]*qfv1.CIDR`; PortSet: parses `spec.ports[]` ("80" or "8080-8090") → `[]*qfv1.PortRange`; HostSet: resolves `spec.selector` via `SelectorMatcher` → active hosts → IP from label `"ip"` → /32 or /128 CIDR
- P2-COMP-01: selector matching — `cp/internal/policy/selector.go`; `Selector` (matchLabels + matchExpressions In/NotIn/Exists/DoesNotExist); `MatchesLabels`; `SelectorMatcher.ResolveHosts` loads all tenant hosts, filters active+matching; `ResolveHostIDs` returns UUIDs
- P2-GRPC-04: BundleAck + BundleApplied — `handleBundleAck` logs bad-sig warning; `handleBundleApplied` calls `UpdateHostGeneration` on success, logs failure with generation+duration; no DB write on failure (logged only)
- P2-GRPC-03: Heartbeat — `handleHeartbeat` calls `UpdateHostHeartbeat` (last_heartbeat_at=NOW, current_generation, agent/kernel version); stale lag warning if heartbeat.ts older than 3 min; `parseUUIDs` helper
- P2-GRPC-02: Hello/Welcome — `session.go`; Stream receives Hello, queries host `current_generation`, sends Welcome (accepted, server_generation, defaultConfig); if agent behind → calls `BundleProvider.GetBundle` and pushes PolicyBundle; main receive loop dispatches to `handleMessage`; `BundleProvider` interface decouples compiler; `defaultConfig` (30s heartbeat, 60s counters, 100-event batch)
- P2-GRPC-01: mTLS gRPC server — `cp/internal/agentsrv/server.go` + `auth.go`; `NewMTLSServer(serverCert, ca, rc)` builds TLS13 server with `RequireAndVerifyClientCert`, CA pool, revocation `VerifyPeerCertificate`; `StreamAuthInterceptor` extracts `PeerIdentity{HostID,TenantID}` from cert CN/OU into context; `AgentServer` stub with `PeerIdentityFromContext`
- P2-PKI-04: enrollment endpoint — `cp/internal/pki/enrollment.go`; `EnrollmentServer` implements `EnrollmentServiceServer`; validates token via `TokenStore.ValidateAndConsume`; single_host: hostname check against target; bulk: creates host record; signs CSR with `CA.SignHostCSR` (CN=hostID, OU=tenantID); saves cert to DB; updates host status to active; returns cert+ca+bundle_pub+cp_endpoint
- P2-DB-04: sqlc codegen — queries for hosts, policies+rules+refs, object_groups, bootstrap_tokens, certificates, tenants+default_policies; generated `cp/internal/store/gen/` (8 files); sqlc v2, pgx/v5 driver
- P2-PKI-05: revocation blocklist — `cp/internal/pki/revocation.go`; `RevocationChecker` with RWMutex in-memory set; `Reload` from PostgreSQL, `Revoke` (immediate), `IsRevoked`; `VerifyPeerCertificate` + `WrapTLSConfig` for mTLS; `StartPeriodicReload` goroutine
- P2-DB-03: versioning + audit migrations — `000003_versioning.sql` (config_versions, audit_log + indexes)
- P2-PKI-03: bootstrap tokens CRUD — `cp/internal/pki/tokens.go`; `TokenStore` over pgxpool; `CreateSingleHostToken`, `CreateBulkToken`, `ValidateAndConsume` (atomic UPDATE, distinguishes expired vs exhausted), `DeleteToken`, `ListTokens`; token = 32-byte random base64url, stored as SHA-256 hex
- P2-DB-02: PKI migrations — `000002_pki.sql` (certificates + bootstrap_tokens tables + indexes)
- P2-PKI-01: CA init — `cp/internal/pki/ca.go`; ECDSA P-256 root CA; AES-256-GCM encrypted key on disk; `LoadOrInit`, `SignCSR`, `TLSCertificate`, `LoadOrGenerateServerCert`
- P2-PKI-02: bundle signing key — `cp/internal/pki/bundle_signer.go`; Ed25519 keypair; `LoadOrInitBundleSigner`, `SignBundle` (deterministic proto marshal → Ed25519 sign)
- P2-DB-01: goose migrations bootstrap — `cp/migrations/000001_init.sql` (tenants, hosts, policies, rules, object_groups, rule_objectgroup_refs, default_policies + indexes); `cp/migrations/embed.go` FS embed; `cp/internal/store/migrate.go` `Migrate(ctx, db)` helper; deps: goose v3.27.1, pgx v5.9.2
- P1-BPF-01: packet parser — Eth→IPv4/IPv6→TCP/UDP/ICMP into `struct pkt_ctx`
- P1-BPF-02: rule matching engine — bounded loop (EVAL_MAX_RULES=64), inline CIDR/port, first-match-wins, per-rule counters
- P1-BPF-03: CIDR IPSet spill — LPM trie (`qf_ipsets`, `BPF_MAP_TYPE_LPM_TRIE`) for rules with >8 CIDRs; compound key `{prefixlen, ipset_id, addr}`; `ClearIPSets`/`PushIPSet` Go API; `RuleApplier` interface extended
- P1-BPF-05: conntrack state machine — TCP FSM, UDP pseudo-state, ICMP echo/reply
- P1-BPF-06: CT state predicate in rule matching — CT_NONE (stateless), CT_NEW, CT_ESTABLISHED; CT_RELATED stub
- `EVAL_MAX_RULES=64` cap to stay within BPF verifier jump-complexity limit (8192)
- P1-GO-02: rule push API — `PushRules(rules []RuleSpec)`, `ParseCIDR4`, `PortRange`, priority sort, full validation before any map write
- P1-GO-03: conntrack reader — `ConntrackDump`, `ConntrackLookup` (auto-canonicalises key direction), `TCPCS*` sub-state constants
- P1-BPF-07: counter reader — `ReadCounter(idx)` / `ReadCounters()` aggregate per-CPU `qf_rule_counters`
- P1-BPF-08: event consumer — `EventReader` wraps `ringbuf.Reader`; `LogEvent` decoded from 44-byte wire struct; verified ring buffer delivers from `BPF_PROG_TEST_RUN` on kernel 7.0
- P1-BPF-09: config init — `Config`, `DefaultConfig`, `SetConfig`/`GetConfig`; called in `Load` to initialise `qf_config[0..2]`
- Go test helpers for BPF map population; byte-order fix for `__be32` fields on x86_64
- P1-GO-04: policy compiler — `CompileBundle(PolicyBundle) → ([]RuleSpec, Config, warnings)`; IPv6 CIDR filter; UUID parse; priority = array index
- P1-GO-05: policy handler — `PolicyHandler.Apply` compiles + pushes to BPF; `MakeBundleApplied` builds CP ack proto; `RuleApplier` interface for testing
- P1-GO-06: agent lifecycle — `Agent.Start` drains BPF event ring buffer; `Agent.Reload` hot-reloads policy bundle; graceful shutdown via context cancellation; `main.go` wired up
- P1-POL-01: bundle file format — `SaveBundle`/`LoadBundle` (proto wire, atomic rename); `policy/bundle.go`
- P1-POL-02: bundle signature — `SignBundle`/`VerifyBundle` Ed25519; canonical bytes = deterministic proto marshal with `Signature` field cleared; `ErrInvalidSignature`/`ErrNoSignature` sentinels
- P1-POL-03: boot loader — `Agent.LoadAndApply(path, pubKey)` loads cached bundle, verifies sig (optional), applies via `Reload`; missing file is not an error
- P1-TEST-01..05: full BPF test suite — 26 tests in `loader_test.go` (rule matching, CIDR/port/IPSet, conntrack TCP/UDP/ICMP, TCP 3WHS state transitions, ring buffer); 6 boot tests in `boot_test.go`; helpers: `buildSYN/ACK/SYNACK/FIN`, `buildUDP`, `buildICMPEcho`, `runEgress`

### Fixed
- ICMP conntrack key asymmetry — parser now sets `dst_port = echo_id` (same as `src_port`) for `ICMP_ECHO`/`ICMP_ECHOREPLY` and ICMPv6 equivalents so canonical CT key matches in both directions
- BPF verifier complexity explosion — `match_rule` changed from `__always_inline` to `__noinline`; verifier analyses it once as a subprogram instead of inlining into EVAL_MAX_RULES=64 loop iterations (was: 434-line error, now: loads cleanly)

---

## [0.1.0] — 2026-05-31

Phase 0 spikes complete. All technical unknowns de-risked.

### Added
- P0-PROTO-01: proto files moved to `proto/qf/v1/`; `buf generate` producing `.pb.go` + `_grpc.pb.go`
- P0-BPF-01/02: minimal eBPF TC program with CO-RE (`vmlinux.h`); compiles and loads on kernel 5.10–7.0
- P0-GO-01: `bpf2go` codegen in `agent/internal/loader/`; `Load(iface)` / `Close()` via TCX
- P0-GO-02: `BPF_PROG_TEST_RUN` harness; verdict tests green
- P0-SPIKE-01: TCX attach proof on kernel ≥6.6; `bpftool net` chain verified
- P0-SPIKE-02: TCX + Cilium coexistence on eth0 (kernel 6.8); clean detach after `Close()`
- BPF maps declared: `qf_rules`, `qf_rule_count`, `qf_conntrack`, `qf_rule_counters`, `qf_events`, `qf_config`
- `common.h` structs: `cidr4`, `port_range`, `ct_key`, `ct_entry`, `rule_match`, `rule_entry`, `rule_counter`, `log_event`
- Go module `github.com/qf/qf`; dependencies: cilium/ebpf, huma, chi, pgx, goose, grpc, protobuf
- `Makefile` with targets: `generate`, `proto`, `bpf`, `build`, `clean`

### Infrastructure
- SSH MCP server (`mcp-ssh-manager`) configured for `qf` (178.105.179.98) and `qw1` (qw1.euio.ru)
- buf 1.70.0 for proto lint + codegen

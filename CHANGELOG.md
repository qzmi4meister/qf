# Changelog

All notable changes to qf are documented in this file.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning: [Semantic Versioning](https://semver.org/).

---

## [0.8.31] — 2026-06-10

### Fixed
- Dashboard convergence panel: drifted filter was `current !== desired` — incorrect because `desired_generation` was initialized to 0 by migration 006 while `current_generation` was already >0; counters diverge by design. Correct check: `current < desired` (agent behind = not converged)

## [0.8.30] — 2026-06-10

### Changed
- Dashboard: replaced "Bundle pushes" stub panel with real "Policy convergence" panel — shows converged/total hosts, progress bar, list of drifted hosts (current_gen → desired_gen) with link to host detail
- API: `GET /api/v1/hosts` now includes `desired_generation` field in each host response

## [0.8.29] — 2026-06-10

### Fixed
- P8f-09: heartbeat DB error now logs warn + returns nil (best-effort) instead of tearing down gRPC stream
- P8f-10: API now rejects rules with `state=related` (422) — CT_RELATED unimplemented in BPF datapath
- P8f-11: agent logs IPv6 compile warnings via slog.Warn on bundle apply
- P8f-12: compat eval_rules null-rule check changed from `continue` to `break` — matches full/bpf_loop stop semantics
- P8f-13: `ChangePassword` rejects passwords >72 bytes (bcrypt silent truncation)
- P8f-14: `ingester_events_dropped_total` counter added; incremented on log/flow/counter/system channel drops
- P8f-15: `isTCXUnsupported` uses `errors.Is` against `syscall.EOPNOTSUPP`/`ENOSYS`/`ENOENT` instead of string match
- P8f-16: `handleHeartbeat` receives caller's stream context instead of creating `context.Background()`
- P8f-17: `uuidStr` in auth package uses `%08x-%04x-%04x-%04x-%012x` — matches padded `uuidToStr` in api package

## [Unreleased]

### Planning
- Phase 8 UI design system: 25 задач (P8a-01..P8d-06) — CSS tokens, Mantine theme, Logomark, AppShell, shared components (QFBadge/StatusBadge/Chip/QFCard/EmptyState/ErrorState/Skeleton/PageHead), screen redesigns (Hosts/HostDetail/Policies/PolicyDetail+Inspector/Login split/Dashboard KPIs+Donut+Sparklines)

### Added (P8a)
- `qf-tokens.css`: design token system (slate + indigo primitives, light/dark semantic vars, nav/row CSS classes)
- Mantine theme: slate-950 dark ramp, custom indigo ramp, 13px base font, qf radii
- `components/Mark.tsx`: SVG logomark (eBPF hook motif)
- AppShell Header: Mark + tenant chip + search placeholder + theme toggle + user menu
- AppShell Nav: 2px indigo active spine, indigo icon, version footer

### Added (P8b)
- `components/QFBadge.tsx`: tone-based badge (ok/bad/warn/info/pol/term/neutral) with exported TONE_VARS
- `components/StatusBadge.tsx`: status dot + label + glow-ring on active
- `components/Chip.tsx`: key|value mono-pill for host labels
- `components/QFCard.tsx`: border-defined surface card, no shadow, pad={false} for tables
- `components/EmptyState.tsx`: icon + title + body + action slot
- `components/ErrorState.tsx`: bad-fg triangle + body mono + retry button
- `components/Skeleton.tsx`: shimmer animation + SkeletonRow helper
- `components/PageHead.tsx`: title (19px 700) + mono sub + actions slot
- `components/QFTable.tsx`: TH (11px uppercase 0.05em) + TD (40px, mono variant) + QFTable wrapper

### Added (P8c)
- Hosts: QFCard+QFTable+SortTH+StatusBadge+Chip+SkeletonRow+EmptyState; tone toggle-filters
- HostDetail: QFCard overview grid, Chip label-pills, StatusBadge glow, custom breadcrumb header
- Policies: QFCard+QFTable+SortTH+SkeletonRow+EmptyState+brand "New policy" button
- PolicyDetail Inspector: 320px flex side panel (not modal), indigo left-spine on selected rule row
- PolicyDetail Rule Table: TH/TD, QFBadge action (ok=allow, bad=deny, info=log), mono prio+port

### Added (P8d)
- Login: split-layout (46% kernel-grid aside + right form pane), responsive (aside hides < 768px)
- Mark.tsx: updated to canonical design-handoff SVG (rounded rect + eBPF hook + filled packet rect)
- Dashboard: 5 KPI cards (Kpi component, 30px mono, tone coloring, breakdown mini-bar), SVG Donut 148px (segments by tone), Sparkline bar chart (34px), fleet status breakdown bars, recent activity table
- AuditLog: PageHead+QFCard+QFTable+SortTH+QFBadge; actor avatar; modal for before/after diff
- Events: PageHead+QFCard+QFTable+FilterPills+QFBadge; keeps SSE live-stream logic
- Flows: PageHead+QFCard+QFTable+FilterPills+EmptyState; keeps host selector
- Tokens: PageHead+QFCard+QFTable+SortTH+QFBadge+EmptyState; keeps all modals
- Users: PageHead+QFCard+QFTable+SortTH+QFBadge+EmptyState+UserAvatar; keeps all modals

## [0.8.20] — 2026-06-08

### Added
- P8e-06: `audit_log` JOIN `users` → `actor_username` field in API response; AuditLog.tsx shows username instead of UUID
- P8e-07: Username auth — migration `000009_username.sql` adds `username` column (backfill from email local-part, unique per tenant); login by username instead of email; JWT Claims include `username`; OIDC uses `preferred_username` claim; bootstrap admin via `QF_ADMIN_USERNAME` env
- P8e-08: Helm — `QF_ADMIN_USERNAME` in `values.yaml`, `secret.yaml`, deployment env; OpenAPI spec updated (`email` → `username` in login schema)

### Fixed
- P8e-01: Login → Dashboard transition: removed `await` from `qc.invalidateQueries(['me'])` — navigate fires immediately after cookie set
- P8e-02: WebSocket backend: gorilla/websocket hub, `GET /ws` endpoint (auth-gated), fan-out `{"topic":"<t>"}` on every audit log write
- P8e-03: WebSocket frontend: `useWebSocket` hook with exponential backoff reconnect; global in Layout; invalidates TanStack Query cache by topic; removed `refetchInterval: 30_000` from Dashboard
- P8e-04: Dashboard Refresh button now invalidates hosts, policies, and audit-log (was hosts-only)
- P8e-05: Removed dead "Push bundle" button from Dashboard

## [0.8.19] — 2026-06-07

### Added
- WebSocket invalidation hub (`cp/internal/ws/hub.go`): gorilla/websocket, ping/pong keepalive (30s ping, 60s pong timeout), fan-out broadcast
- `useWebSocket` hook: exponential backoff (500ms → 30s), reconnects on close/error, invalidates queryClient by topic on message

### Fixed
- Login transition slowness: `await qc.invalidateQueries` after login blocked navigation; now fire-and-forget
- Dashboard Refresh button: only refetched hosts; now invalidates all three queries (hosts/policies/audit-log)
- Dashboard: removed non-functional "Push bundle" button; live indicator updated from "auto-refresh 30s" to "Live · WS"

## [0.8.2] — 2026-06-03

### Added
- Per-host `flow_events_enabled` flag: DB column + PATCH API + Switch toggle in HostDetail Overview
- PATCH `flow_events_enabled` sends DisconnectRequest to active agent stream so it reconnects and picks up new config immediately

## [0.8.1] — 2026-06-03

### Added
- HostDetail: "Ruleset" tab shows effective resolved rules applied to host; "Download JSON" button exports ruleset
- Backend: `GET /hosts/{id}/ruleset` endpoint compiles and returns effective ruleset for a host

## [0.8.0] — 2026-06-03

### Fixed
- Preview impact: `h.added`/`h.removed`/`h.changed` can be `null` from backend; accessing `.length` crashed React — guarded with `?? []`

## [0.7.9] — 2026-06-03

### Fixed
- Preview impact: `DryRunRuleSpec.Match` was `[]byte` — Go decoded it as base64 string, but UI sends JSON object; changed to `json.RawMessage` so wire format is raw JSON
- Preview impact: catch block now shows red notification instead of silent white modal

## [0.7.8] — 2026-06-03

### Fixed
- ObjectGroups: Tabs.Panel was dynamic-valued — Mantine couldn't match panels to tabs, list never rendered; replaced with static per-type panels
- ObjectGroups: added onError handler — creation/update errors now shown as red notification instead of silent no-op

## [0.7.7] — 2026-06-02

### Fixed
- ObjectGroups: portset spec field `ranges` → `ports` to match resolver expectation
- ObjectGroups: hostset spec now wraps labels under `selector.matchLabels` as required by Selector type
- ObjectGroups: SpecSummary for portset reads `spec.ports`; for hostset reads `spec.selector.matchLabels`
- Rule editor: object group type names `ip_set`/`port_set`/`host_set` → `ipset`/`portset`/`hostset`

## [0.7.6] — 2026-06-02

### Fixed
- Rule editor: object group type names corrected (`ipset`/`portset`/`hostset`, not `ip_set`/`port_set`/`host_set`) — groups now actually appear in selectors
- ObjectGroups page: portset SpecSummary read `spec.ranges` instead of `spec.ports` — ports now display correctly

## [0.7.5] — 2026-06-02

### Added
- Rule editor: Object Group selectors for src/dst IP (ip_set, host_set) and src/dst ports (port_set); inline and group are mutually exclusive per field

### Fixed
- Rule match `src_ip`/`dst_ip` legacy field names migrated to correct `src_cidrs`/`dst_cidrs` on save (backend was silently ignoring the old field names)

## [0.7.4] — 2026-06-02

### Changed
- All date/time in UI now formatted as DD.MM.YYYY HH:MM:SS (European, 24h); centralised in `ui/src/utils/date.ts`

## [0.7.3] — 2026-06-02

### Fixed
- Tokens API: `GET /tokens` and `POST /tokens` now resolve tenant from JWT cookie claims first (like hosts endpoint), falling back to `X-Tenant-ID` header; UI was unable to list or create tokens without the header

## [0.7.2] — 2026-06-02

### Fixed
- Events live mode: IPs shown as base64, direction/action as integers, timestamps as "Invalid Date" — publishToHub now converts proto bytes→IP string, enums→string, Timestamp→RFC3339 before SSE publish
- Events table: protocol column now shows name (TCP/UDP/ICMP) instead of proto enum integer

## [0.7.1] — 2026-06-02

### Changed
- Events: search field now matches src/dst port in addition to IP and rule ID; placeholder updated to "IP, port or rule ID…"

## [0.7.0] — 2026-06-02

### Added
- BPF dual-variant compat: `TcFilterCompat` (kernel 5.15–5.16, 32 rules, bounded loop) and `TcFilter` (kernel ≥5.17, 64 rules, bpf_loop); variant selected at runtime via `KernelVersion()`
- Fixed "infinite loop detected" BPF verifier error on kernel 5.15: loop counter kept in callee-saved register by using a separate `key` variable instead of `&i`
- `BpfObjects` wrapper struct unifies both generated variants behind a single interface

## [0.6.4] — 2026-06-02

### Changed
- PolicyDetail: host selector replaced with key-value label editor; rule match replaced with structured fields (protocol, src/dst IP, src/dst ports)
- ObjectGroups: spec textarea replaced with type-specific editors (CIDR list, port/range list, label selector); table shows human-readable content

## [0.6.3] — 2026-06-02

### Added
- Dark mode: toggle button (sun/moon icon) in header; persists to localStorage; respects OS `prefers-color-scheme` on first load; anti-FOUC inline script in `index.html`

## [0.6.2] — 2026-06-01

### Fixed
- `GET /policies/{id}` now returns `rules[]` inline (was missing — UI showed empty rules list on policy open)
- `PUT /policies/{id}` now accepts and syncs `rules[]` from request body; previously rules were silently discarded on save

## [0.6.1] — 2026-06-01

### Fixed
- chi routing conflict: `GET /hosts/{id}` returned 404 because nested `r.Route("/{id}", ...)` intercepted the request before `registerHosts`' handler. Flattened by moving event routes to `/{id}/events`, `/{id}/flows`, etc. directly in the `/hosts` subrouter.

## [0.6.0] — 2026-06-01

### Fixed
- REV-1.1: Offline policy sync — agents that reconnect after missed cascade pushes now receive the updated bundle. Added `desired_generation` column to `hosts`; cascade increments it in DB before dispatching; session catch-up check compares `hello.current_generation < desired_generation` (not `current_generation`); migration `000006_desired_generation.sql`
- REV-1.3: Agent `Hello` message now sends `hostname`, `kernel_version` (from `/proc/sys/kernel/osrelease`), and `interfaces` list
- REV-1.4: Cascade recompiler wired to `/objectgroups` (update/delete), `/default-policy` (put), and `/hosts/{id}` label patch; added `OnDefaultPolicyChanged` and `OnHostLabelsChanged` to `CascadeRecompiler`
- REV-1.5: `EventBatcher.flush()` writes batch to `DiskBuffer` when gRPC send fails; added `SetDiskBuf` setter; events survive CP outage and replay on reconnect
- REV-2.1: BPF conntrack: first packet of new connection now counted (`new_e.packets_fwd = 1; new_e.bytes_fwd = ctx->pkt_size` set at entry creation)
- REV-2.2: CP background goroutine marks hosts `stale` every 30s when `last_heartbeat_at < NOW() - 90s`; `MarkStaleHosts` SQL query added
- REV-2.3: Default host status in `POST /hosts` changed from `"pending"` (violates DB constraint) to `"enrolling"`
- REV-1.2 (partial): `loader.go` returns clear error when TCX attach fails on kernels <6.6; full `clsact` fallback deferred to Phase 2

### Added
- P5-SEC-04: TLS hardening CP HTTP — `http.Server`: ReadHeaderTimeout 10s, ReadTimeout 30s, IdleTimeout 120s, TLSConfig with MinVersion TLS 1.2 and 6 ECDHE-AES-GCM/CHACHA20 cipher suites (takes effect when ListenAndServeTLS used); HSTS middleware on `/app/*` (`Strict-Transport-Security: max-age=31536000; includeSubDomains`); `loginRateLimiter` (in-process sliding window, 5 attempts/ip/min → 429) wired on `POST /auth/login` via `r.With()`; lazy GC of expired windows; tests: `TestLoginRateLimiter_{Allow,DifferentIPs,WindowReset}` (all pass)
- P5-SEC-03: Secrets audit — (1) `QF_MASTER_KEY` entropy check at startup: `isLowEntropy()` detects all-bytes-identical keys (all-zero, all-FF, etc.) → fatal error with `openssl rand -hex 32` hint; (2) `QF_JWT_SECRET` min-length warn: `< 32` bytes → `slog.Warn` with byte count; (3) API token entropy: `crypto/rand` 32-byte hex already in place (verified); (4) OIDC client secret masking: `OIDCConfig.String()` added → `fmt.Sprintf("%v", cfg)` and slog struct attrs emit `<masked>` not raw secret; tests: `TestIsLowEntropy` (7 subtests, cp/cmd/qf-cp), `TestOIDCConfig_String_{MasksSecret,NoSecret,ZeroValue}` (cp/internal/auth)
- P5-SEC-02: PKI hardening — (1) expired cert re-enrollment: `main.go` detects TLS "expired/not yet valid" error → calls `grpcclient.Enroll` (new enrollment client, plain gRPC :8444) if `QF_ENROLL_TOKEN` configured, rewrites PKI files atomically, reconnects; (2) revocation blocklist: 5-min periodic reload already wired in main.go, added 4 unit tests (`Revoke/IsRevoked`, concurrent access -race, `VerifyPeerCertificate` rejects revoked cert, `WrapTLSConfig`); (3) token race-free single-use: fixed critical bug `AND uses_count < max_uses` → `AND (max_uses=0 OR uses_count < max_uses)` (unlimited tokens max_uses=0 were always rejected); token integration tests `TestTokenStore_{UnlimitedToken,SingleUse_IsRaceFree,ExpiredToken,ExhaustedToken}` (skip if no QF_TEST_DSN); `make test-pki` runs all via SSH
- P5-SEC-01: Bundle signing hardening — removed `if pubKey != nil` skip-branch in `LoadAndApply` (boot.go); `VerifyBundle` guards nil key (len != 32 → ErrInvalidSignature, no panic); `main.go` loads `bundle-signing.pub` from `QF_PKI_DIR` at startup and passes to `RunFullConfig.BundleKey` (fatal if file missing); 12 unit tests: `TestHandleBundle_TamperedPayload/WrongKey/NoSignature/ValidSignature/ApplyError` (grpcclient), `TestBoot_LoadAndApply_NilKey_RejectsSignedBundle/NilKey_RejectsUnsignedBundle` (agent); all pass
- P5-DOC-03: Ops runbook — `docs/ops-runbook.md`: PKI backup/restore, QF_MASTER_KEY rotation (AES-256-GCM re-key), JWT rotation (session invalidation), bundle signing key rotation, agent re-enrollment flow, certificate revocation, PostgreSQL partition retention tuning (TTL constants), horizontal scaling requirements (shared PKI ReadWriteMany PVC, identical secrets), debug checklist (enrolling/bundle/events/TLS/CP startup errors)
- P5-DOC-02: Install guide — `docs/install.md`: 3 сценария (Standalone Linux binary+postgres+systemd, k8s node с CP Helm, All-in-k8s DaemonSet stub); enrollment token flow; troubleshooting раздел (enrolling stuck, bundle not arriving, events missing, TLS errors, systemd failures)
- P5-DOC-01: OpenAPI spec — `docs/openapi.yaml` (OpenAPI 3.0.3): все REST-эндпоинты (auth, hosts, events/flows/counters, policies+rules+preview+versions, objectgroups, tokens, default-policy, audit-log, users, api-tokens); полные schemas (Host/Policy/Rule/ObjectGroup/Token/DefaultPolicy/LogEvent/FlowEvent/Counter/AuditLog/User/APIToken + request/response types); securitySchemes cookieAuth+bearerAuth; reusable parameters (TenantID/ResourceID/Limit/StartTime/EndTime); `docs/embed.go` — `package docs` с `//go:embed openapi.yaml`; `GET /openapi.yaml` → application/yaml (cached 5min); `GET /docs` → Swagger UI (unpkg CDN); `go vet` чисто
- P5-PKG-03: Helm chart CP — `deploy/helm/qf-cp/`: Chart.yaml (v0.4.9), values.yaml (replicaCount, image, service ports 8080/8443/8444, ingress, pki.persistentVolume existingClaim/storageClass/size, secrets block→Kubernetes Secret, env block→direct env); templates: _helpers.tpl, secret.yaml (QF_DB_DSN/MASTER_KEY/JWT_SECRET/ADMIN_*/OIDC_CLIENT_SECRET), pvc.yaml (ReadWriteOnce, skipped if existingClaim set), service.yaml (ClusterIP), deployment.yaml (envFrom Secret + env map, PKI volumeMount, liveness+readiness /healthz), ingress.yaml (disabled by default); `make helm-package` → `deploy/helm/qf-cp-0.4.9.tgz`; `helm template` verified
- P5-PKG-02: Container image CP — `deploy/Dockerfile.cp`: multi-stage builder (golang:1.25 + CGO_ENABLED=0 -trimpath -s -w) → distroless/static:nonroot; EXPOSE 8080/8443/8444; all QF_* env vars documented in Dockerfile comments; health via GET /healthz (k8s livenessProbe); `make docker-cp` builds multi-arch manifest (linux/amd64+arm64) via podman into `localhost/qf-cp:VERSION`
- P5-PKG-01: deb/rpm packaging — `deploy/packaging/nfpm.yaml` (VERSION/ARCH env vars), `deploy/systemd/qf-agent.service` (CAP_NET_ADMIN+CAP_BPF+CAP_SYS_ADMIN, Restart=on-failure), `deploy/packaging/agent.conf` (config template with QF_CP_ENDPOINT/QF_IFACE/QF_PKI_DIR/QF_LOG_LEVEL), `deploy/packaging/postinst` (daemon-reload + enable), `deploy/packaging/prerm` (stop + disable); `make pkg-agent` builds binary then runs nfpm for .deb → `deploy/packaging/deb/` and .rpm → `deploy/packaging/rpm/`
- P5-BENCH-04: Conntrack stress bench — `agent/internal/loader/bench_conntrack_test.go`: 4 benchmarks (HotPath_Full_TCP/UDP/Mixed at 65536-entry LRU saturation, LRUEviction_Put throughput with every Put triggering eviction) + 3 correctness tests (NoCorruption: state check after full fill; SizeStable: map never exceeds 65536 after 75776 Puts; HitRate: measures LRU eviction accuracy, asserts hit_rate > 60%); `make bench-conntrack` runs benchmarks + correctness tests via SSH (CAP_BPF required)
- P5-BENCH-03: Event ingest bench — `cp/internal/ingest/bench_ingest_test.go`: 3 direct COPY benchmarks (100/500/2000 rows/batch, report rows/s + ms/batch) + 4 full-pipeline throughput benchmarks (1k/5k/10k/50k events/IngestLogEvents call, report events/s + peak_queue_depth); requires `QF_BENCH_DSN` env var (auto-skip if not set); `ensureTodayPartition` creates today's log_events partition; `waitDrain` waits for logCh to empty after b.N iterations; `make bench-ingest` via SSH to remote with PostgreSQL access
- P5-BENCH-02: Bundle fan-out bench — `cp/internal/agentsrv/bench_fanout_test.go`: 4 benchmarks sweeping concurrency 1/50/100/200 dispatcher goroutines across 1000 stub agents; `stubStream` fake implements `AgentService_StreamServer` (no gRPC transport, no DB); per-agent delivery latency measured from dispatch start to `Send()` call inside stub; reports p50/p95/p99/max in ms via `b.ReportMetric`; `make bench-fanout` runs locally (pure Go, no eBPF)
- P5-BENCH-01: BPF datapath bench — `agent/internal/loader/bench_bpf_test.go`: 7 benchmarks (Baseline, HotPath_Established, ColdPath 8/32/64 LastMatch, ColdPath 8/64 NoMatch) using `prog.Benchmark(pkt, b.N, b.ResetTimer)` (kernel-measured CPU time via `BPF_PROG_TEST_RUN`); reports ns/pkt + pps via `b.ReportMetric`; helper signatures in `loader_test.go` widened from `*testing.T` to `testing.TB`; `make bench-bpf` runs via SSH on remote Linux with CAP_BPF

### Added (Phase 4)
- P4-UI-01..13: Web UI — React 19 + TypeScript + Vite + Mantine 7 + TanStack Query 5 + TanStack Table 8; `ui/` scaffold (package.json, vite.config.ts, postcss, tsconfig); `cp/internal/embeddedui/embed.go`: go:embed all:dist, SPA fallback handler; chi router serves `/app/*` from embedded FS, `/` redirects to `/app/`; `make ui-build` (npm install + vite build to embeddedui/dist); login page (email+password + OIDC button), httpOnly cookie auth, axios 401→redirect interceptor, RouteGuard; AppShell layout (sidebar 10 nav items, header user menu + logout); Dashboard (host status cards + bar chart + recent audit events); Hosts (TanStack Table, search/filter, detail tabs: Overview/Events/Flows/Counters); Policies (list, create/edit form, rule editor, dry-run preview modal + diff accordion, version history + revert); Object Groups (IPSet/PortSet/HostSet tabs, CRUD); Default Policy (ingress/egress action selectors, Allow→Deny warning modal); Events (SSE live tail via EventSource, pause-on-hover, live↔history toggle, action filter); Flows explorer; Audit log (before/after JSON expand, CSV export); Tokens (create modal with install snippet, revoke); Users & Roles (Admin-only, OIDC badge, edit role + reset password)

- P4-BE-01: SSE live tail — `cp/internal/pubsub/hub.go`: thread-safe per-key fan-out hub (Subscribe/Publish/cancel); `Ingester.hub` field + `publishToHub` publishes each log event as RFC3339Nano-id SSE message; `GET /hosts/{id}/events/stream`: text/event-stream, Last-Event-ID reconnect flushes DB history then subscribes hub; RouterConfig.Hub wired
- P4-BE-02: Policy dry-run — `cp/internal/policy/dryrun.go`: `DryRunRuleSpec`, `HostDiff`, `PreviewResult`; `CompileWithOverride` replaces one policy's DB rules with in-memory specs; `DryRunPreview` iterates all hosts, computes before/after, diffs rule-id sets (added/removed/changed); `POST /policies/{id}/preview` handler; RouterConfig.Compiler added
- P4-BE-03: Policy version history — `versions.sql` queries: `InsertConfigVersion` (auto-increment version via subquery), `ListConfigVersions`, `GetConfigVersion`; `snapshotPolicy(policy, rules)→JSON`; `update` handler writes snapshot async after PUT; `GET /policies/{id}/versions` list; `POST /policies/{id}/versions/{v}/revert` loads snapshot, overwrites policy+rules, writes new version
- P4-AUTH-05: OIDC flow — `cp/internal/auth/oidc.go`: `OIDCConfig` (Issuer/ClientID/ClientSecret/RedirectURL), `OIDCConfigFromEnv()` reads QF_OIDC_ISSUER/CLIENT_ID/CLIENT_SECRET; `NewOIDCHandler(ctx, q, secret, tenantID, cfg)` initialises go-oidc provider + verifier; `Login` → CSRF state cookie + redirect; `Callback` → code exchange → ID token verify → lookup/create user (new OIDC users get auditor role) → issue access+refresh cookies → redirect /app; `GET /auth/oidc/enabled` returns JSON; `UpdateUserOIDCSubject` sqlc query; RouterConfig.OIDCHandler+OIDCEnabled; main.go: OIDCConfigFromEnv + NewOIDCHandler wired
- P4-AUTH-01..04,06: Auth & RBAC backend — `cp/migrations/000005_auth.sql`: `users`, `user_roles`, `api_tokens` tables; sqlc queries (`auth.sql`); `cp/internal/auth/`: `jwt.go` (HS256 access 15m + refresh 7d), `middleware.go` (JWTMiddleware cookie+Bearer, RequireRole), `apitoken.go` (APITokenMiddleware, sha256 hash), `bootstrap.go` (EnsureAdminUser from QF_ADMIN_EMAIL+QF_ADMIN_PASSWORD), `handlers.go` (Login/Logout/Refresh/Me), `users.go` (CRUD GET/POST/PATCH/DELETE /users + role endpoints, Admin-only mutate), `apitokens_handler.go` (GET/POST/DELETE /api-tokens, plain token returned once on create); `cp/internal/api/router.go`: RouterConfig{JWTSecret,TenantID}, auth routes public, all other routes behind authAny middleware; `cp/internal/store/tenant.go`: EnsureDefaultTenant; `main.go`: EnsureAdminUser + ephemeral QF_JWT_SECRET fallback

### Added
- P3-EXT-01: Forwarder — `cp/internal/forwarder/`: `Forwarder` interface (`Forward([]LogEvent) error`, `Close() error`), `LogEvent` flat struct; syslog RFC5424 backend (`Open(dsn)` parses `syslog://host:port` (TCP) / `syslog+udp://host:port`, query `facility=local0`); TCP reconnect on write failure; PRI=(facility×8)+severity (deny→ERR/3, other→INFO/6); structured data `[qf@32473 direction="" src="" ... rule_id=""]`; `Ingester.NewWithForwarder`: routes `IngestLogEvents` to forwarder instead of DB; flow/counter/system/audit always go to PostgreSQL; `QF_FORWARDER_DSN` env var in `main.go`
- P3-AGT-07: Agent main rewire — `agent.Agent.RunFull(ctx, RunFullConfig)` runs full gRPC session loop with `RunWithReconnect`; `runSession` orchestrates HandshakeResult→disk replay→SystemEvent goroutines (HeartbeatSender, EventBatcher, CounterPoller, FlowEventCollector, CertRotator); main receive loop dispatches PolicyBundle/ConfigUpdate/DisconnectRequest/CertRenewalResponse; `Start(ctx)` kept for test compat; `main.go` rewritten: config from `/etc/qf/agent.conf`, slog JSON logging, `RunFull` instead of simple drain
- P3-AGT-06: DiskBuffer — `grpcclient/diskbuffer.go`; persists `AgentMessage` protos as `<unix_ns>.pb` in `/var/lib/qf/events/`; bounded 100 MB (drop oldest on overflow); `Write(msg)` + `Replay(ctx, sendFn)` — replay deletes each file on successful send; corrupt files deleted silently
- P3-AGT-05: ConfigUpdate handler — `grpcclient/configupdate.go`; `ApplyConfigUpdate(cfg, hb, eb, cp, fc)` updates HeartbeatSender interval, EventBatcher batch size + max age, CounterPoller interval, FlowEventCollector enabled flag; all nil-safe
- P3-AGT-04: SystemEventEmitter — `grpcclient/systemevent.go`; `SendSystemEvent(stream, type, severity, detail, attrs)`; constants: `EventAgentStarted`, `EventCPConnected`, `EventCPDisconnected`, `EventBundleApplied`, `EventBundleCacheLoaded`, `EventAttachFailed`, `EventCertRotated`
- P3-AGT-03: FlowEventCollector — `grpcclient/flowcollector.go`; periodic conntrack scan when `flow_events_enabled`; `flowKey` struct with fixed `[4]byte` arrays (net.IP not comparable); detects TCP_CLOSED flows + disappeared flows; emits `FlowEvents` batch; `SetEnabled(bool)` for runtime toggle
- P3-AGT-02: CounterPoller — `grpcclient/counterpoller.go`; ticks on `counter_report_interval_ms` (default 60s); reads BPF counters via `loader.Loader.ReadCounters()`; maps index → rule_id from `handler.ApplyResult.Rules` (added `Rules []loader.RuleSpec` field to ApplyResult); sends `CounterUpdate`; BPF read errors skipped silently
- P3-AGT-01: EventBatcher — `grpcclient/eventbatcher.go`; reads `loader.EventReader` in goroutine; flush on `batchSize` events or `maxAgeMs` timer (timer drain + Reset after size-flush); `SetBatchSize`/`SetMaxAgeMs` for runtime ConfigUpdate; reads+resets `qf_suppressed_count` via `loader.Loader` before send; `loaderEventToProto`: `timestamppb.Now()` for Ts (CLOCK_MONOTONIC offset deferred), bpf→proto enum converters for Direction/Action/Protocol/ConntrackState; `uuidBytesToString([16]byte)`
- P3-BPF-01: token bucket rate limiting — `common.h`: `struct token_bucket{last_ns, tokens}`; `maps.h`: `qf_rate_limits` PERCPU_ARRAY (MAX_RULES) + `qf_suppressed_count` PERCPU_ARRAY (1 slot); `tc_filter.c`: `check_rate_limit(rule_idx, rate_limit)` — refill `elapsed*rate/1e9` tokens (cap elapsed 1s), suppress + increment `qf_suppressed_count` when empty; `emit_log` + `apply_action` take `rule_idx` + `rate_limit`; Go stubs: `TcFilterTokenBucket`, `QfRateLimit`/`QfSuppressedCount` map fields; `loader/counters.go`: `ReadSuppressedCount`, `ResetSuppressedCount` (PERCPU aggregate + zero-reset)
- P3-INGEST-02: gRPC telemetry handlers — `agentsrv/handlers.go`: `handleLogEvents`, `handleFlowEvents`, `handleCounterUpdate`, `handleSystemEvent` (best-effort, no stream close on error); `session.go` `handleMessage` dispatches 4 new cases; `AgentServer` carries `*ingest.Ingester` field; `NewMTLSServer` + `NewAgentServer` accept ingester
- P3-INGEST-03: PartitionManager — `cp/internal/ingest/partitions.go`; runs on startup + 24h tick; creates daily partitions 7 days ahead for log_events/flow_events, weekly 2 weeks ahead for counter_snapshots; drops expired partitions (log 7d, flow 14d, counter 30d); DELETEs expired system_events rows (30d TTL); `validPartitionName` regex guards DDL; started from `main.go`

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
- SSH MCP server (`mcp-ssh-manager`) configured for remote Linux hosts
- buf 1.70.0 for proto lint + codegen

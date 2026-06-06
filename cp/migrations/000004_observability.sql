-- +goose Up

-- ── Log events: per-packet events from BPF ring buffer ───────────────────
-- Partitioned daily; partition manager (P3-INGEST-03) extends and prunes.
-- No FK on host_id: host is validated at stream-auth level; immutable telemetry.
CREATE TABLE log_events (
    id          UUID        NOT NULL DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    host_id     UUID        NOT NULL,
    rule_id     UUID,
    policy_id   UUID,
    direction   TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    protocol    SMALLINT    NOT NULL DEFAULT 0,
    src_ip      INET,
    src_port    INT,
    dst_ip      INET,
    dst_port    INT,
    packet_size INT,
    tcp_flags   INT,
    ct_state    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

CREATE INDEX log_events_host_time ON log_events (host_id, created_at);
CREATE INDEX log_events_rule_time ON log_events (rule_id, created_at) WHERE rule_id IS NOT NULL;

-- ── Flow events: per-connection events from conntrack ────────────────────
-- Partitioned daily.
CREATE TABLE flow_events (
    id            UUID        NOT NULL DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL,
    host_id       UUID        NOT NULL,
    protocol      SMALLINT    NOT NULL DEFAULT 0,
    src_ip        INET,
    src_port      INT,
    dst_ip        INET,
    dst_port      INT,
    bytes_orig    BIGINT      NOT NULL DEFAULT 0,
    bytes_reply   BIGINT      NOT NULL DEFAULT 0,
    packets_orig  BIGINT      NOT NULL DEFAULT 0,
    packets_reply BIGINT      NOT NULL DEFAULT 0,
    final_state   TEXT,
    started_at    TIMESTAMPTZ,
    ended_at      TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (created_at);

CREATE INDEX flow_events_host_time ON flow_events (host_id, created_at);

-- ── Counter snapshots: per-rule packet/byte counts ───────────────────────
-- One row per rule per CounterUpdate snapshot. Partitioned weekly.
-- ts = snapshot timestamp from the agent (not insert time).
CREATE TABLE counter_snapshots (
    id        UUID        NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID        NOT NULL,
    host_id   UUID        NOT NULL,
    rule_id   UUID        NOT NULL,
    policy_id UUID,
    packets   BIGINT      NOT NULL DEFAULT 0,
    bytes     BIGINT      NOT NULL DEFAULT 0,
    ts        TIMESTAMPTZ NOT NULL
) PARTITION BY RANGE (ts);

CREATE INDEX counter_snapshots_host_rule ON counter_snapshots (host_id, rule_id, ts);
CREATE INDEX counter_snapshots_host_ts   ON counter_snapshots (host_id, ts);

-- ── System events: agent lifecycle events, low-volume ────────────────────
-- Not partitioned; TTL enforced by periodic DELETE in partition manager.
CREATE TABLE system_events (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id),
    host_id    UUID        NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,
    severity   TEXT        NOT NULL DEFAULT 'info'
                           CHECK (severity IN ('info','warning','error')),
    detail     TEXT        NOT NULL DEFAULT '',
    attributes JSONB       NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX system_events_host_time ON system_events (host_id, created_at);
CREATE INDEX system_events_type      ON system_events (type, created_at);

-- ── Seed initial partitions ───────────────────────────────────────────────

-- Daily partitions for log_events: today + 30 days forward
-- +goose StatementBegin
DO $$
DECLARE
    d      DATE;
    d_next DATE;
    tbl    TEXT;
BEGIN
    FOR i IN 0..30 LOOP
        d      := CURRENT_DATE + i;
        d_next := d + 1;
        tbl    := 'log_events_' || to_char(d, 'YYYY_MM_DD');
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF log_events
             FOR VALUES FROM (%L) TO (%L)',
            tbl, d::TIMESTAMPTZ, d_next::TIMESTAMPTZ
        );
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- Daily partitions for flow_events: today + 30 days forward
-- +goose StatementBegin
DO $$
DECLARE
    d      DATE;
    d_next DATE;
    tbl    TEXT;
BEGIN
    FOR i IN 0..30 LOOP
        d      := CURRENT_DATE + i;
        d_next := d + 1;
        tbl    := 'flow_events_' || to_char(d, 'YYYY_MM_DD');
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF flow_events
             FOR VALUES FROM (%L) TO (%L)',
            tbl, d::TIMESTAMPTZ, d_next::TIMESTAMPTZ
        );
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- Weekly partitions for counter_snapshots: current ISO week + 8 weeks forward
-- +goose StatementBegin
DO $$
DECLARE
    w      DATE;
    w_next DATE;
    tbl    TEXT;
BEGIN
    w := date_trunc('week', CURRENT_DATE)::DATE;
    FOR i IN 0..7 LOOP
        w_next := w + 7;
        tbl    := 'counter_snapshots_' || to_char(w, 'IYYY_IW');
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF counter_snapshots
             FOR VALUES FROM (%L) TO (%L)',
            tbl, w::TIMESTAMPTZ, w_next::TIMESTAMPTZ
        );
        w := w_next;
    END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE IF EXISTS system_events;
DROP TABLE IF EXISTS counter_snapshots;  -- drops all weekly partition children
DROP TABLE IF EXISTS flow_events;        -- drops all daily partition children
DROP TABLE IF EXISTS log_events;         -- drops all daily partition children

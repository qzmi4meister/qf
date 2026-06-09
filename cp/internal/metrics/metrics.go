// Package metrics defines the shared Prometheus metrics for the CP.
// All metrics are registered on the default registry via init().
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// ActiveStreams is the number of currently connected agent gRPC streams.
	ActiveStreams = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "qf",
		Name:      "grpc_active_streams",
		Help:      "Number of active mTLS agent gRPC streams.",
	})

	// BundlePushDuration tracks the latency of bundle fan-out to one stream.
	BundlePushDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "qf",
		Name:      "bundle_push_duration_seconds",
		Help:      "Latency of sending a PolicyBundle to one agent stream.",
		Buckets:   prometheus.DefBuckets,
	})

	// EventsIngested counts telemetry rows successfully flushed to PostgreSQL.
	EventsIngested = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qf",
		Name:      "events_ingested_total",
		Help:      "Total telemetry rows bulk-inserted into PostgreSQL.",
	}, []string{"type"}) // type: log | flow | counter | system

	// AgentCPU tracks the last-reported CPU % per host.
	AgentCPU = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qf",
		Name:      "agent_cpu_percent",
		Help:      "Agent CPU utilisation from last heartbeat.",
	}, []string{"host_id"})

	// AgentMemBytes tracks the last-reported memory usage per host.
	AgentMemBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qf",
		Name:      "agent_mem_bytes",
		Help:      "Agent memory usage in bytes from last heartbeat.",
	}, []string{"host_id"})

	// AgentConntrackUtil tracks BPF conntrack map utilisation per host.
	AgentConntrackUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "qf",
		Name:      "agent_conntrack_utilization",
		Help:      "Agent BPF conntrack map fill ratio (0–1) from last heartbeat.",
	}, []string{"host_id"})

	// EventsDropped counts telemetry events dropped due to full ingest channels.
	EventsDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "qf",
		Name:      "ingester_events_dropped_total",
		Help:      "Total telemetry events dropped because the ingest channel was full.",
	}, []string{"type"}) // type: log | flow | counter | system

	// DBPoolAcquired is the number of connections currently acquired from the pool.
	DBPoolAcquired = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "qf",
		Name:      "db_pool_acquired_conns",
		Help:      "Number of DB connections currently acquired from the pgx pool.",
	})

	// DBPoolIdle is the number of idle connections in the pool.
	DBPoolIdle = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "qf",
		Name:      "db_pool_idle_conns",
		Help:      "Number of idle DB connections in the pgx pool.",
	})
)

func init() {
	prometheus.MustRegister(
		ActiveStreams,
		BundlePushDuration,
		EventsIngested,
		EventsDropped,
		AgentCPU,
		AgentMemBytes,
		AgentConntrackUtil,
		DBPoolAcquired,
		DBPoolIdle,
	)
}

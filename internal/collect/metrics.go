package collect

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ingestEntriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "ingest_entries_total",
		Help:      "Total entries ingested, by source.",
	}, []string{"source"})

	ingestRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "ingest_requests_total",
		Help:      "Total ingest requests, by status.",
	}, []string{"status"})

	ingestDroppedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "ingest_dropped_total",
		Help:      "Total entries dropped during validation.",
	})

	ingestDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "ingest_duration_seconds",
		Help:      "Ingest request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	ingestTokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "ingest_tokens_total",
		Help:      "Total tokens ingested, by type (input or output).",
	}, []string{"type"})

	batchFlushDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "batch_flush_duration_seconds",
		Help:      "Batch flush duration to DuckDB in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	activeSessions = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "active_sessions",
		Help:      "Number of currently active sessions.",
	})

	activeAgents = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "active_agents",
		Help:      "Number of currently active agents.",
	})

	dbSizeBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "db_size_bytes",
		Help:      "DuckDB database file size in bytes.",
	})

	wsConnectionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "collector",
		Name:      "ws_connections_active",
		Help:      "Number of active WebSocket connections.",
	})
)

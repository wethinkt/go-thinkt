package export

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	exportEntriesShipped = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "entries_shipped_total",
		Help:      "Total entries successfully shipped to collector, by source.",
	}, []string{"source"})

	exportEntriesFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "entries_failed_total",
		Help:      "Total entries that failed to ship, by source.",
	}, []string{"source"})

	exportEntriesBuffered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "entries_buffered_total",
		Help:      "Total entries written to disk buffer.",
	})

	shipDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "ship_duration_seconds",
		Help:      "Ship request duration in seconds.",
		Buckets:   prometheus.DefBuckets,
	})

	shipRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "ship_requests_total",
		Help:      "Total ship requests by status.",
	}, []string{"status"})

	bufferSizeBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "buffer_size_bytes",
		Help:      "Current disk buffer size in bytes.",
	})

	watchedDirs = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "watched_dirs",
		Help:      "Number of directories being watched.",
	})

	fileEventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "file_events_total",
		Help:      "Total file events processed, by source.",
	}, []string{"source"})

	bufferDrainedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "exporter",
		Name:      "buffer_drained_total",
		Help:      "Total payloads successfully drained from disk buffer.",
	})
)

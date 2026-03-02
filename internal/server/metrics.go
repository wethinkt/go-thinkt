package server

import (
	"bufio"
	"bytes"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/expfmt"
)

var (
	apiRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "thinkt",
		Subsystem: "api",
		Name:      "http_requests_total",
		Help:      "Total HTTP API requests by method, route, and status.",
	}, []string{"method", "route", "status"})

	apiRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "thinkt",
		Subsystem: "api",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP API request duration in seconds by method and route.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	apiRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thinkt",
		Subsystem: "api",
		Name:      "http_requests_in_flight",
		Help:      "Current number of in-flight HTTP API requests.",
	})
)

func apiMetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiRequestsInFlight.Inc()
		defer apiRequestsInFlight.Dec()

		ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}

		route := "unknown"
		if rc := chi.RouteContext(r.Context()); rc != nil {
			if pattern := rc.RoutePattern(); pattern != "" {
				route = pattern
			}
		}

		method := r.Method
		apiRequestsTotal.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
		apiRequestDurationSeconds.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	})
}

func combinedMetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			http.Error(w, "failed to gather metrics", http.StatusInternalServerError)
			return
		}

		var out bytes.Buffer
		for _, mf := range mfs {
			if _, err := expfmt.MetricFamilyToText(&out, mf); err != nil {
				http.Error(w, "failed to encode metrics", http.StatusInternalServerError)
				return
			}
		}

		// Include indexer metrics when available over RPC.
		// Prefix names to avoid collisions with API/runtime metrics.
		if text, err := indexerMetrics(); err == nil && text != "" {
			if out.Len() > 0 && out.Bytes()[out.Len()-1] != '\n' {
				out.WriteByte('\n')
			}
			out.WriteString(prefixPrometheusMetricNames(text, "thinkt_indexer_"))
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write(out.Bytes())
	})
}

func prefixPrometheusMetricNames(text, prefix string) string {
	if prefix == "" || text == "" {
		return text
	}

	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "# HELP "):
			parts := strings.SplitN(line, " ", 4)
			if len(parts) == 4 {
				line = "# HELP " + prefixMetricName(parts[2], prefix) + " " + parts[3]
			}
		case strings.HasPrefix(line, "# TYPE "):
			parts := strings.SplitN(line, " ", 4)
			if len(parts) == 4 {
				line = "# TYPE " + prefixMetricName(parts[2], prefix) + " " + parts[3]
			}
		case len(line) == 0 || strings.HasPrefix(line, "#"):
			// Leave blank lines and other comments untouched.
		default:
			i := strings.IndexAny(line, " \t")
			if i > 0 {
				nameWithLabels := line[:i]
				rest := line[i:]
				if brace := strings.IndexByte(nameWithLabels, '{'); brace >= 0 {
					name := nameWithLabels[:brace]
					labels := nameWithLabels[brace:]
					line = prefixMetricName(name, prefix) + labels + rest
				} else {
					line = prefixMetricName(nameWithLabels, prefix) + rest
				}
			}
		}

		out.WriteString(line)
		out.WriteByte('\n')
	}

	return out.String()
}

func prefixMetricName(name, prefix string) string {
	if name == "" || strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

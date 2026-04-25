package metrics

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ConnectionsEvaluated tracks the total number of outbound network events,
	// sliced by the K8s namespace, destination country, the action taken, and the policy name.
	ConnectionsEvaluated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sovereign_connections_total",
			Help: "Total number of outbound connections evaluated by the sensor",
		},
		[]string{"namespace", "dst_country", "action", "policy_name"},
	)

	// EvaluationDuration measures how long our Go pipeline takes to process a single event
	// (GeoIP lookup + Policy matching). This proves to users that our agent is fast.
	EvaluationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "sovereign_evaluation_duration_seconds",
			Help:    "Time taken to geo-resolve and evaluate a connection",
			Buckets: prometheus.DefBuckets, // Default buckets are generally fine for this
		},
	)
)

// StartMetricsServer spins up a lightweight HTTP server to expose Prometheus metrics.
// This runs in a non-blocking goroutine.
func StartMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	slog.Info("Starting Prometheus metrics server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("Prometheus metrics server failed", "err", err)
	}
}

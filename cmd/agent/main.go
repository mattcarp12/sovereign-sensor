package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mattcarp12/sovereign-sensor/pkg/event"
	"github.com/mattcarp12/sovereign-sensor/pkg/geo"
	"github.com/mattcarp12/sovereign-sensor/pkg/k8s"
	"github.com/mattcarp12/sovereign-sensor/pkg/metrics"
	"github.com/mattcarp12/sovereign-sensor/pkg/policy"
	"github.com/prometheus/client_golang/prometheus"
)

type OutputPayload struct {
	event.SovereignEvent
	Verdict *policy.Verdict `json:"verdict,omitempty"`
}

func main() {
	// ─── Context & Graceful Shutdown ──────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupShutdownHandler(cancel)

	// ─── Initialize Core Components ───────────────────────────────
	slog.Info("Initializing Sovereign Sensor components...")

	geoip, err := geo.NewGeoIP()
	if err != nil {
		slog.Error("Failed to load MaxMind DB", "err", err)
		os.Exit(1)
	}
	defer geoip.Close()

	matcher := policy.NewMatcher()
	evaluator := policy.NewEvaluator(matcher, slog.Default())

	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9091" // change default away from 9090
	}
	go metrics.StartMetricsServer(metricsAddr)

	// Start watching Kubernetes for policy changes in the background
	go k8s.WatchPolicies(ctx.Done(), matcher)

	// Create a buffered channel to hold events flowing from Tetragon
	eventsChan := make(chan event.SovereignEvent, 1000)

	// Start the Tetragon consumer in the background
	serverAddr := os.Getenv("TETRAGON_SERVER")
	if serverAddr == "" {
		serverAddr = "127.0.0.1:54321"
	}
	go k8s.StreamTetragonEvents(ctx, serverAddr, eventsChan)

	slog.Info("Pipeline active. Waiting for traffic...")
	enc := json.NewEncoder(os.Stdout)

	// Range over the channel. This loop blocks until a new event arrives.
	for ev := range eventsChan {

		timer := prometheus.NewTimer(metrics.EvaluationDuration)

		// Step A: Enrich GeoLocation
		if ev.DestIP != "unknown" {
			if country, err := geoip.LookupCountry(ev.DestIP); err == nil {
				ev.DestCountry = country
			}
		}

		// Step B: Evaluate Policy
		verdict := evaluator.Evaluate(&ev)

		timer.ObserveDuration()
		metrics.ConnectionsEvaluated.WithLabelValues(ev.Namespace, ev.DestCountry, string(verdict.Action), verdict.PolicyName).Inc()

		// Step C: Output
		payload := OutputPayload{SovereignEvent: ev, Verdict: &verdict}
		if err := enc.Encode(payload); err != nil {
			slog.Warn("Failed to encode JSON", "err", err)
		}
	}
}

// ─── Helpers ────────────────────────────────────────────────────────

func setupShutdownHandler(cancel context.CancelFunc) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		slog.Info("\nShutting down gracefully...")
		cancel()                    // This cancels the context, which cleanly stops the Tetragon gRPC loop
		time.Sleep(1 * time.Second) // Give goroutines a moment to exit
		os.Exit(0)
	}()
}

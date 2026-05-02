package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mattcarp12/sovereign-sensor/api/v1alpha1" // Import your CRDs
	"github.com/mattcarp12/sovereign-sensor/internal/event"
	"github.com/mattcarp12/sovereign-sensor/internal/geo"
	"github.com/mattcarp12/sovereign-sensor/internal/k8s"
	"github.com/mattcarp12/sovereign-sensor/internal/metrics"
	"github.com/mattcarp12/sovereign-sensor/internal/policy"
	"github.com/prometheus/client_golang/prometheus"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type OutputPayload struct {
	event.SovereignEvent
	Verdict *policy.Verdict `json:"verdict,omitempty"`
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupShutdownHandler(cancel)

	slog.Info("Initializing Sovereign Sensor components...")

	// 1. Setup K8s Client & Scheme
	scheme := runtime.NewScheme()
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	k8sConfig := config.GetConfigOrDie()
	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		slog.Error("Failed to create K8s client", "err", err)
		os.Exit(1)
	}

	// 2. Initialize Core Components
	geoip, err := geo.NewGeoIP()
	if err != nil {
		slog.Error("Failed to load MaxMind DB", "err", err)
		os.Exit(1)
	}
	defer geoip.Close()

	matcher := policy.NewMatcher()
	evaluator := policy.NewEvaluator(matcher) // No longer takes a logger
	reporter := policy.NewReporter(k8sClient)

	// 3. Start Background Processes
	go metrics.StartMetricsServer(":9091")
	go k8s.WatchPolicies(ctx, k8sClient, matcher)

	eventsChan := make(chan event.SovereignEvent, 1000)
	serverAddr := os.Getenv("TETRAGON_SERVER")
	if serverAddr == "" {
		serverAddr = "127.0.0.1:54321"
	}
	go k8s.StreamTetragonEvents(ctx, serverAddr, eventsChan)

	slog.Info("Pipeline active. Waiting for traffic...")
	enc := json.NewEncoder(os.Stdout)

	// 4. The Main Event Loop
	for ev := range eventsChan {
		timer := prometheus.NewTimer(metrics.EvaluationDuration)

		if ev.DestIP != "unknown" {
			if country, err := geoip.LookupCountry(ev.DestIP); err == nil {
				ev.DestCountry = country
			}
		}

		verdict := evaluator.Evaluate(&ev)

		// Record Metrics
		actionStr := "allow"
		if len(verdict.Actions) > 0 {
			actionStr = string(verdict.Actions[0]) // Just record the primary action for Prometheus
		}
		metrics.ConnectionsEvaluated.WithLabelValues(ev.Namespace, ev.DestCountry, actionStr, verdict.PolicyName).Inc()
		timer.ObserveDuration()

		// Handle Side Effects (Logging & Reporting)
		if verdict.Violated {
			slog.Warn("SOVEREIGNTY VIOLATION",
				"pod", ev.PodName,
				"namespace", ev.Namespace,
				"dst_ip", ev.DestIP,
				"dst_country", ev.DestCountry,
				"policy", verdict.PolicyName,
				"actions", verdict.Actions,
			)

			// If the policy requires blocking, report the IP to the Control Plane
			for _, action := range verdict.Actions {
				if action == "block-kill" || action == "block-noconn" {
					if err := reporter.ReportViolator(ctx, verdict.PolicyName, ev.Namespace, ev.DestIP); err != nil {
						slog.Error("Failed to report violator to K8s API", "err", err)
					}
					break
				}
			}
		}

		// Output JSON payload for debugging/fluentbit
		payload := OutputPayload{SovereignEvent: ev, Verdict: &verdict}
		if err := enc.Encode(payload); err != nil {
			slog.Warn("Failed to encode JSON", "err", err)
		}
	}
}

func setupShutdownHandler(cancel context.CancelFunc) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		slog.Info("\nShutting down gracefully...")
		cancel()
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}

package k8s

import (
	"context"
	"log/slog"
	"time"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"github.com/mattcarp12/sovereign-sensor/internal/policy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WatchPolicies polls the Kubernetes API for SovereigntyPolicies and updates the Matcher.
// Note: While a true K8s Informer/Watch stream is slightly more efficient, 
// a fast polling loop using the controller-runtime caching client is exceptionally 
// lightweight and vastly simpler to implement without the full Manager framework.
func WatchPolicies(ctx context.Context, c client.Client, matcher *policy.Matcher) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	slog.Info("Starting Kubernetes policy watcher...")

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping K8s policy watcher")
			return
		case <-ticker.C:
			var policyList secv1alpha1.SovereigntyPolicyList
			
			// Because 'c' is a controller-runtime caching client, this Get/List 
			// is usually served directly from memory and doesn't hammer the API server.
			if err := c.List(ctx, &policyList); err != nil {
				slog.Error("Failed to fetch policies from K8s", "err", err)
				continue
			}

			// Update the thread-safe matcher with the fresh state of the world
			matcher.UpdatePolicies(policyList.Items)
		}
	}
}
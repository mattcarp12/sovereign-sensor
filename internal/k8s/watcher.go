package k8s

import (
	"context"
	"log/slog"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"github.com/mattcarp12/sovereign-sensor/internal/policy"

	k8scache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// WatchPolicies attaches to the controller-runtime cache Informer.
// It receives instant push notifications from the K8s API server when policies change.
func WatchPolicies(ctx context.Context, c cache.Cache, matcher *policy.Matcher) {
	slog.Info("Starting native Kubernetes Informer for policies...")

	// Request the informer for our specific CRD
	informer, err := c.GetInformer(ctx, &secv1alpha1.SovereigntyPolicy{})
	if err != nil {
		slog.Error("Failed to get Informer", "err", err)
		return
	}

	// Register the callback functions for the Watch events
	_, err = informer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			p := obj.(*secv1alpha1.SovereigntyPolicy)
			matcher.UpsertPolicy(p)
			slog.Info("Policy added to memory", "name", p.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			p := newObj.(*secv1alpha1.SovereigntyPolicy)
			matcher.UpsertPolicy(p)
			slog.Info("Policy updated in memory", "name", p.Name)
		},
		DeleteFunc: func(obj interface{}) {
			p, ok := obj.(*secv1alpha1.SovereigntyPolicy)
			if !ok {
				// Handle edge case where the watch disconnected right as the object was deleted
				tombstone, ok := obj.(k8scache.DeletedFinalStateUnknown)
				if !ok {
					slog.Error("Failed to decode deleted object tombstone")
					return
				}
				p, ok = tombstone.Obj.(*secv1alpha1.SovereigntyPolicy)
				if !ok {
					slog.Error("Tombstone contained unexpected object type")
					return
				}
			}
			matcher.RemovePolicy(p)
			slog.Info("Policy removed from memory", "name", p.Name)
		},
	})

	if err != nil {
		slog.Error("Failed to add event handlers to Informer", "err", err)
		return
	}

	// Block until context is cancelled
	<-ctx.Done()
}

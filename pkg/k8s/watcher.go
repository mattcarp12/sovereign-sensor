package k8s

import (
	"log/slog"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/mattcarp12/sovereign-sensor/pkg/policy"
)

// WatchPolicies connects to K8s and pipes CRD changes directly into the Matcher
func WatchPolicies(stopCh <-chan struct{}, matcher *policy.Matcher) {
	config, err := getKubeConfig() // Reusing the auth helper we built earlier
	if err != nil {
		slog.Error("Failed to get kubeconfig for watcher", "err", err)
		return
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		slog.Error("Failed to create dynamic client for watcher", "err", err)
		return
	}

	// Target our exact CRD group/version
	gvr := schema.GroupVersionResource{
		Group:    "sec.sovereign.io",
		Version:  "v1alpha1",
		Resource: "sovereigntypolicies",
	}

	// Create an informer that polls/watches the API every 10 minutes (and instantly on K8s events)
	factory := dynamicinformer.NewDynamicSharedInformerFactory(dynClient, 10*time.Minute)
	informer := factory.ForResource(gvr).Informer()

	// Wire up the K8s events to our thread-safe Matcher
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if p := parseUnstructured(obj); p != nil {
				slog.Info("Policy Added", "name", p.Name)
				matcher.UpsertPolicy(p)
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			if p := parseUnstructured(newObj); p != nil {
				slog.Info("Policy Updated", "name", p.Name)
				matcher.UpsertPolicy(p)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if p := parseUnstructured(obj); p != nil {
				slog.Info("Policy Deleted", "name", p.Name)
				matcher.RemovePolicy(p)
			}
		},
	})
	if err != nil {
		slog.Error("Failed to add event handlers", "err", err)
		return
	}

	slog.Info("Starting SovereigntyPolicy Watcher...")
	informer.Run(stopCh)
}

// parseUnstructured safely extracts the spec from the raw K8s JSON
func parseUnstructured(obj interface{}) *policy.Policy {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return nil
	}

	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	p := &policy.Policy{
		Name: u.GetName(),
	}

	if action, ok := spec["action"].(string); ok {
		p.Action = policy.Action(action)
	}

	if namespaces, ok := spec["namespaces"].([]interface{}); ok {
		for _, ns := range namespaces {
			p.Namespaces = append(p.Namespaces, ns.(string))
		}
	}

	if countries, ok := spec["allowedCountries"].([]interface{}); ok {
		for _, c := range countries {
			p.AllowedCountries = append(p.AllowedCountries, c.(string))
		}
	}

	return p
}

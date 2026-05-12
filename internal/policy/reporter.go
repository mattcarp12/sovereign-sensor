package policy

import (
	"context"
	"slices"
	"sync"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"github.com/mattcarp12/sovereign-sensor/internal/event"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PolicyReporter struct {
	K8sClient   client.Client
	Recorder    record.EventRecorder
	reportedIPs map[string]bool
	mu          sync.Mutex
}

func NewPolicyReporter(c client.Client, r record.EventRecorder) *PolicyReporter {
	return &PolicyReporter{
		K8sClient:   c,
		Recorder:    r,
		reportedIPs: make(map[string]bool),
	}
}

func (pr *PolicyReporter) ReportViolator(ctx context.Context, policyName, violatorIP string) error {
	cacheKey := policyName + ":" + violatorIP

	// 1. Local Cache Check
	pr.mu.Lock()
	if pr.reportedIPs[cacheKey] {
		pr.mu.Unlock()
		return nil // We already reported this; skip the network call entirely
	}
	// Optimistically mark as reported to block the thundering herd from THIS pod
	pr.reportedIPs[cacheKey] = true
	pr.mu.Unlock()

	// 2. K8s API Update with guaranteed retry logic
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current secv1alpha1.SovereigntyPolicy
		if err := pr.K8sClient.Get(ctx, types.NamespacedName{Name: policyName}, &current); err != nil {
			return err
		}

		// Check if another agent pod already beat us to adding this IP
		if slices.Contains(current.Status.DiscoveredViolatorIPs, violatorIP) {
			return nil
		}

		current.Status.DiscoveredViolatorIPs = append(current.Status.DiscoveredViolatorIPs, violatorIP)
		return pr.K8sClient.Status().Update(ctx, &current)
	})

	// 3. Rollback cache on definitive failure
	if err != nil {
		// If the policy was simply deleted, we don't need to roll back
		if !apierrors.IsNotFound(err) {
			pr.mu.Lock()
			delete(pr.reportedIPs, cacheKey)
			pr.mu.Unlock()
		}
		return err
	}

	return nil
}

func (pr *PolicyReporter) EmitViolationEvent(ev event.SovereignEvent, policyName string, action string) {
	if ev.PodName == "" || ev.Namespace == "" {
		return
	}

	podRef := &corev1.ObjectReference{
		Kind:      "Pod",
		Name:      ev.PodName,
		Namespace: ev.Namespace,
	}

	pr.Recorder.Eventf(
		podRef,
		corev1.EventTypeWarning,
		"SovereigntyViolation",
		"Action: %s | Blocked connection to %s (%s)[%s] by policy %s",
		action, ev.DestIP, ev.DestCountry, ev.DestCountryName, policyName,
	)
}

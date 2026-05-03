package policy

import (
	"context"
	"sync"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1" // Your event struct
	"github.com/mattcarp12/sovereign-sensor/internal/event"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
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

// ReportViolator uses Server-Side Apply (SSA) to atomically append an IP to the cluster-scoped policy.
func (pr *PolicyReporter) ReportViolator(ctx context.Context, policyName, violatorIP string) error {
	// Local node deduplication
	pr.mu.Lock()
	cacheKey := policyName + ":" + violatorIP
	if pr.reportedIPs[cacheKey] {
		pr.mu.Unlock()
		return nil
	}
	pr.reportedIPs[cacheKey] = true
	pr.mu.Unlock()

	// Construct a partial object containing ONLY the data we want to merge
	patchObj := &secv1alpha1.SovereigntyPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "sec.sovereign.io/v1alpha1",
			Kind:       "SovereigntyPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: policyName,
		},
		Status: secv1alpha1.SovereigntyPolicyStatus{
			DiscoveredViolatorIPs: []string{violatorIP},
		},
	}

	return pr.K8sClient.Status().Patch(
		ctx,
		patchObj,
		client.Apply,
		client.FieldOwner("sovereign-sensor-agent"),
		client.ForceOwnership,
	)
}

// EmitViolationEvent creates a native Kubernetes Event attached to the violating Pod
func (pr *PolicyReporter) EmitViolationEvent(ev event.SovereignEvent, policyName string, action string) {
	// Construct a lightweight reference to the Pod that caused the violation
	podRef := &corev1.ObjectReference{
		Kind:      "Pod",
		Name:      ev.PodName,
		Namespace: ev.Namespace,
	}

	// This pushes the event directly into etcd, making it searchable via kubectl
	pr.Recorder.Eventf(
		podRef,
		corev1.EventTypeWarning,
		"SovereigntyViolation",
		"Action: %s | Blocked connection to %s (%s) by policy %s",
		action, ev.DestIP, ev.DestCountry, policyName,
	)
}

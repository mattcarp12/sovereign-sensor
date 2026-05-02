package policy

import (
	"context"
	"sync"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reporter struct {
	K8sClient   client.Client
	reportedIPs map[string]bool
	mu          sync.Mutex
}

func NewReporter(c client.Client) *Reporter {
	return &Reporter{
		K8sClient:   c,
		reportedIPs: make(map[string]bool),
	}
}

// ReportViolator uses Server-Side Apply (SSA) to atomically append an IP to the cluster-scoped policy.
func (pr *Reporter) ReportViolator(ctx context.Context, policyName, violatorIP string) error {
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
			// ABSOLUTELY NO NAMESPACE HERE
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

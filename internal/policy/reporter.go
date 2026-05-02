package policy

import (
	"context"
	"sync"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PolicyReporter handles the feedback loop from the Agent back to the Control Plane
type PolicyReporter struct {
	K8sClient client.Client

	// reportedIPs prevents the Agent from spamming the K8s API with the same IP
	reportedIPs map[string]bool
	mu          sync.Mutex
}

func NewReporter(c client.Client) *PolicyReporter {
	return &PolicyReporter{
		K8sClient:   c,
		reportedIPs: make(map[string]bool),
	}
}

// ReportViolator adds the discovered IP to the policy's status so the Operator can block it
func (pr *PolicyReporter) ReportViolator(ctx context.Context, policyName, policyNamespace, violatorIP string) error {
	pr.mu.Lock()
	if pr.reportedIPs[policyName+"/"+violatorIP] {
		pr.mu.Unlock()
		return nil // We already reported this IP, skip API call
	}
	// Mark as reported locally to prevent duplicate concurrent API calls
	pr.reportedIPs[policyName+"/"+violatorIP] = true
	pr.mu.Unlock()

	// 1. Fetch the absolute latest version of the policy from K8s
	var policy secv1alpha1.SovereigntyPolicy
	if err := pr.K8sClient.Get(ctx, client.ObjectKey{Name: policyName, Namespace: policyNamespace}, &policy); err != nil {
		return err
	}

	// 2. Check if another Agent on a different node already reported this IP
	for _, existingIP := range policy.Status.DiscoveredViolatorIPs {
		if existingIP == violatorIP {
			return nil // Already handled globally
		}
	}

	// 3. Append the new IP
	policy.Status.DiscoveredViolatorIPs = append(policy.Status.DiscoveredViolatorIPs, violatorIP)

	// 4. Update the Status subresource
	if err := pr.K8sClient.Status().Update(ctx, &policy); err != nil {
		// If update fails, remove from local cache so we try again on the next packet
		pr.mu.Lock()
		delete(pr.reportedIPs, policyName+"/"+violatorIP)
		pr.mu.Unlock()
		return err
	}

	return nil
}

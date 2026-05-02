// policy.go
package policy

import (
	"sync"

	"github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

// ─── Matcher ──────────────────────────────────────────────────────────────────
// Matcher provides O(1) policy lookup by namespace.
// Built once at startup from the PolicyConfig.
type Matcher struct {
	// exact maps a namespace name to the first matching policy.
	exact map[string]*v1alpha1.SovereigntyPolicy
	// wildcard is the catch-all policy (namespace == "*"), if any.
	wildcard *v1alpha1.SovereigntyPolicy
	// mutex for dynamic updates
	mu sync.RWMutex
}

// NewMatcher compiles a PolicyConfig into a fast lookup structure.
func NewMatcher() *Matcher {
	return &Matcher{
		exact: make(map[string]*v1alpha1.SovereigntyPolicy),
	}
}

// UpdatePolicies completely refreshes the in-memory state based on the K8s API.
// It builds a new map and swaps it instantly to handle both updates and deletions safely.
func (m *Matcher) UpdatePolicies(k8sPolicies []v1alpha1.SovereigntyPolicy) {
	newExact := make(map[string]*v1alpha1.SovereigntyPolicy)
	var newWildcard *v1alpha1.SovereigntyPolicy

	for i := range k8sPolicies {
		// Take a pointer to the item in the array so we don't copy the whole struct
		p := &k8sPolicies[i]
		
		for _, ns := range p.Spec.Namespaces {
			if ns == "*" {
				newWildcard = p
			} else {
				newExact[ns] = p
			}
		}
	}

	// Safely swap the state under a write lock
	m.mu.Lock()
	m.exact = newExact
	m.wildcard = newWildcard
	m.mu.Unlock()
}

// Match returns the policy applicable to a namespace.
// Returns nil if no policy matches (no wildcard defined).
func (m *Matcher) Match(namespace string) *v1alpha1.SovereigntyPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Exact match takes precedence
	if p, ok := m.exact[namespace]; ok {
		return p
	}
	return m.wildcard
}

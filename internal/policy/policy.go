package policy

import (
	"sync"
	"github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

// Matcher provides thread-safe policy lookup.
type Matcher struct {
	mu       sync.RWMutex
	policies map[string]*v1alpha1.SovereigntyPolicy
}

func NewMatcher() *Matcher {
	return &Matcher{
		policies: make(map[string]*v1alpha1.SovereigntyPolicy),
	}
}

// UpsertPolicy adds or updates a policy dynamically
func (m *Matcher) UpsertPolicy(p *v1alpha1.SovereigntyPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[p.Name] = p
}

// RemovePolicy deletes a policy dynamically
func (m *Matcher) RemovePolicy(p *v1alpha1.SovereigntyPolicy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.policies, p.Name)
}

// Match returns ALL policies applicable to a given namespace.
func (m *Matcher) Match(namespace string) []*v1alpha1.SovereigntyPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []*v1alpha1.SovereigntyPolicy

	for _, p := range m.policies {
		for _, ns := range p.Spec.Namespaces {
			if ns == "*" || ns == namespace {
				matches = append(matches, p)
				break // Policy matched this namespace, move to next policy
			}
		}
	}

	return matches
}
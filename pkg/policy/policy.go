// policy.go
package policy

import (
	"sync"
)

// ─── Policy types ─────────────────────────────────────────────────────────────

type Action string

const (
	ActionAllow Action = "allow"
	ActionLog   Action = "log"
	ActionBlock Action = "block"
)

// Policy is a single named sovereignty rule.
type Policy struct {
	Name             string   `yaml:"name"`
	Namespaces       []string `yaml:"namespaces"`
	AllowedCountries []string `yaml:"allowed_countries"`
	Action           Action   `yaml:"action"`
	Description      string   `yaml:"description"`
}

// ─── Matcher ──────────────────────────────────────────────────────────────────
// Matcher provides O(1) policy lookup by namespace.
// Built once at startup from the PolicyConfig.
type Matcher struct {
	// exact maps a namespace name to the first matching policy.
	exact map[string]*Policy
	// wildcard is the catch-all policy (namespace == "*"), if any.
	wildcard *Policy
	// mutex for dynamic updates
	mu sync.RWMutex
}

// NewMatcher compiles a PolicyConfig into a fast lookup structure.
func NewMatcher() *Matcher {
	return &Matcher{
		exact: make(map[string]*Policy),
	}
}

// UpsertPolicy adds or updates a policy dynamically
func (m *Matcher) UpsertPolicy(p *Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ns := range p.Namespaces {
		if ns == "*" {
			m.wildcard = p
		} else {
			m.exact[ns] = p
		}
	}
}

// RemovePolicy deletes a policy dynamically
func (m *Matcher) RemovePolicy(p *Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ns := range p.Namespaces {
		if ns == "*" {
			if m.wildcard != nil && m.wildcard.Name == p.Name {
				m.wildcard = nil
			}
		} else {
			if existing, ok := m.exact[ns]; ok && existing.Name == p.Name {
				delete(m.exact, ns)
			}
		}
	}
}

// Match returns the policy applicable to a namespace.
// Returns nil if no policy matches (no wildcard defined).
func (m *Matcher) Match(namespace string) *Policy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Exact match takes precedence
	if p, ok := m.exact[namespace]; ok {
		return p
	}
	return m.wildcard
}

package policy

import (
	"reflect"
	"testing"

	"github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"github.com/mattcarp12/sovereign-sensor/internal/event"
)

func TestEvaluator_Evaluate(t *testing.T) {
	// 1. Setup the "State of the World"
	// We build a Matcher and inject a few distinct policies to test all logic branches.
	matcher := NewMatcher()
	matcher.UpdatePolicies([]v1alpha1.SovereigntyPolicy{
		{
			// A highly restrictive production policy
			Spec: v1alpha1.SovereigntyPolicySpec{
				Namespaces:          []string{"prod"},
				Actions:             []v1alpha1.Action{v1alpha1.ActionBlock},
				AllowedCountries:    []string{"US", "CA"},
				DisallowedCountries: []string{"RU", "CN"}, // Explicit Deny
			},
		},
		{
			// A permissive dev policy with just a blacklist
			Spec: v1alpha1.SovereigntyPolicySpec{
				Namespaces:          []string{"dev"},
				Actions:             []v1alpha1.Action{v1alpha1.ActionLog},
				AllowedCountries:    []string{}, // No allowlist, everything implicitly allowed except...
				DisallowedCountries: []string{"KP"}, // ...this specific country
			},
		},
	})

	eval := NewEvaluator(matcher)

	// 2. Define the Table of Tests
	tests := []struct {
		name     string
		event    event.SovereignEvent
		expected Verdict
	}{
		{
			name: "Private Address (No Country)",
			event: event.SovereignEvent{
				Namespace:   "prod",
				DestCountry: "", // e.g., 10.0.0.5
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Violated: false, Reason: "private address"},
		},
		{
			name: "Host Process (No Namespace)",
			event: event.SovereignEvent{
				Namespace:   "", // e.g., node agent or kubelet
				DestCountry: "US",
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Violated: false, Reason: "host process / no namespace"},
		},
		{
			name: "No Policy for Namespace",
			event: event.SovereignEvent{
				Namespace:   "unmonitored-ns",
				DestCountry: "RU",
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Violated: false, Reason: "no policy for namespace"},
		},
		{
			name: "Explicit Deny (Disallowed List)",
			event: event.SovereignEvent{
				Namespace:   "prod",
				DestCountry: "RU",
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionBlock}, Violated: true, Reason: "country explicitly in disallowed list"},
		},
		{
			name: "Implicit Deny (Not in Allowed List)",
			event: event.SovereignEvent{
				Namespace:   "prod",
				DestCountry: "GB", // Not in US, CA, RU, or CN
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionBlock}, Violated: true, Reason: "destination country not in allowlist"},
		},
		{
			name: "Explicit Allow",
			event: event.SovereignEvent{
				Namespace:   "prod",
				DestCountry: "CA",
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Violated: false, Reason: "policy permits country"},
		},
		{
			name: "Permissive Policy - Explicit Deny",
			event: event.SovereignEvent{
				Namespace:   "dev",
				DestCountry: "KP",
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionLog}, Violated: true, Reason: "country explicitly in disallowed list"},
		},
		{
			name: "Permissive Policy - Default Allow",
			event: event.SovereignEvent{
				Namespace:   "dev",
				DestCountry: "GB", // dev policy has no allowlist, so anything not in disallowed is OK
			},
			expected: Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Violated: false, Reason: "policy permits country"},
		},
	}

	// 3. Execute the Tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval.Evaluate(&tt.event)

			// We don't check the PolicyName in the struct comparison to keep the test clean,
			// so we just match the core fields.
			if !reflect.DeepEqual(got.Actions, tt.expected.Actions) {
				t.Errorf("Evaluate() Actions = %v, want %v", got.Actions, tt.expected.Actions)
			}
			if got.Violated != tt.expected.Violated {
				t.Errorf("Evaluate() Violated = %v, want %v", got.Violated, tt.expected.Violated)
			}
			if got.Reason != tt.expected.Reason {
				t.Errorf("Evaluate() Reason = %v, want %v", got.Reason, tt.expected.Reason)
			}
		})
	}
}
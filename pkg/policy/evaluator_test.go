package policy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/mattcarp12/sovereign-sensor/pkg/event"
)

func TestEvaluator(t *testing.T) {
	// 1. Set up a dummy policy
	policies := []Policy{
			{
				Name:             "eu-strict",
				Namespaces:       []string{"eu-prod"},
				AllowedCountries: []string{"DE", "FR"}, // Only Germany and France
				Action:           ActionBlock,
			},
			{
				Name:             "default-allow",
				Namespaces:       []string{"*"},
				AllowedCountries: []string{}, // Empty means allow all
				Action:           ActionAllow,
			},
	}

	matcher := NewMatcher()
	matcher.UpsertPolicy(&policies[0])
	matcher.UpsertPolicy(&policies[1])
	// Use a discard logger so we don't spam the test output
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	eval := NewEvaluator(matcher, logger)

	// 2. Table-driven test cases
	tests := []struct {
		name         string
		input        event.SovereignEvent
		wantAction   Action
		wantViolated bool
	}{
		{
			name: "Host process bypassed",
			input: event.SovereignEvent{
				Namespace:   "", // Host processes have no namespace
				DestCountry: "US",
			},
			wantAction:   ActionAllow,
			wantViolated: false,
		},
		{
			name: "Private IP bypassed",
			input: event.SovereignEvent{
				Namespace:   "eu-prod",
				DestCountry: "", // Private IPs return empty string from MaxMind
			},
			wantAction:   ActionAllow,
			wantViolated: false,
		},
		{
			name: "Allowed country in specific policy",
			input: event.SovereignEvent{
				Namespace:   "eu-prod",
				DestCountry: "DE",
			},
			wantAction:   ActionAllow,
			wantViolated: false,
		},
		{
			name: "Blocked country in specific policy",
			input: event.SovereignEvent{
				Namespace:   "eu-prod",
				DestCountry: "US",
			},
			wantAction:   ActionBlock,
			wantViolated: true,
		},
		{
			name: "Wildcard policy allows everything",
			input: event.SovereignEvent{
				Namespace:   "some-other-namespace",
				DestCountry: "RU",
			},
			wantAction:   ActionAllow,
			wantViolated: false,
		},
	}

	// 3. Run the tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval.Evaluate(&tt.input)

			if got.Action != tt.wantAction {
				t.Errorf("Evaluate() Action = %v, want %v", got.Action, tt.wantAction)
			}
			if got.Violated != tt.wantViolated {
				t.Errorf("Evaluate() Violated = %v, want %v", got.Violated, tt.wantViolated)
			}
		})
	}
}

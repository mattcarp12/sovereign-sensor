package policy

import (
	"github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"github.com/mattcarp12/sovereign-sensor/internal/event"
)

// Verdict represents the outcome of a policy evaluation
type Verdict struct {
	Actions    []v1alpha1.Action `json:"actions"`
	PolicyName string            `json:"policy_name,omitempty"`
	Reason     string            `json:"reason"`
	Violated   bool              `json:"violated"`
}

type Evaluator struct {
	matcher *Matcher
}

func NewEvaluator(matcher *Matcher) *Evaluator {
	return &Evaluator{
		matcher: matcher,
	}
}

func (e *Evaluator) Evaluate(ev *event.SovereignEvent) Verdict {
	country := ev.DestCountry

	if country == "" {
		return Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Reason: "private address"}
	}

	if ev.Namespace == "" {
		return Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Reason: "host process / no namespace"}
	}

	// Fetch ALL policies that apply to this namespace
	policies := e.matcher.Match(ev.Namespace)
	if len(policies) == 0 {
		return Verdict{Actions: []v1alpha1.Action{v1alpha1.ActionAllow}, Reason: "no policy for namespace"}
	}

	// EVALUATION CHAIN: The strictest policy (first violation encountered) wins.
	for _, pol := range policies {
		
		// 1. Explicit Deny (DisallowedCountries)
		for _, blocked := range pol.Spec.DisallowedCountries {
			if blocked == country {
				return Verdict{
					Actions:    pol.Spec.Actions,
					PolicyName: pol.Name,
					Reason:     "country explicitly in disallowed list",
					Violated:   true,
				}
			}
		}

		// 2. Implicit Deny (AllowedCountries defined, but not matched)
		if len(pol.Spec.AllowedCountries) > 0 {
			isAllowed := false
			for _, allowed := range pol.Spec.AllowedCountries {
				if allowed == country {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				return Verdict{
					Actions:    pol.Spec.Actions,
					PolicyName: pol.Name,
					Reason:     "destination country not in allowlist",
					Violated:   true,
				}
			}
		}
	}

	// 3. Default Allow (If we survived all matching policies without a violation)
	return Verdict{
		Actions:    []v1alpha1.Action{v1alpha1.ActionAllow},
		PolicyName: "multiple", // Optional: indicates it passed multiple policies
		Reason:     "all applicable policies permit country",
	}
}
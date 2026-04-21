package policy

import (
	"log/slog"

	"github.com/mattcarp12/sovereign-sensor/pkg/event"
)

type Verdict struct {
	Action     Action `json:"action"`
	PolicyName string `json:"policy_name,omitempty"`
	Reason     string `json:"reason"`
	Violated   bool   `json:"violated"`
}

type Evaluator struct {
	matcher *Matcher
	logger  *slog.Logger
}

func NewEvaluator(matcher *Matcher, logger *slog.Logger) *Evaluator {
	return &Evaluator{
		matcher: matcher,
		logger:  logger,
	}
}

func (e *Evaluator) Evaluate(ev *event.SovereignEvent) Verdict {
	// Grab the country that was already attached in main.go
	country := ev.DestCountry

	// 1. Private/internal address — skip policy
	if country == "" {
		return Verdict{Action: ActionAllow, Reason: "private address"}
	}

	// 2. Host processes / no namespace — skip policy
	if ev.Namespace == "" {
		return Verdict{Action: ActionAllow, Reason: "host process / no namespace"}
	}

	// 3. Match namespace to policy
	policy := e.matcher.Match(ev.Namespace)
	if policy == nil {
		return Verdict{Action: ActionAllow, Reason: "no policy for namespace"}
	}

	// 4. Evaluate allowed countries
	if len(policy.AllowedCountries) == 0 {
		return Verdict{
			Action:     ActionAllow,
			PolicyName: policy.Name,
			Reason:     "policy allows all countries",
		}
	}

	for _, allowed := range policy.AllowedCountries {
		if allowed == country {
			return Verdict{
				Action:     ActionAllow,
				PolicyName: policy.Name,
				Reason:     "country in allowlist",
			}
		}
	}

	// 5. Violation
	verdict := Verdict{
		Action:     policy.Action,
		PolicyName: policy.Name,
		Reason:     "destination country not in allowlist",
		Violated:   true,
	}

	attrs := []any{
		"pod", ev.PodName,
		"namespace", ev.Namespace,
		"dst_ip", ev.DestIP,
		"dst_country", country,
		"policy", policy.Name,
		"action", policy.Action,
	}

	switch policy.Action {
	case ActionBlock:
		e.logger.Error("SOVEREIGNTY VIOLATION — BLOCK", attrs...)
	case ActionLog:
		e.logger.Warn("sovereignty violation — log", attrs...)
	}

	return verdict
}

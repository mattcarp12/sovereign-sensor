package policy

import (
	"reflect"
	"testing"

	"github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	"github.com/mattcarp12/sovereign-sensor/internal/event"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// makePolicy is a helper to reduce boilerplate in test cases.
func makePolicy(name string, namespaces, allowed, disallowed []string, actions []v1alpha1.Action) v1alpha1.SovereigntyPolicy {
	return v1alpha1.SovereigntyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1alpha1.SovereigntyPolicySpec{
			Namespaces:          namespaces,
			AllowedCountries:    allowed,
			DisallowedCountries: disallowed,
			Actions:             actions,
		},
	}
}

func makeEvaluator(policies ...v1alpha1.SovereigntyPolicy) *Evaluator {
	m := NewMatcher()
	for i := range policies {
		m.UpsertPolicy(&policies[i])
	}
	return NewEvaluator(m)
}

// ─── Short-circuit cases ──────────────────────────────────────────────────────

func TestEvaluate_PrivateAddress(t *testing.T) {
	eval := makeEvaluator(makePolicy("prod", []string{"prod"}, []string{"US"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: ""})
	assertAllow(t, got, "private address")
}

func TestEvaluate_HostProcess(t *testing.T) {
	eval := makeEvaluator(makePolicy("prod", []string{"prod"}, []string{"US"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "", DestCountry: "RU"})
	assertAllow(t, got, "host process / no namespace")
}

func TestEvaluate_NoPolicyForNamespace(t *testing.T) {
	eval := makeEvaluator(makePolicy("prod", []string{"prod"}, []string{"US"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "unmonitored", DestCountry: "RU"})
	assertAllow(t, got, "no policy for namespace")
}

// ─── Single-policy: explicit deny ────────────────────────────────────────────

func TestEvaluate_ExplicitDeny_Block(t *testing.T) {
	eval := makeEvaluator(makePolicy("prod", []string{"prod"}, nil, []string{"RU", "CN"}, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: "RU"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionBlock}, "country explicitly in disallowed list")
}

func TestEvaluate_ExplicitDeny_Log(t *testing.T) {
	eval := makeEvaluator(makePolicy("dev", []string{"dev"}, nil, []string{"KP"}, []v1alpha1.Action{v1alpha1.ActionLog}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "dev", DestCountry: "KP"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionLog}, "country explicitly in disallowed list")
}

func TestEvaluate_ExplicitDeny_BlockNoConn(t *testing.T) {
	eval := makeEvaluator(makePolicy("p", []string{"payments"}, nil, []string{"RU"}, []v1alpha1.Action{v1alpha1.ActionBlockNoConn}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "payments", DestCountry: "RU"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionBlockNoConn}, "country explicitly in disallowed list")
}

// ─── Single-policy: implicit deny ────────────────────────────────────────────

func TestEvaluate_ImplicitDeny_NotInAllowlist(t *testing.T) {
	eval := makeEvaluator(makePolicy("prod", []string{"prod"}, []string{"US", "CA"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: "GB"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionBlock}, "destination country not in allowlist")
}

func TestEvaluate_ImplicitDeny_MultipleActions(t *testing.T) {
	// A policy can carry both log and block — verify both are returned.
	eval := makeEvaluator(makePolicy("prod", []string{"prod"},
		[]string{"US"},
		nil,
		[]v1alpha1.Action{v1alpha1.ActionLog, v1alpha1.ActionBlock},
	))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: "DE"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionLog, v1alpha1.ActionBlock}, "destination country not in allowlist")
}

// ─── Single-policy: allow ─────────────────────────────────────────────────────

func TestEvaluate_Allow_InAllowlist(t *testing.T) {
	eval := makeEvaluator(makePolicy("prod", []string{"prod"}, []string{"US", "CA"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: "US"})
	assertAllow(t, got, "all applicable policies permit country")
}

func TestEvaluate_Allow_NoAllowlistNoDisallowlist(t *testing.T) {
	// Policy with no allow or disallow list — everything passes.
	eval := makeEvaluator(makePolicy("permissive", []string{"dev"}, nil, nil, []v1alpha1.Action{v1alpha1.ActionLog}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "dev", DestCountry: "CN"})
	assertAllow(t, got, "all applicable policies permit country")
}

func TestEvaluate_Allow_NotInDisallowlist(t *testing.T) {
	// Policy has only a disallow list; country not on it should pass.
	eval := makeEvaluator(makePolicy("dev", []string{"dev"}, nil, []string{"KP"}, []v1alpha1.Action{v1alpha1.ActionLog}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "dev", DestCountry: "DE"})
	assertAllow(t, got, "all applicable policies permit country")
}

// ─── Explicit deny beats implicit allow ──────────────────────────────────────

func TestEvaluate_ExplicitDenyBeatsAllowlist(t *testing.T) {
	// CN is in both AllowedCountries and DisallowedCountries.
	// Explicit deny must win.
	eval := makeEvaluator(makePolicy("confused", []string{"prod"},
		[]string{"US", "CN"},
		[]string{"CN"},
		[]v1alpha1.Action{v1alpha1.ActionBlock},
	))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: "CN"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionBlock}, "country explicitly in disallowed list")
}

// ─── Wildcard namespace ───────────────────────────────────────────────────────

func TestEvaluate_WildcardPolicy_Deny(t *testing.T) {
	eval := makeEvaluator(makePolicy("catch-all", []string{"*"}, []string{"US"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "any-random-ns", DestCountry: "DE"})
	assertViolation(t, got, []v1alpha1.Action{v1alpha1.ActionBlock}, "destination country not in allowlist")
}

func TestEvaluate_WildcardPolicy_Allow(t *testing.T) {
	eval := makeEvaluator(makePolicy("catch-all", []string{"*"}, []string{"US"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock}))
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "some-other-ns", DestCountry: "US"})
	assertAllow(t, got, "all applicable policies permit country")
}

// ─── Multi-policy: strictest wins ────────────────────────────────────────────

func TestEvaluate_MultiPolicy_NamespaceAndWildcard_Deny(t *testing.T) {
	// Wildcard allows everything; namespace-specific policy blocks RU.
	// The namespace-specific block must win.
	wildcard := makePolicy("catch-all", []string{"*"}, nil, nil, []v1alpha1.Action{v1alpha1.ActionLog})
	specific := makePolicy("prod-policy", []string{"prod"}, nil, []string{"RU"}, []v1alpha1.Action{v1alpha1.ActionBlock})
	eval := makeEvaluator(wildcard, specific)
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "prod", DestCountry: "RU"})
	assertViolated(t, got)
}

func TestEvaluate_MultiPolicy_BothAllow(t *testing.T) {
	// Two policies cover the same namespace; country passes both.
	p1 := makePolicy("p1", []string{"shared"}, []string{"US", "DE"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock})
	p2 := makePolicy("p2", []string{"shared"}, nil, []string{"RU"}, []v1alpha1.Action{v1alpha1.ActionLog})
	eval := makeEvaluator(p1, p2)
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "shared", DestCountry: "US"})
	assertAllow(t, got, "all applicable policies permit country")
}

func TestEvaluate_MultiPolicy_OneAllowOneImplicitDeny(t *testing.T) {
	// p1 has no allowlist (permissive); p2 has allowlist that excludes DE.
	// DE must be denied because p2 blocks it.
	p1 := makePolicy("p1", []string{"shared"}, nil, nil, []v1alpha1.Action{v1alpha1.ActionLog})
	p2 := makePolicy("p2", []string{"shared"}, []string{"US"}, nil, []v1alpha1.Action{v1alpha1.ActionBlock})
	eval := makeEvaluator(p1, p2)
	got := eval.Evaluate(&event.SovereignEvent{Namespace: "shared", DestCountry: "DE"})
	assertViolated(t, got)
}

// ─── Matcher unit tests ───────────────────────────────────────────────────────

func TestMatcher_UpsertAndRemove(t *testing.T) {
	m := NewMatcher()
	p := makePolicy("p1", []string{"prod"}, nil, nil, nil)
	m.UpsertPolicy(&p)

	if got := m.Match("prod"); len(got) != 1 {
		t.Fatalf("expected 1 policy after upsert, got %d", len(got))
	}

	m.RemovePolicy(&p)
	if got := m.Match("prod"); len(got) != 0 {
		t.Fatalf("expected 0 policies after remove, got %d", len(got))
	}
}

func TestMatcher_Upsert_Idempotent(t *testing.T) {
	m := NewMatcher()
	p := makePolicy("p1", []string{"prod"}, nil, nil, nil)
	m.UpsertPolicy(&p)
	m.UpsertPolicy(&p) // second upsert should overwrite, not duplicate
	if got := m.Match("prod"); len(got) != 1 {
		t.Fatalf("expected 1 policy after double upsert, got %d", len(got))
	}
}

func TestMatcher_NoMatch_ReturnsEmpty(t *testing.T) {
	m := NewMatcher()
	p := makePolicy("p1", []string{"prod"}, nil, nil, nil)
	m.UpsertPolicy(&p)
	if got := m.Match("staging"); len(got) != 0 {
		t.Fatalf("expected 0 policies for unregistered namespace, got %d", len(got))
	}
}

func TestMatcher_WildcardMatchesAnyNamespace(t *testing.T) {
	m := NewMatcher()
	p := makePolicy("global", []string{"*"}, nil, nil, nil)
	m.UpsertPolicy(&p)
	for _, ns := range []string{"prod", "dev", "kube-system", "totally-random"} {
		if got := m.Match(ns); len(got) != 1 {
			t.Errorf("wildcard policy should match namespace %q, got %d matches", ns, len(got))
		}
	}
}

func TestMatcher_MultiplePoliciessameNamespace(t *testing.T) {
	m := NewMatcher()
	p1 := makePolicy("p1", []string{"prod"}, nil, nil, nil)
	p2 := makePolicy("p2", []string{"prod"}, nil, nil, nil)
	m.UpsertPolicy(&p1)
	m.UpsertPolicy(&p2)
	if got := m.Match("prod"); len(got) != 2 {
		t.Fatalf("expected 2 policies for namespace with 2 policies, got %d", len(got))
	}
}

// ─── Assertion helpers ────────────────────────────────────────────────────────

func assertAllow(t *testing.T, got Verdict, reason string) {
	t.Helper()
	if got.Violated {
		t.Errorf("Violated = true, want false (reason: %q)", got.Reason)
	}
	if !reflect.DeepEqual(got.Actions, []v1alpha1.Action{v1alpha1.ActionAllow}) {
		t.Errorf("Actions = %v, want [allow]", got.Actions)
	}
	if got.Reason != reason {
		t.Errorf("Reason = %q, want %q", got.Reason, reason)
	}
}

func assertViolation(t *testing.T, got Verdict, wantActions []v1alpha1.Action, reason string) {
	t.Helper()
	if !got.Violated {
		t.Errorf("Violated = false, want true (reason: %q)", got.Reason)
	}
	if !reflect.DeepEqual(got.Actions, wantActions) {
		t.Errorf("Actions = %v, want %v", got.Actions, wantActions)
	}
	if got.Reason != reason {
		t.Errorf("Reason = %q, want %q", got.Reason, reason)
	}
}

// assertViolated checks only that a violation occurred, without asserting specific
// actions or reason — useful for multi-policy tests where the winning policy is
// non-deterministic due to map iteration order.
func assertViolated(t *testing.T, got Verdict) {
	t.Helper()
	if !got.Violated {
		t.Errorf("Violated = false, want true (reason: %q)", got.Reason)
	}
}
/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

// tracingPolicyGVK is the GVK for Tetragon TracingPolicy objects.
// Defined once here to avoid repetition across tests.
var tracingPolicyGVK = schema.GroupVersionKind{
	Group:   "cilium.io",
	Version: "v1alpha1",
	Kind:    "TracingPolicy",
}

// reconciler builds a SovereigntyPolicyReconciler wired to the envtest k8sClient.
// Called at the start of each It() block so each test gets a fresh instance.
func reconciler() *SovereigntyPolicyReconciler {
	return &SovereigntyPolicyReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// reconcilePolicy is a convenience wrapper that fires a single reconcile loop
// for a named cluster-scoped policy and asserts no error.
func reconcilePolicy(ctx context.Context, name string) {
	GinkgoHelper()
	_, err := reconciler().Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
	Expect(err).NotTo(HaveOccurred())
}

// getTracingPolicy fetches the Tetragon TracingPolicy created by the reconciler.
// TracingPolicies live in kube-system (set by the controller after the diff).
func getTracingPolicy(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	tp := &unstructured.Unstructured{}
	tp.SetGroupVersionKind(tracingPolicyGVK)
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "kube-system"}, tp)
	return tp, err
}

// ─── Test suite ───────────────────────────────────────────────────────────────

var _ = Describe("SovereigntyPolicy Controller", func() {

	// ── Helpers ──────────────────────────────────────────────────────────────

	// createPolicy creates a cluster-scoped SovereigntyPolicy and registers
	// automatic cleanup via DeferCleanup so each It() is hermetic.
	createPolicy := func(ctx context.Context, policy *secv1alpha1.SovereigntyPolicy) {
		GinkgoHelper()
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())
		DeferCleanup(func() {
			// Fetch the latest version to avoid ResourceVersion conflicts on delete.
			latest := &secv1alpha1.SovereigntyPolicy{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: policy.Name}, latest); err == nil {
				_ = k8sClient.Delete(ctx, latest)
			}
		})
	}

	// ── Reconcile: non-existent resource ─────────────────────────────────────

	Context("when the policy does not exist", func() {
		It("should return no error (NotFound is terminal)", func() {
			ctx := context.Background()
			_, err := reconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "ghost-policy"},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Reconcile: Ready condition ────────────────────────────────────────────

	Context("when reconciling a policy for the first time", func() {
		It("should set the Ready condition to True", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "cond-test"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces:       []string{"prod"},
					Actions:          []secv1alpha1.Action{secv1alpha1.ActionLog},
					AllowedCountries: []string{"US"},
				},
			}
			createPolicy(ctx, policy)
			reconcilePolicy(ctx, policy.Name)

			// Re-fetch to get the updated status.
			updated := &secv1alpha1.SovereigntyPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: policy.Name}, updated)).To(Succeed())

			cond := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
			Expect(cond).NotTo(BeNil(), "Ready condition should be set")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("Reconciled"))
		})

		It("should be idempotent: reconciling twice does not error", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "idempotent-test"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces: []string{"prod"},
					Actions:    []secv1alpha1.Action{secv1alpha1.ActionLog},
				},
			}
			createPolicy(ctx, policy)
			reconcilePolicy(ctx, policy.Name)
			reconcilePolicy(ctx, policy.Name) // second pass must also succeed
		})
	})

	// ── buildTracingPolicy: no-op cases ──────────────────────────────────────

	Context("when the policy action is log-only", func() {
		It("should not create a TracingPolicy even if violator IPs are present", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "log-only"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces: []string{"dev"},
					Actions:    []secv1alpha1.Action{secv1alpha1.ActionLog},
				},
			}
			createPolicy(ctx, policy)

			// Inject a violator IP directly into status.
			policy.Status.DiscoveredViolatorIPs = []string{"1.2.3.4"}
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())

			reconcilePolicy(ctx, policy.Name)

			_, err := getTracingPolicy(ctx, "log-only-ebpf-rule")
			Expect(err).To(HaveOccurred(), "no TracingPolicy should exist for a log-only action")
		})
	})

	Context("when the policy has no discovered violator IPs", func() {
		It("should not create a TracingPolicy even for a block action", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "block-no-ips"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces:          []string{"prod"},
					Actions:             []secv1alpha1.Action{secv1alpha1.ActionBlock},
					DisallowedCountries: []string{"RU"},
				},
				// Status.DiscoveredViolatorIPs is empty by default
			}
			createPolicy(ctx, policy)
			reconcilePolicy(ctx, policy.Name)

			_, err := getTracingPolicy(ctx, "block-no-ips-ebpf-rule")
			Expect(err).To(HaveOccurred(), "TracingPolicy must not be created before any IPs are discovered")
		})
	})

	// ── buildTracingPolicy: CREATE path ──────────────────────────────────────

	Context("when the policy action is block-kill and violator IPs exist", func() {
		It("should create a TracingPolicy with a Sigkill action", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "block-kill-policy"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces:          []string{"prod"},
					Actions:             []secv1alpha1.Action{secv1alpha1.ActionBlock},
					DisallowedCountries: []string{"RU"},
				},
			}
			createPolicy(ctx, policy)

			policy.Status.DiscoveredViolatorIPs = []string{"1.2.3.4", "5.6.7.8"}
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())

			reconcilePolicy(ctx, policy.Name)

			tp, err := getTracingPolicy(ctx, "block-kill-policy-ebpf-rule")
			Expect(err).NotTo(HaveOccurred())

			// Verify the IPs were encoded as /32 CIDRs inside the spec.
			assertTracingPolicyCIDRs(tp, []string{"1.2.3.4/32", "5.6.7.8/32"})

			// Verify the Tetragon action is Sigkill.
			assertTracingPolicyAction(tp, "Sigkill")

			// Verify the OwnerReference points back to our SovereigntyPolicy.
			assertOwnerReference(tp, policy.Name)

			DeferCleanup(func() {
				tp, err := getTracingPolicy(ctx, "block-kill-policy-ebpf-rule")
				if err == nil {
					_ = k8sClient.Delete(ctx, tp)
				}
			})
		})
	})

	Context("when the policy action is block-noconn and violator IPs exist", func() {
		It("should create a TracingPolicy with an Override/ECONNREFUSED action", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "block-noconn-policy"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces:          []string{"payments"},
					Actions:             []secv1alpha1.Action{secv1alpha1.ActionBlockNoConn},
					DisallowedCountries: []string{"CN"},
				},
			}
			createPolicy(ctx, policy)

			policy.Status.DiscoveredViolatorIPs = []string{"9.9.9.9"}
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())

			reconcilePolicy(ctx, policy.Name)

			tp, err := getTracingPolicy(ctx, "block-noconn-policy-ebpf-rule")
			Expect(err).NotTo(HaveOccurred())

			assertTracingPolicyAction(tp, "Override")

			DeferCleanup(func() {
				tp, err := getTracingPolicy(ctx, "block-noconn-policy-ebpf-rule")
				if err == nil {
					_ = k8sClient.Delete(ctx, tp)
				}
			})
		})
	})

	// ── buildTracingPolicy: UPDATE path ──────────────────────────────────────

	Context("when a new violator IP is discovered after a TracingPolicy already exists", func() {
		It("should update the existing TracingPolicy with the new CIDR", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "update-ips"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces: []string{"prod"},
					Actions:    []secv1alpha1.Action{secv1alpha1.ActionBlock},
				},
			}
			createPolicy(ctx, policy)

			// First reconcile: one IP
			policy.Status.DiscoveredViolatorIPs = []string{"10.0.0.1"}
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())
			reconcilePolicy(ctx, policy.Name)

			tp, err := getTracingPolicy(ctx, "update-ips-ebpf-rule")
			Expect(err).NotTo(HaveOccurred())
			assertTracingPolicyCIDRs(tp, []string{"10.0.0.1/32"})

			// Second reconcile: agent discovered a second IP
			latest := &secv1alpha1.SovereigntyPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: policy.Name}, latest)).To(Succeed())
			latest.Status.DiscoveredViolatorIPs = []string{"10.0.0.1", "10.0.0.2"}
			Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed())
			reconcilePolicy(ctx, policy.Name)

			tp2, err := getTracingPolicy(ctx, "update-ips-ebpf-rule")
			Expect(err).NotTo(HaveOccurred())
			assertTracingPolicyCIDRs(tp2, []string{"10.0.0.1/32", "10.0.0.2/32"})

			DeferCleanup(func() {
				tp, err := getTracingPolicy(ctx, "update-ips-ebpf-rule")
				if err == nil {
					_ = k8sClient.Delete(ctx, tp)
				}
			})
		})
	})

	// ── buildTracingPolicy: DELETE (garbage collection) path ─────────────────

	Context("when a block policy is downgraded to log-only after a TracingPolicy exists", func() {
		It("should delete the orphaned TracingPolicy", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "downgrade-policy"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces: []string{"prod"},
					Actions:    []secv1alpha1.Action{secv1alpha1.ActionBlock},
				},
			}
			createPolicy(ctx, policy)

			// Step 1: put the cluster in a state where a TracingPolicy exists.
			policy.Status.DiscoveredViolatorIPs = []string{"1.1.1.1"}
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())
			reconcilePolicy(ctx, policy.Name)

			_, err := getTracingPolicy(ctx, "downgrade-policy-ebpf-rule")
			Expect(err).NotTo(HaveOccurred(), "TracingPolicy should exist before downgrade")

			// Step 2: operator changes the action to log-only.
			latest := &secv1alpha1.SovereigntyPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: policy.Name}, latest)).To(Succeed())
			latest.Spec.Actions = []secv1alpha1.Action{secv1alpha1.ActionLog}
			Expect(k8sClient.Update(ctx, latest)).To(Succeed())
			reconcilePolicy(ctx, policy.Name)

			_, err = getTracingPolicy(ctx, "downgrade-policy-ebpf-rule")
			Expect(err).To(HaveOccurred(), "TracingPolicy should have been garbage-collected")
		})
	})

	Context("when a block policy has its violator IPs cleared", func() {
		It("should delete the orphaned TracingPolicy", func() {
			ctx := context.Background()

			policy := &secv1alpha1.SovereigntyPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "cleared-ips"},
				Spec: secv1alpha1.SovereigntyPolicySpec{
					Namespaces: []string{"prod"},
					Actions:    []secv1alpha1.Action{secv1alpha1.ActionBlock},
				},
			}
			createPolicy(ctx, policy)

			policy.Status.DiscoveredViolatorIPs = []string{"2.2.2.2"}
			Expect(k8sClient.Status().Update(ctx, policy)).To(Succeed())
			reconcilePolicy(ctx, policy.Name)

			_, err := getTracingPolicy(ctx, "cleared-ips-ebpf-rule")
			Expect(err).NotTo(HaveOccurred())

			// Clear the IPs.
			latest := &secv1alpha1.SovereigntyPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: policy.Name}, latest)).To(Succeed())
			latest.Status.DiscoveredViolatorIPs = nil
			Expect(k8sClient.Status().Update(ctx, latest)).To(Succeed())
			reconcilePolicy(ctx, policy.Name)

			_, err = getTracingPolicy(ctx, "cleared-ips-ebpf-rule")
			Expect(err).To(HaveOccurred(), "TracingPolicy should be removed when there are no violator IPs")
		})
	})
})

// ─── Assertion helpers ────────────────────────────────────────────────────────

// assertTracingPolicyCIDRs walks the TracingPolicy's spec.kprobes[0].selectors[0].matchArgs[0].values
// and checks that the set of CIDRs matches wantCIDRs exactly (order-independent).
func assertTracingPolicyCIDRs(tp *unstructured.Unstructured, wantCIDRs []string) {
	GinkgoHelper()

	values, found, err := unstructured.NestedSlice(tp.Object,
		"spec", "kprobes", "0", "selectors", "0", "matchArgs", "0", "values",
	)
	// The controller stores kprobes as a []interface{} slice, so numeric
	// indices won't work with NestedSlice key path — walk manually.
	if !found || err != nil {
		values = extractCIDRsManually(tp)
	}

	gotSet := make(map[string]bool, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			gotSet[s] = true
		}
	}

	for _, cidr := range wantCIDRs {
		Expect(gotSet).To(HaveKey(cidr), "expected CIDR %q in TracingPolicy", cidr)
	}
	Expect(gotSet).To(HaveLen(len(wantCIDRs)), "unexpected CIDRs in TracingPolicy: %v", gotSet)
}

// assertTracingPolicyAction verifies that the Tetragon matchActions[0].action field
// equals wantAction (e.g. "Sigkill" or "Override").
func assertTracingPolicyAction(tp *unstructured.Unstructured, wantAction string) {
	GinkgoHelper()
	action := extractActionManually(tp)
	Expect(action).To(Equal(wantAction), "unexpected Tetragon action in TracingPolicy")
}

// assertOwnerReference verifies that the TracingPolicy carries an OwnerReference
// pointing to the named SovereigntyPolicy.
func assertOwnerReference(tp *unstructured.Unstructured, policyName string) {
	GinkgoHelper()
	owners := tp.GetOwnerReferences()
	Expect(owners).NotTo(BeEmpty(), "TracingPolicy should have an OwnerReference")
	Expect(owners[0].Name).To(Equal(policyName))
	Expect(owners[0].Kind).To(Equal("SovereigntyPolicy"))
}

// extractCIDRsManually walks the unstructured object without relying on
// string-keyed index paths for slice elements.
func extractCIDRsManually(tp *unstructured.Unstructured) []interface{} {
	spec, ok := tp.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	kprobes, ok := spec["kprobes"].([]interface{})
	if !ok || len(kprobes) == 0 {
		return nil
	}
	kp, ok := kprobes[0].(map[string]interface{})
	if !ok {
		return nil
	}
	selectors, ok := kp["selectors"].([]interface{})
	if !ok || len(selectors) == 0 {
		return nil
	}
	sel, ok := selectors[0].(map[string]interface{})
	if !ok {
		return nil
	}
	matchArgs, ok := sel["matchArgs"].([]interface{})
	if !ok || len(matchArgs) == 0 {
		return nil
	}
	arg, ok := matchArgs[0].(map[string]interface{})
	if !ok {
		return nil
	}
	values, _ := arg["values"].([]interface{})
	return values
}

// extractActionManually walks the unstructured object to find matchActions[0].action.
func extractActionManually(tp *unstructured.Unstructured) string {
	spec, ok := tp.Object["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	kprobes, ok := spec["kprobes"].([]interface{})
	if !ok || len(kprobes) == 0 {
		return ""
	}
	kp, ok := kprobes[0].(map[string]interface{})
	if !ok {
		return ""
	}
	selectors, ok := kp["selectors"].([]interface{})
	if !ok || len(selectors) == 0 {
		return ""
	}
	sel, ok := selectors[0].(map[string]interface{})
	if !ok {
		return ""
	}
	matchActions, ok := sel["matchActions"].([]interface{})
	if !ok || len(matchActions) == 0 {
		return ""
	}
	action, ok := matchActions[0].(map[string]interface{})
	if !ok {
		return ""
	}
	s, _ := action["action"].(string)
	return s
}

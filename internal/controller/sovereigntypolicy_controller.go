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
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SovereigntyPolicyReconciler reconciles a SovereigntyPolicy object
type SovereigntyPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereigntypolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereigntypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereigntypolicies/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SovereigntyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the SovereigntyPolicy instance
	var policy secv1alpha1.SovereigntyPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			// The policy was deleted. K8s OwnerReferences will automatically delete the TracingPolicy.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Build the Tetragon TracingPolicy based on discovered IPs
	tracingPolicy := r.buildTracingPolicy(&policy)

	// 3. Apply, Update, or Delete the TracingPolicy
	if tracingPolicy != nil {
		found := &unstructured.Unstructured{}
		found.SetGroupVersionKind(tracingPolicy.GroupVersionKind())

		err := r.Get(ctx, types.NamespacedName{Name: tracingPolicy.GetName(), Namespace: tracingPolicy.GetNamespace()}, found)

		if err != nil && apierrors.IsNotFound(err) {
			// CREATE: The Agent found the first bad IP
			logger.Info("Deploying dynamic eBPF blocklist", "Name", tracingPolicy.GetName())
			if err := ctrl.SetControllerReference(&policy, tracingPolicy, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, tracingPolicy); err != nil {
				logger.Error(err, "Failed to create TracingPolicy")
				return ctrl.Result{}, err
			}
		} else if err == nil {
			// UPDATE: The Agent found MORE bad IPs, or the policy changed. Patch the existing eBPF hook.
			tracingPolicy.SetResourceVersion(found.GetResourceVersion())
			if err := ctrl.SetControllerReference(&policy, tracingPolicy, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Update(ctx, tracingPolicy); err != nil {
				logger.Error(err, "Failed to update TracingPolicy with new IPs")
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, err
		}
	} else {
		// DELETE (Garbage Collection): If tracingPolicy is nil, but a TracingPolicy exists in the cluster,
		// it means the user changed the action to "log" or cleared the IPs. We must delete the kernel hook.
		found := &unstructured.Unstructured{}
		found.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cilium.io",
			Version: "v1alpha1",
			Kind:    "TracingPolicy",
		})

		err := r.Get(ctx, types.NamespacedName{Name: policy.Name + "-ebpf-rule"}, found)
		if err == nil {
			logger.Info("Removing eBPF blocklist (no longer required by policy)", "Name", found.GetName())
			if err := r.Delete(ctx, found); err != nil {
				logger.Error(err, "Failed to delete orphaned TracingPolicy")
				return ctrl.Result{}, err
			}
		}
	}

	// 4. Update the SovereigntyPolicy Status Condition
	// We define what the "Ready" state means for our specific controller at this exact moment.
	condition := metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "Policy successfully parsed and enforcement rules are active",
	}

	// meta.SetStatusCondition automatically handles timestamps, overwriting existing
	// conditions of the same Type, and avoiding unnecessary updates if nothing changed.
	meta.SetStatusCondition(&policy.Status.Conditions, condition)

	if err := r.Status().Update(ctx, &policy); err != nil {
		logger.Error(err, "Failed to update SovereigntyPolicy status conditions")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SovereigntyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secv1alpha1.SovereigntyPolicy{}).
		Named("sovereigntypolicy").
		Complete(r)
}

// buildTracingPolicy translates dynamically discovered IPs into a kernel-level Tetragon blocklist
func (r *SovereigntyPolicyReconciler) buildTracingPolicy(policy *secv1alpha1.SovereigntyPolicy) *unstructured.Unstructured {
	// 1. Determine the Kernel Action based on the new granular actions array
	var tetragonAction map[string]interface{}

	if hasAction(policy.Spec.Actions, secv1alpha1.ActionBlock) {
		tetragonAction = map[string]interface{}{"action": "Sigkill"}
	} else if hasAction(policy.Spec.Actions, "block-noconn") {
		tetragonAction = map[string]interface{}{
			"action":   "Override",
			"argError": -111, // Return ECONNREFUSED to the application
		}
	} else {
		// If it's just "log" or undefined, we don't need a kernel blocklist
		return nil
	}

	// 2. Check if the Agent has discovered any violator IPs yet
	if len(policy.Status.DiscoveredViolatorIPs) == 0 {
		return nil
	}

	// 3. Construct the eBPF Policy
	tp := &unstructured.Unstructured{}
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	tp.SetName(policy.Name + "-ebpf-rule")
	tp.SetNamespace("kube-system")

	// Convert the discovered IPs into /32 CIDRs for Tetragon
	var ipValues []interface{}
	for _, ip := range policy.Status.DiscoveredViolatorIPs {
		ipValues = append(ipValues, ip+"/32")
	}

	tp.Object["spec"] = map[string]interface{}{
		"kprobes": []interface{}{
			map[string]interface{}{
				"call":    "tcp_connect",
				"syscall": false,
				"args": []interface{}{
					map[string]interface{}{
						"index": 0,
						"type":  "sock",
					},
				},
				"selectors": []interface{}{
					map[string]interface{}{
						"matchArgs": []interface{}{
							map[string]interface{}{
								"index":    0,
								"operator": "DAddr",
								"values":   ipValues,
							},
						},
						"matchActions": []interface{}{
							tetragonAction,
						},
					},
				},
			},
		},
	}

	return tp
}

// hasAction is a helper to check if a specific action exists in the policy
func hasAction(actions []secv1alpha1.Action, target secv1alpha1.Action) bool {
	return slices.Contains(actions, target)
}

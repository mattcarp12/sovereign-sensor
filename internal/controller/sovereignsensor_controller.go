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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	_ "embed"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereignsensors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereignsensors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereignsensors/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete;bind;escalate
// +kubebuilder:rbac:groups=cilium.io,resources=tracingpolicies,verbs=get;list;watch;create;update;patch;delete

// SovereignSensorReconciler reconciles a SovereignSensor object
type SovereignSensorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SovereignSensorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// 1. Fetch the SovereignSensor instance
	var sensor secv1alpha1.SovereignSensor
	if err := r.Get(ctx, req.NamespacedName, &sensor); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	const finalizerName = "sec.sovereign.io/cleanup"

	// Handle deletion
	if !sensor.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&sensor, finalizerName) {
			// Clean up cluster-scoped resources manually
			cr := r.agentClusterRole(&sensor)
			crb := r.agentClusterRoleBinding(&sensor)
			for _, res := range []client.Object{crb, cr} {
				if err := r.Delete(ctx, res); client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(&sensor, finalizerName)
			if err := r.Update(ctx, &sensor); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&sensor, finalizerName) {
		controllerutil.AddFinalizer(&sensor, finalizerName)
		if err := r.Update(ctx, &sensor); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 1.5 Auto-Deploy Baseline Tracing Policy
	baselinePolicy := r.baselineTracingPolicy(&sensor)
	if err := r.Patch(ctx, baselinePolicy, client.Apply, client.ForceOwnership, client.FieldOwner("sovereign-sensor-controller")); err != nil {
		return ctrl.Result{}, err
	}

	// 2. Define all desired resources
	sa := r.agentServiceAccount(&sensor)
	cr := r.agentClusterRole(&sensor)
	crb := r.agentClusterRoleBinding(&sensor)
	ds := r.agentDaemonSet(&sensor)

	// 3. Set owner references and create/update resources
	// Namespace-scoped resources — owner references work fine here
	namespacedResources := []client.Object{sa, ds}
	for _, res := range namespacedResources {
		if err := ctrl.SetControllerReference(&sensor, res, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.createIfNotExists(ctx, res); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Cluster-scoped resources — owner references don't work cross-namespace.
	// We manage cleanup via a finalizer instead.
	clusterScopedResources := []client.Object{cr, crb}
	for _, res := range clusterScopedResources {
		if err := r.createIfNotExists(ctx, res); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Update Status
	sensor.Status.Phase = "Running"
	if err := r.Status().Update(ctx, &sensor); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *SovereignSensorReconciler) createIfNotExists(ctx context.Context, obj client.Object) error {
	err := r.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, obj)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, obj)
	}
	return err
}

// agentDaemonSet returns a DaemonSet object for the SovereignSensor agent
func (r *SovereignSensorReconciler) agentDaemonSet(sensor *secv1alpha1.SovereignSensor) *appsv1.DaemonSet {
	labels := map[string]string{"app": "sovereign-sensor-agent", "sensor_cr": sensor.Name}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sensor.Name + "-agent",
			Namespace: sensor.Spec.TargetNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sensor.Name + "-agent-sa",
					HostNetwork:        true, // Crucial for eBPF/Tetragon
					Containers: []corev1.Container{{
						Name:            "agent",
						Image:           "sovereign-sensor-agent:dev", // We will make this configurable later
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env: []corev1.EnvVar{
							{Name: "LOG_LEVEL", Value: sensor.Spec.LogLevel},
							{Name: "METRICS_ADDR", Value: ":9091"},
							{Name: "TETRAGON_SERVER", Value: "127.0.0.1:54321"},
						},
						Ports: []corev1.ContainerPort{{
							Name:          "metrics",
							ContainerPort: 9091,
							Protocol:      corev1.ProtocolTCP,
						}},
					}},
				},
			},
		},
	}
}

func (r *SovereignSensorReconciler) agentServiceAccount(sensor *secv1alpha1.SovereignSensor) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sensor.Name + "-agent-sa",
			Namespace: sensor.Spec.TargetNamespace,
		},
	}
}

func (r *SovereignSensorReconciler) agentClusterRole(sensor *secv1alpha1.SovereignSensor) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: sensor.Name + "-agent-role",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"sec.sovereign.io"},
				Resources: []string{"sovereigntypolicies"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"sec.sovereign.io"},
				Resources: []string{"sovereigntypolicies/status"},
				Verbs:     []string{"get", "update", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch", "update"},
			},
		},
	}
}

func (r *SovereignSensorReconciler) agentClusterRoleBinding(sensor *secv1alpha1.SovereignSensor) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: sensor.Name + "-agent-binding",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     sensor.Name + "-agent-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sensor.Name + "-agent-sa",
				Namespace: sensor.Spec.TargetNamespace,
			},
		},
	}
}

// baselineTracingPolicy generates the native Kubernetes object for Tetragon's TCP monitor
func (r *SovereignSensorReconciler) baselineTracingPolicy(sensor *secv1alpha1.SovereignSensor) *unstructured.Unstructured {
	tp := &unstructured.Unstructured{}
	tp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: "v1alpha1",
		Kind:    "TracingPolicy",
	})
	tp.SetName("sovereign-tcp-monitor")
	tp.SetNamespace("kube-system") // Tetragon's default namespace

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
			},
		},
	}
	return tp
}

// SetupWithManager sets up the controller with the Manager.
func (r *SovereignSensorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secv1alpha1.SovereignSensor{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}

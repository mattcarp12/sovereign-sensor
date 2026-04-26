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
	"bytes"
	"context"
	"fmt"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	_ "embed"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

// SovereignSensorReconciler reconciles a SovereignSensor object
type SovereignSensorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//go:embed manifests/tetragon.yaml
var tetragonManifests []byte

// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereignsensors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereignsensors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sec.sovereign.io,resources=sovereignsensors/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SovereignSensor object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/reconcile
// Reconcile is part of the main kubernetes reconciliation loop.
func (r *SovereignSensorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. Fetch the SovereignSensor instance
	var sensor secv1alpha1.SovereignSensor
	if err := r.Get(ctx, req.NamespacedName, &sensor); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 1.5 Auto-Deploy Tetragon Dependency
	if sensor.Spec.DeployTetragon {
		logger.Info("DeployTetragon is true, verifying Tetragon installation...")
		if err := r.applyManifests(ctx, tetragonManifests); err != nil {
			logger.Error(err, "Failed to deploy Tetragon dependency")
			return ctrl.Result{}, err
		}
	}

	// 2. Define all desired resources
	sa := r.agentServiceAccount(&sensor)
	cr := r.agentClusterRole(&sensor)
	crb := r.agentClusterRoleBinding(&sensor)
	ds := r.agentDaemonSet(&sensor)

	// 3. Apply Resources sequentially
	resources := []client.Object{sa, cr, crb, ds}

	for _, res := range resources {
		// Set owner reference for garbage collection
		if err := ctrl.SetControllerReference(&sensor, res, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		// Check if it exists
		err := r.Get(ctx, types.NamespacedName{Name: res.GetName(), Namespace: res.GetNamespace()}, res)
		if err != nil && apierrors.IsNotFound(err) {
			logger.Info("Creating Resource", "Kind", res.GetObjectKind().GroupVersionKind().Kind, "Name", res.GetName())
			if err = r.Create(ctx, res); err != nil {
				return ctrl.Result{}, err
			}
		} else if err != nil {
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
						Image:           "sovereign-sensor:latest", // We will make this configurable later
						ImagePullPolicy: corev1.PullIfNotPresent,
						Env: []corev1.EnvVar{
							{
								Name:  "LOG_LEVEL",
								Value: sensor.Spec.LogLevel,
							},
						},
						// Note: You would mount your MaxMind DB volume here just like in Helm
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
				Resources: []string{"sovereigntypolicies", "sovereigntypolicies/status"},
				Verbs:     []string{"get", "list", "watch"},
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

// applyManifests parses a multi-document YAML and applies each object to the cluster.
func (r *SovereignSensorReconciler) applyManifests(ctx context.Context, yamlData []byte) error {
	logger := log.FromContext(ctx)
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlData), 4096)

	for {
		// We use unstructured.Unstructured because we don't have the Go types
		// for every possible Kubernetes object (like Tetragon's custom CRDs).
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if err == io.EOF {
				break // End of file
			}
			return fmt.Errorf("failed to decode yaml: %w", err)
		}

		// Skip empty objects (sometimes caused by trailing --- in YAML)
		if len(obj.Object) == 0 {
			continue
		}

		// Attempt to fetch the object to see if it already exists
		found := &unstructured.Unstructured{}
		found.SetGroupVersionKind(obj.GroupVersionKind())
		err := r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, found)

		if err != nil && apierrors.IsNotFound(err) {
			logger.Info("Deploying Dependency", "Kind", obj.GetKind(), "Name", obj.GetName())
			if err := r.Create(ctx, obj); err != nil {
				return fmt.Errorf("failed to create %s %s: %w", obj.GetKind(), obj.GetName(), err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to get %s %s: %w", obj.GetKind(), obj.GetName(), err)
		}
		// Note: For a fully mature operator, you would add an 'else' block here to Update()
		// the object if it exists but differs from the embedded manifest.
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SovereignSensorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secv1alpha1.SovereignSensor{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}

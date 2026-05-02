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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

const sensorFinalizer = "sec.sovereign.io/cleanup"

// sensorReconciler returns a fresh SovereignSensorReconciler for each test.
func sensorReconciler() *SovereignSensorReconciler {
	return &SovereignSensorReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// reconcileSensor fires a single reconcile loop for a named cluster-scoped
// SovereignSensor and asserts no error.
func reconcileSensor(ctx context.Context, name string) {
	GinkgoHelper()
	_, err := sensorReconciler().Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name},
	})
	Expect(err).NotTo(HaveOccurred())
}

// createSensor creates a cluster-scoped SovereignSensor and registers
// automatic cleanup so each It() block is hermetic.
// DeployTetragon defaults to false to keep tests fast and self-contained;
// pass a mutator func to override spec fields.
func createSensor(ctx context.Context, name string, mutate func(*secv1alpha1.SovereignSensor)) *secv1alpha1.SovereignSensor {
	GinkgoHelper()
	sensor := &secv1alpha1.SovereignSensor{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: secv1alpha1.SovereignSensorSpec{
			DeployTetragon:  false,
			LogLevel:        "INFO",
			TargetNamespace: "default",
		},
	}
	if mutate != nil {
		mutate(sensor)
	}
	Expect(k8sClient.Create(ctx, sensor)).To(Succeed())

	DeferCleanup(func() {
		latest := &secv1alpha1.SovereignSensor{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, latest); err != nil {
			return
		}
		// Remove the finalizer so the object isn't stuck in Terminating.
		controllerutil.RemoveFinalizer(latest, sensorFinalizer)
		_ = k8sClient.Update(ctx, latest)
		_ = k8sClient.Delete(ctx, latest)
	})

	return sensor
}

// ─── Test suite ───────────────────────────────────────────────────────────────

var _ = Describe("SovereignSensor Controller", func() {

	// ── NotFound: no-op ──────────────────────────────────────────────────────

	Context("when the SovereignSensor does not exist", func() {
		It("should return no error", func() {
			ctx := context.Background()
			_, err := sensorReconciler().Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "does-not-exist"},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Finalizer ────────────────────────────────────────────────────────────

	Context("finalizer management", func() {
		It("should add the finalizer on first reconcile", func() {
			ctx := context.Background()
			createSensor(ctx, "finalizer-add", nil)
			reconcileSensor(ctx, "finalizer-add")

			updated := &secv1alpha1.SovereignSensor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "finalizer-add"}, updated)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updated, sensorFinalizer)).To(BeTrue())
		})

		It("should be idempotent: adding the finalizer twice does not error", func() {
			ctx := context.Background()
			createSensor(ctx, "finalizer-idempotent", nil)
			reconcileSensor(ctx, "finalizer-idempotent")
			reconcileSensor(ctx, "finalizer-idempotent")

			updated := &secv1alpha1.SovereignSensor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "finalizer-idempotent"}, updated)).To(Succeed())

			count := 0
			for _, f := range updated.Finalizers {
				if f == sensorFinalizer {
					count++
				}
			}
			Expect(count).To(Equal(1), "finalizer must appear exactly once")
		})
	})

	// ── Child resource creation ───────────────────────────────────────────────

	Context("child resource creation (DeployTetragon=false)", func() {
		It("should create a DaemonSet in the target namespace", func() {
			ctx := context.Background()
			createSensor(ctx, "ds-test", nil)
			reconcileSensor(ctx, "ds-test")

			ds := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ds-test-agent",
				Namespace: "default",
			}, ds)).To(Succeed())

			Expect(ds.Spec.Template.Spec.HostNetwork).To(BeTrue(),
				"agent pods must run with host networking for eBPF")

			containers := ds.Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(1))
			Expect(containers[0].Name).To(Equal("agent"))

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ds) })
		})

		It("should set the DaemonSet's OwnerReference to the SovereignSensor", func() {
			ctx := context.Background()
			createSensor(ctx, "owner-ref-test", nil)
			reconcileSensor(ctx, "owner-ref-test")

			ds := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "owner-ref-test-agent",
				Namespace: "default",
			}, ds)).To(Succeed())

			Expect(ds.OwnerReferences).NotTo(BeEmpty())
			Expect(ds.OwnerReferences[0].Name).To(Equal("owner-ref-test"))
			Expect(ds.OwnerReferences[0].Kind).To(Equal("SovereignSensor"))

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ds) })
		})

		It("should create a ServiceAccount in the target namespace", func() {
			ctx := context.Background()
			createSensor(ctx, "sa-test", nil)
			reconcileSensor(ctx, "sa-test")

			sa := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "sa-test-agent-sa",
				Namespace: "default",
			}, sa)).To(Succeed())

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, sa) })
		})

		It("should create a ClusterRole with the correct policy rules", func() {
			ctx := context.Background()
			createSensor(ctx, "cr-test", nil)
			reconcileSensor(ctx, "cr-test")

			cr := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cr-test-agent-role"}, cr)).To(Succeed())

			// Verify that the role grants list/watch on SovereigntyPolicies.
			Expect(cr.Rules).NotTo(BeEmpty())
			verbs := cr.Rules[0].Verbs
			Expect(verbs).To(ContainElements("get", "list", "watch"))

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })
		})

		It("should create a ClusterRoleBinding pointing to the correct ServiceAccount", func() {
			ctx := context.Background()
			createSensor(ctx, "crb-test", nil)
			reconcileSensor(ctx, "crb-test")

			crb := &rbacv1.ClusterRoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "crb-test-agent-binding"}, crb)).To(Succeed())

			Expect(crb.RoleRef.Name).To(Equal("crb-test-agent-role"))
			Expect(crb.Subjects).To(HaveLen(1))
			Expect(crb.Subjects[0].Name).To(Equal("crb-test-agent-sa"))
			Expect(crb.Subjects[0].Namespace).To(Equal("default"))

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, crb) })
		})

		It("should not create a ClusterRole OwnerReference (cross-namespace ownership is unsupported)", func() {
			ctx := context.Background()
			createSensor(ctx, "no-owner-cr", nil)
			reconcileSensor(ctx, "no-owner-cr")

			cr := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "no-owner-cr-agent-role"}, cr)).To(Succeed())
			Expect(cr.OwnerReferences).To(BeEmpty(),
				"cluster-scoped RBAC resources must not carry OwnerReferences to cluster-scoped CRs")

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cr) })
		})
	})

	// ── LogLevel propagation ──────────────────────────────────────────────────

	Context("spec propagation", func() {
		It("should propagate LogLevel to the agent container env var", func() {
			ctx := context.Background()
			createSensor(ctx, "loglevel-test", func(s *secv1alpha1.SovereignSensor) {
				s.Spec.LogLevel = "DEBUG"
			})
			reconcileSensor(ctx, "loglevel-test")

			ds := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "loglevel-test-agent",
				Namespace: "default",
			}, ds)).To(Succeed())

			envVars := ds.Spec.Template.Spec.Containers[0].Env
			var logLevelValue string
			for _, e := range envVars {
				if e.Name == "LOG_LEVEL" {
					logLevelValue = e.Value
				}
			}
			Expect(logLevelValue).To(Equal("DEBUG"))

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ds) })
		})

		It("should deploy resources into the configured TargetNamespace", func() {
			ctx := context.Background()

			// Create the namespace first so the DaemonSet and SA can land there.
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "sensor-target"}}
			_ = k8sClient.Create(ctx, ns) // ignore already-exists
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ns) })

			createSensor(ctx, "ns-test", func(s *secv1alpha1.SovereignSensor) {
				s.Spec.TargetNamespace = "sensor-target"
			})
			reconcileSensor(ctx, "ns-test")

			ds := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "ns-test-agent",
				Namespace: "sensor-target",
			}, ds)).To(Succeed())

			DeferCleanup(func() { _ = k8sClient.Delete(ctx, ds) })
		})
	})

	// ── Status ───────────────────────────────────────────────────────────────

	Context("status updates", func() {
		It("should set Status.Phase to Running after reconciliation", func() {
			ctx := context.Background()
			createSensor(ctx, "status-test", nil)
			reconcileSensor(ctx, "status-test")

			updated := &secv1alpha1.SovereignSensor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "status-test"}, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal("Running"))
		})
	})

	// ── Idempotency ───────────────────────────────────────────────────────────

	Context("idempotency", func() {
		It("should not error when reconciled multiple times (createIfNotExists)", func() {
			ctx := context.Background()
			createSensor(ctx, "idempotent-sensor", nil)

			for i := 0; i < 3; i++ {
				reconcileSensor(ctx, "idempotent-sensor")
			}

			// DaemonSet should exist exactly once, not duplicated.
			dsList := &appsv1.DaemonSetList{}
			Expect(k8sClient.List(ctx, dsList)).To(Succeed())
			count := 0
			for _, ds := range dsList.Items {
				if ds.Name == "idempotent-sensor-agent" {
					count++
				}
			}
			Expect(count).To(Equal(1))

			DeferCleanup(func() {
				ds := &appsv1.DaemonSet{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: "idempotent-sensor-agent", Namespace: "default"}, ds); err == nil {
					_ = k8sClient.Delete(ctx, ds)
				}
			})
		})
	})

	// ── Deletion / finalizer cleanup ──────────────────────────────────────────

	Context("deletion and finalizer cleanup", func() {
		It("should delete the ClusterRole and ClusterRoleBinding when the sensor is deleted", func() {
			ctx := context.Background()
			createSensor(ctx, "deletion-test", nil)
			reconcileSensor(ctx, "deletion-test")

			// Verify RBAC was created.
			cr := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "deletion-test-agent-role"}, cr)).To(Succeed())

			crb := &rbacv1.ClusterRoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "deletion-test-agent-binding"}, crb)).To(Succeed())

			// Delete the sensor — this sets DeletionTimestamp.
			sensor := &secv1alpha1.SovereignSensor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "deletion-test"}, sensor)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sensor)).To(Succeed())

			// Trigger the deletion reconcile loop (DeletionTimestamp is now set).
			reconcileSensor(ctx, "deletion-test")

			// ClusterRole and ClusterRoleBinding should now be gone.
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "deletion-test-agent-role"}, cr)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "ClusterRole should be deleted during cleanup")

			err = k8sClient.Get(ctx, types.NamespacedName{Name: "deletion-test-agent-binding"}, crb)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "ClusterRoleBinding should be deleted during cleanup")
		})

		It("should remove the finalizer after cleanup so the object can be garbage collected", func() {
			ctx := context.Background()
			createSensor(ctx, "finalizer-removal", nil)
			reconcileSensor(ctx, "finalizer-removal")

			// Delete and reconcile the deletion path.
			sensor := &secv1alpha1.SovereignSensor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "finalizer-removal"}, sensor)).To(Succeed())
			Expect(k8sClient.Delete(ctx, sensor)).To(Succeed())
			reconcileSensor(ctx, "finalizer-removal")

			// After the deletion reconcile, the object should be gone entirely
			// (finalizer removed → k8s completes garbage collection).
			updated := &secv1alpha1.SovereignSensor{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "finalizer-removal"}, updated)
			Expect(errors.IsNotFound(err)).To(BeTrue(),
				"sensor should be fully deleted after finalizer is removed")
		})
	})

	// ── applyManifests (DeployTetragon=true) ─────────────────────────────────

	Context("when DeployTetragon is true", func() {
		It("should apply the embedded Tetragon manifests without error", func() {
			ctx := context.Background()
			createSensor(ctx, "tetragon-deploy", func(s *secv1alpha1.SovereignSensor) {
				s.Spec.DeployTetragon = true
			})

			// The Tetragon manifest contains only standard k8s types (SA, ConfigMap,
			// ClusterRole, ClusterRoleBinding, DaemonSet, Deployment, Service,
			// Role, RoleBinding) so envtest can handle all of them natively.
			// A successful reconcile without error is the assertion.
			reconcileSensor(ctx, "tetragon-deploy")

			// Spot-check: the Tetragon ServiceAccount defined in tetragon.yaml
			// should now exist in kube-system.
			sa := &corev1.ServiceAccount{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "tetragon",
				Namespace: "kube-system",
			}, sa)).To(Succeed())
		})
	})
})
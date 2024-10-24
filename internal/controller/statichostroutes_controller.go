/*
Copyright 2024.

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
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkv1alpha1 "operator.linode.io/vlanoperator/api/v1alpha1"
)

// StaticHostRoutesReconciler reconciles a StaticHostRoutes object
type StaticHostRoutesReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=network.operator.linode.io,resources=statichostroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.operator.linode.io,resources=statichostroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=network.operator.linode.io,resources=statichostroutes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StaticHostRoutes object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *StaticHostRoutesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the StaticHostRoutes instance
	var staticHostRoutes networkv1alpha1.StaticHostRoutes
	if err := r.Get(ctx, req.NamespacedName, &staticHostRoutes); err != nil {
		logger.Error(err, "unable to fetch StaticHostRoutes")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// Ensure namespace is set
	namespace := req.Namespace
	if namespace == "" {
		namespace = "default"
	}

	// Check if the object is being deleted
	if !staticHostRoutes.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if containsString(staticHostRoutes.ObjectMeta.Finalizers, "finalizer.statichostroutes.network.operator.linode.io") {
			// Delete the associated DaemonSet
			daemonSet := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "customhostroute",
					Namespace: namespace, // Set the namespace
				},
			}
			if err := r.Delete(ctx, daemonSet); err != nil {
				return ctrl.Result{}, err
			}

			// Remove the finalizer
			staticHostRoutes.ObjectMeta.Finalizers = removeString(staticHostRoutes.ObjectMeta.Finalizers, "finalizer.statichostroutes.network.operator.linode.io")
			if err := r.Update(ctx, &staticHostRoutes); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !containsString(staticHostRoutes.ObjectMeta.Finalizers, "finalizer.statichostroutes.network.operator.linode.io") {
		staticHostRoutes.ObjectMeta.Finalizers = append(staticHostRoutes.ObjectMeta.Finalizers, "finalizer.statichostroutes.network.operator.linode.io")
		if err := r.Update(ctx, &staticHostRoutes); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Build the command to add routes from the list of CIDR ranges and gateways
	var routeCommands string
	for _, route := range staticHostRoutes.Spec.Routes {
		routeCommands += fmt.Sprintf(`
		echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') Adding route %s via %s"
		ip route add %s via %s 2>&1 | while IFS= read -r line; do
			echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S')     $line"
		done || true
		`, route.Cidr, route.Gateway, route.Cidr, route.Gateway)
	}

	var sleepCommand string
	if staticHostRoutes.Spec.Interval.Duration > 0 {
		sleepCommand = fmt.Sprintf("sleep %d", int(staticHostRoutes.Spec.Interval.Seconds()))
	} else {
		sleepCommand = "sleep 60"
	}

	// Define the desired DaemonSet
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "customhostroute",
			Namespace: namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "customhostroute",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "customhostroute",
					},
				},
				Spec: corev1.PodSpec{
					HostNetwork: true,
					Containers: []corev1.Container{
						{
							Name:  "route-adder",
							Image: "alpine:3.14",
							SecurityContext: &corev1.SecurityContext{
								Privileged: pointer.BoolPtr(true),
							},
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								fmt.Sprintf(`
								echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') Installing iproute2"
								apk add --no-cache iproute2 2>&1 | sed 's/^/    /'
								while true; do
								echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') Starting route configuration"
								%s
								echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') Route configuration complete, sleeping"
								%s
								done
								`, routeCommands, sleepCommand),
							},
						},
					},
				},
			},
		},
	}

	// Set the owner reference
	if err := ctrl.SetControllerReference(&staticHostRoutes, daemonSet, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the DaemonSet already exists
	found := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating DaemonSet", "Namespace", daemonSet.Namespace, "Name", daemonSet.Name)
		if err := r.Create(ctx, daemonSet); err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		// DaemonSet exists, check if it needs to be updated
		if !reflect.DeepEqual(found.Spec, daemonSet.Spec) {
			logger.Info("Updating DaemonSet", "Namespace", daemonSet.Namespace, "Name", daemonSet.Name)
			found.Spec = daemonSet.Spec
			if err := r.Update(ctx, found); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

// Helper functions to manage finalizers
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticHostRoutesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv1alpha1.StaticHostRoutes{}).
		Complete(r)
}

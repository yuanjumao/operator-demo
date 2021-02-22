/*
Copyright 2021.

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

package controllers

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cachev1alpha1 "yuan.com/example/memcached-operator/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.example.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Memcached object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("memcached", req.NamespacedName)

	// your logic here
	memcached := &cachev1alpha1.Memcached{}
	err := r.Get(ctx, req.NamespacedName, memcached)
	if nil != err {
		if errors.IsNotFound(err) {
			log.Info("Memcached resource not found. Ignoring since object must be deleted",
				"Memcached.NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: memcached.Name, Namespace: memcached.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			dep := r.deploymentForMemcached(memcached)
			if err = r.Create(ctx, dep); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	// Ensure the deployment size is the same as the spec.
	if *found.Spec.Replicas != memcached.Spec.Size {
		found.Spec.Replicas = &memcached.Spec.Size
		if err = r.Update(ctx, found); nil != err {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	// Update the Memcached status with the pod names.
	// List the pods for this CR's deployment.
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(memcached.Namespace),
		client.MatchingLabels(labelsForApp(memcached.Name)),
	}
	if err = r.List(ctx, podList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}
	// Update status.Nodes if needed.
	podNames := getPodName(podList.Items)
	if !reflect.DeepEqual(podNames, memcached.Status.Nodes) {
		memcached.Status.Nodes = podNames
		if err := r.Status().Update(ctx, memcached); err != nil {
			log.Error(err, "update memcached.status error", memcached.Name, "memcached.Name", memcached.Name)
			return ctrl.Result{}, err
		}
		log.Info("update memcached status successful",
			"memcached.Namespace", memcached.Name, "memcached.Name", memcached.Name)
	}
	return ctrl.Result{}, nil
}

func (r *MemcachedReconciler) deploymentForMemcached(m *cachev1alpha1.Memcached) *appsv1.Deployment {
	lbls := labelsForApp(m.Name)
	replicas := m.Spec.Size
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: lbls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: lbls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:   m.Spec.Image,
						Name:    "memcached",
						Command: []string{"/bin/sh"},
						Args:    []string{"-c", "while true; do echo hello; sleep 10;done"},
					}},
				},
			},
		},
	}
	controllerutil.SetOwnerReference(m, dep, r.Scheme)
	return dep
}

// labelsForApp creates a simple set of labels for Memcached.
func labelsForApp(name string) map[string]string {
	return map[string]string{"cr_name": name}
}

func getPodName(pods []corev1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Memcached{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}

/*
Copyright 2020.

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
	daisycomv1 "github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/controllers/daisymanager"
	"github.com/daisy/daisy-operator/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DaisyInstallationReconciler reconciles a DaisyInstallation object
type DaisyInstallationReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
	DMM      daisymanager.Manager
}

// +kubebuilder:rbac:groups=daisy.com,resources=daisyinstallations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=daisy.com,resources=daisyinstallations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=daisy.com,resources=daisyinstallations/finalizers,verbs=update
// +kubebuilder:rbac:groups=daisy.com,resources=daisyoperatorconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=daisy.com,resources=daisyoperatorconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=daisy.com,resources=daisyoperatorconfigurations/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;persistentvolumeclaims;pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *DaisyInstallationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	cur := &daisycomv1.DaisyInstallation{}
	if err = r.Client.Get(ctx, req.NamespacedName, cur); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// main logic
	if err := r.DMM.Sync(cur); err != nil {
		if k8s.IsRequeueError(err) {
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DaisyInstallationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&daisycomv1.DaisyInstallation{}).
		//For(&daisycomv1.DaisyOperatorConfiguration{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

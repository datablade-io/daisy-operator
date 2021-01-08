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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
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
	DMM      Manager
}

// +kubebuilder:rbac:groups=daisy.com,resources=daisyinstallations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=daisy.com,resources=daisyinstallations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=daisy.com,resources=daisyinstallations/finalizers,verbs=update
// +kubebuilder:rbac:groups=daisy.com,resources=daisyoperatorconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=daisy.com,resources=daisyoperatorconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=daisy.com,resources=daisyoperatorconfigurations/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DaisyInstallation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *DaisyInstallationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("Namespace", req.Namespace, "Installation", req.Name)

	diTmp := &daisycomv1.DaisyInstallation{}
	if err := r.Client.Get(ctx, req.NamespacedName, diTmp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	//di := diTmp.DeepCopy()
	di := diTmp

	// main logic
	oldStatus := di.Status.DeepCopy()
	old := &daisycomv1.DaisyInstallation{}

	isDelete := false
	if !di.DeletionTimestamp.IsZero() {
		isDelete = true
	}

	oldSpec, err := GetInstallationLastAppliedSpec(di)
	if err != nil {
		log.Info("fail to get last applied spec")
	} else {
		old.Spec = *oldSpec
	}

	// No spec change and not to delete, it must be status change
	if apiequality.Semantic.DeepEqual(oldSpec, di.Spec) && !isDelete {
		log.V(2).Info("Spec is the same, it should be status change", "oldSpec", oldSpec, "newSpec", di.Spec)
	}
	// Need to take action to update daisy installation

	if err := r.DMM.Sync(old, di); err != nil {
		if IsRequeueError(err) {
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	if !isDelete {
		if !apiequality.Semantic.DeepEqual(oldStatus, di.Status) {
			// after update the installation, the spec will be changed by Client, therefore backup the spec first
			before := di.DeepCopy()
			if err = r.Client.Update(ctx, di); err != nil {
				return ctrl.Result{}, err
			}

			di.Spec = before.Spec
			if err = SetInstallationLastAppliedConfigAnnotation(di); err != nil {
				return ctrl.Result{}, nil
			}

			log.Info("status is different", "oldStatus", oldStatus, "newStatus", di.Status)
			if err = r.Client.Status().Update(ctx, before); err != nil {
				return ctrl.Result{}, err
			}
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

package controllers

import (
	"context"
	"github.com/daisy/daisy-operator/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/controllers/daisymanager"
)

// DaisyTemplateReconciler reconciles a DaisyTemplate object
type DaisyTemplateReconciler struct {
	client.Client
	CfgMgr *daisymanager.ConfigManager
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=daisy.com,resources=daisytemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=daisy.com,resources=daisytemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=daisy.com,resources=daisytemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the DaisyTemplate object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.0/pkg/reconcile
func (r *DaisyTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("Namespace", req.Namespace, "Installation", req.Name)

	var err error
	dt := v1.DaisyTemplate{}
	if err = r.Client.Get(ctx, req.NamespacedName, &dt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Delete
	if !dt.ObjectMeta.DeletionTimestamp.IsZero() {
		r.CfgMgr.Config().DeleteDaisyTemplate((*v1.DaisyInstallation)(&dt))
		return reconcile.Result{}, nil
	} else {
		log.V(3).Info("Registering finalizer for Daisy Template")

		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !util.InArray(daisymanager.DaisyFinalizer, dt.GetFinalizers()) {
			dt.SetFinalizers(append(dt.GetFinalizers(), daisymanager.DaisyFinalizer))
			//if err := m.deps.Client.Update(context.Background(), cur); err != nil {
			//	return err
			//}
		}
	}

	//cur := r.CfgMgr.Config().
	//// Add
	//r.CfgMgr.Config().AddDaisyTemplate((*v1.DaisyInstallation)(&dt))
	//log.Info("Add Daisy Template")
	//
	//// Update
	//log.Info("Update Daisy Template")
	//if old.ObjectMeta.ResourceVersion == new.ObjectMeta.ResourceVersion {
	//	log.V(2).Info("Update Daisy Template, ResourceVersion did not change", "ResourceVersion", old.ObjectMeta.ResourceVersion)
	//	// No need to react
	//	return ctrl.Result{},nil
	//}

	log.Info("Update Daisy Template successfully")
	//r.CfgMgr.Config().UpdateDaisyTemplate((*v1.DaisyInstallation)(&dt))
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DaisyTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.DaisyTemplate{}).
		Complete(r)
}

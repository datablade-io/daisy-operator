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

package v1

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var daisyinstallationlog = logf.Log.WithName("daisyinstallation-webhook")
var inclient client.Client

func (r *DaisyInstallation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	inclient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-daisy-com-v1-daisyinstallation,mutating=true,failurePolicy=fail,sideEffects=None,groups=daisy.com,resources=daisyinstallations,verbs=create;update,versions=v1,name=mdaisyinstallation.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &DaisyInstallation{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DaisyInstallation) Default() {
	daisyinstallationlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	// To avoid call Default() multi times when requeue
	if r.Labels == nil {
		r.Labels = make(map[string]string)
	}
	r.Labels["daisy.com/daisy-webhook-default"] = "handled"
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-daisy-com-v1-daisyinstallation,mutating=false,failurePolicy=fail,sideEffects=None,groups=daisy.com,resources=daisyinstallations,verbs=create;update,versions=v1,name=vdaisyinstallation.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &DaisyInstallation{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DaisyInstallation) ValidateCreate() error {
	daisyinstallationlog.Info("validate create", "name", r.Name)
	if err := ValidateStorageLimit(r); err != nil {
		return err
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DaisyInstallation) ValidateUpdate(old runtime.Object) error {
	daisyinstallationlog.Info("validate update", "name", r.Name)
	if err := ValidateStorageLimit(r); err != nil {
		return err
	}
	if err := ValidateScalInOut(r, old); err != nil {
		return err
	}
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DaisyInstallation) ValidateDelete() error {
	daisyinstallationlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

func ValidateStorageLimit(installation *DaisyInstallation) error {
	if installation.Spec.Templates.VolumeClaimTemplates != nil {
		if installation.Spec.Defaults.Templates.DataVolumeClaimTemplate != "" {
			dataVolume := getVolume(installation, installation.Spec.Defaults.Templates.DataVolumeClaimTemplate)
			if dataVolume != nil {
				ds := dataVolume.(*VolumeClaimTemplate).Spec.Resources.Requests.Storage()
				dataStorageMinSize := resource.MustParse("1Gi")
				if ds.Cmp(dataStorageMinSize) < 0 {
					err := errors.Errorf("unreasonable data volume storage")
					return err
				}
			} else {
				err := errors.Errorf("data volume " + installation.Spec.Defaults.Templates.DataVolumeClaimTemplate + " not config, config it")
				return err
			}
		}
		if installation.Spec.Defaults.Templates.LogVolumeClaimTemplate != "" {
			logVolume := getVolume(installation, installation.Spec.Defaults.Templates.LogVolumeClaimTemplate)
			if logVolume != nil {
				ls := logVolume.(*VolumeClaimTemplate).Spec.Resources.Requests.Storage()
				logStorageMinSize := resource.MustParse("100Mi")
				if ls.Cmp(logStorageMinSize) < 0 {
					err := errors.Errorf("unreasonable log volume storage")
					return err
				}
			} else {
				err := errors.Errorf("log volume " + installation.Spec.Defaults.Templates.LogVolumeClaimTemplate + " not config, config it")
				return err
			}
		}
		log.Info("ValidateStorageLimit ok")
	}
	return nil
}

func getVolume(di *DaisyInstallation, name string) interface{} {
	if di.Spec.Templates.VolumeClaimTemplates != nil {
		for _, value := range di.Spec.Templates.VolumeClaimTemplates {
			if value.Name == name {
				return &value
			}
		}
	}
	return nil
}

func ValidateScalInOut(newObj *DaisyInstallation, old runtime.Object) error {
	if newObj.Spec.Templates.VolumeClaimTemplates != nil && old != nil {
		oldObj := old.(*DaisyInstallation)
		for i := 0; i < len(newObj.Spec.Templates.VolumeClaimTemplates); i++ {
			if err := validVolume(newObj, oldObj, newObj.Spec.Templates.VolumeClaimTemplates[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
func validVolume(newObj *DaisyInstallation, oldObj *DaisyInstallation, temp VolumeClaimTemplate) error {
	oldDataVolume := temp
	newDataVolume := getVolume(newObj, temp.Name)
	cmp := CompareStorageRequests(oldDataVolume.Spec.Resources, newDataVolume.(*VolumeClaimTemplate).Spec.Resources)
	switch {
	case cmp.Increase:
		// storage increase requested: ensure the storage class allows volume expansion
		if err := ValidateStorageClass(newObj); err != nil {
			return err
		}
		//if err := ValidateStorageCapacity(newObj,oldObj); err != nil {
		//	return err
		//}
	case cmp.Decrease:
		// storage decrease is not supported
		err := errors.Errorf("scal in not support")
		return err
	}
	if len(newObj.Spec.Configuration.Clusters) < len(oldObj.Spec.Configuration.Clusters) {
		err := errors.Errorf("scal in not support")
		return err
	}
	for key, value := range newObj.Spec.Configuration.Clusters {
		if value.Layout.ShardsCount < oldObj.Spec.Configuration.Clusters[key].Layout.ShardsCount || value.Layout.ReplicasCount < oldObj.Spec.Configuration.Clusters[key].Layout.ReplicasCount {
			err := errors.Errorf("scal in not support")
			return err
		}
	}
	return nil
}
func ValidateStorageClass(installation *DaisyInstallation) error {
	if installation.Spec.Templates.VolumeClaimTemplates != nil {
		var updatedClaim corev1.PersistentVolumeClaim
		updatedClaim.Spec = getVolume(installation, installation.Spec.Defaults.Templates.DataVolumeClaimTemplate).(*VolumeClaimTemplate).Spec
		var noValidateStorageClass = installation.Spec.Validate.NoValidateStorageClass
		if err := EnsureClaimSupportsExpansion(inclient, updatedClaim, !noValidateStorageClass); err != nil {
			return err
		}
		updatedClaim.Spec = getVolume(installation, installation.Spec.Defaults.Templates.LogVolumeClaimTemplate).(*VolumeClaimTemplate).Spec
		if err := EnsureClaimSupportsExpansion(inclient, updatedClaim, !noValidateStorageClass); err != nil {
			return err
		}
	}
	return nil
}

func EnsureClaimSupportsExpansion(k8sClient client.Client, claim corev1.PersistentVolumeClaim, validateStorageClass bool) error {
	if !validateStorageClass {
		log.V(1).Info("Skipping storage class validation")
		return nil
	}
	_, err := getStorageClass(k8sClient, claim)
	if err != nil {
		return err
	}
	//if !allowsVolumeExpansion(sc) {
	//	return fmt.Errorf("claim %s (storage class %s) does not support volume expansion", claim.Name, sc.Name)
	//}
	return nil
}

// getStorageClass returns the storage class specified by the given claim,
// or the default storage class if the claim does not specify any.
func getStorageClass(k8sClient client.Client, claim corev1.PersistentVolumeClaim) (storagev1.StorageClass, error) {
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		return getDefaultStorageClass(k8sClient)
	}
	var sc storagev1.StorageClass
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: *claim.Spec.StorageClassName}, &sc); err != nil {
		return storagev1.StorageClass{}, fmt.Errorf("cannot retrieve storage class: %w", err)
	}
	return sc, nil
}

// allowsVolumeExpansion returns true if the given storage class allows volume expansion.
func allowsVolumeExpansion(sc storagev1.StorageClass) bool {
	return sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion
}

// getDefaultStorageClass returns the default storage class in the current k8s cluster,
// or an error if there is none.
func getDefaultStorageClass(k8sClient client.Client) (storagev1.StorageClass, error) {
	var scs storagev1.StorageClassList
	if err := k8sClient.List(context.Background(), &scs); err != nil {
		return storagev1.StorageClass{}, err
	}
	for _, sc := range scs.Items {
		if isDefaultStorageClass(sc) {
			return sc, nil
		}
	}
	return storagev1.StorageClass{}, errors.New("no default storage class found")
}

// isDefaultStorageClass inspects the given storage class and returns true if it is annotated as the default one.
func isDefaultStorageClass(sc storagev1.StorageClass) bool {
	if len(sc.Annotations) == 0 {
		return false
	}
	if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
		sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
		return true
	}
	return false
}

// CompareStorageRequests compares storage requests in the given resource requirements.
// It returns a zero-ed StorageComparison in case one of the requests is zero (value not set: comparison not possible).
func CompareStorageRequests(initial corev1.ResourceRequirements, updated corev1.ResourceRequirements) StorageComparison {
	initialSize := initial.Requests.Storage()
	updatedSize := updated.Requests.Storage()
	if initialSize.IsZero() || updatedSize.IsZero() {
		return StorageComparison{}
	}
	switch updatedSize.Cmp(*initialSize) {
	case -1: // decrease
		return StorageComparison{Decrease: true}
	case 1: // increase
		return StorageComparison{Increase: true}
	default: // same size
		return StorageComparison{}
	}
}

type StorageComparison struct {
	Increase bool
	Decrease bool
}

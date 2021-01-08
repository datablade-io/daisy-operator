/*
 * Copyright (c) 2020. Daisy Team, 360, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controllers

import (
	"encoding/json"
	"fmt"
	v1 "github.com/daisy/daisy-operator/api/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/klog/v2"
)

const (
	// LastAppliedPodTemplate is annotation key of the last applied pod template
	LastAppliedPodTemplate = "daisy.com/last-applied-podtemplate"

	// LastAppliedConfigAnnotation is annotation key of last applied configuration
	LastAppliedConfigAnnotation = "daisy.com/last-applied-configuration"
)

// GetDeploymentLastAppliedPodTemplate set last applied pod template from Deployment's annotation
func GetDeploymentLastAppliedPodTemplate(dep *appsv1.Deployment) (*corev1.PodSpec, error) {
	applied, ok := dep.Annotations[LastAppliedPodTemplate]
	if !ok {
		return nil, fmt.Errorf("deployment:[%s/%s] not found spec's apply config", dep.GetNamespace(), dep.GetName())
	}
	podSpec := &corev1.PodSpec{}
	err := json.Unmarshal([]byte(applied), podSpec)
	if err != nil {
		return nil, err
	}
	return podSpec, nil
}

// GetInstallationLastAppliedSpec get last applied daisy installation spec template from DaisyInstallation's annotation
func GetInstallationLastAppliedSpec(di *v1.DaisyInstallation) (*v1.DaisyInstallationSpec, error) {
	applied, ok := di.Annotations[LastAppliedConfigAnnotation]
	if !ok {
		return nil, fmt.Errorf("daisyinstallation:[%s/%s] not found spec's apply config", di.GetNamespace(), di.GetName())
	}
	instSpec := &v1.DaisyInstallationSpec{}
	err := json.Unmarshal([]byte(applied), instSpec)
	if err != nil {
		return nil, err
	}
	return instSpec, nil
}

// DeploymentPodSpecChanged checks whether the new deployment differs with the old one's last-applied-config
func DeploymentPodSpecChanged(newDep *appsv1.Deployment, oldDep *appsv1.Deployment) bool {
	lastAppliedPodTemplate, err := GetDeploymentLastAppliedPodTemplate(oldDep)
	if err != nil {
		klog.Warningf("error get last-applied-config of deployment %s/%s: %v", oldDep.Namespace, oldDep.Name, err)
		return true
	}
	return !apiequality.Semantic.DeepEqual(newDep.Spec.Template.Spec, lastAppliedPodTemplate)
}

// SetServiceLastAppliedConfigAnnotation set last applied config info to Service's annotation
func SetServiceLastAppliedConfigAnnotation(svc *corev1.Service) error {
	b, err := json.Marshal(svc.Spec)
	if err != nil {
		return err
	}
	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}
	svc.Annotations[LastAppliedConfigAnnotation] = string(b)
	return nil
}

// SetInstallationLastAppliedConfigAnnotation set last applied config info to Service's annotation
func SetInstallationLastAppliedConfigAnnotation(di *v1.DaisyInstallation) error {
	b, err := json.Marshal(di.Spec)
	if err != nil {
		return err
	}
	if di.Annotations == nil {
		di.Annotations = map[string]string{}
	}
	di.Annotations[LastAppliedConfigAnnotation] = string(b)
	di.Status.PrevSpec = di.Spec
	return nil
}

// SetStatefulSetLastAppliedConfigAnnotation set last applied config to Statefulset's annotation
func SetStatefulSetLastAppliedConfigAnnotation(set *appsv1.StatefulSet) error {
	setApply, err := json.Marshal(set.Spec)
	if err != nil {
		return err
	}
	if set.Annotations == nil {
		set.Annotations = map[string]string{}
	}
	set.Annotations[LastAppliedConfigAnnotation] = string(setApply)
	return nil
}

// ServiceEqual compares the new Service's spec with old Service's last applied config
func ServiceEqual(newSvc, oldSvc *corev1.Service) (bool, error) {
	oldSpec := corev1.ServiceSpec{}
	if lastAppliedConfig, ok := oldSvc.Annotations[LastAppliedConfigAnnotation]; ok {
		err := json.Unmarshal([]byte(lastAppliedConfig), &oldSpec)
		if err != nil {
			klog.Errorf("unmarshal ServiceSpec: [%s/%s]'s applied config failed,error: %v", oldSvc.GetNamespace(), oldSvc.GetName(), err)
			return false, err
		}
		return apiequality.Semantic.DeepEqual(oldSpec, newSvc.Spec), nil
	}
	return false, nil
}

// statefulSetEqual compares the new Statefulset's spec with old Statefulset's last applied config
func StatefulSetEqual(new *appsv1.StatefulSet, old *appsv1.StatefulSet) bool {
	// The annotations in old sts may include LastAppliedConfigAnnotation
	tmpAnno := map[string]string{}
	for k, v := range old.Annotations {
		if k != LastAppliedConfigAnnotation {
			tmpAnno[k] = v
		}
	}
	if !apiequality.Semantic.DeepEqual(new.Annotations, tmpAnno) {
		return false
	}
	oldConfig := appsv1.StatefulSetSpec{}
	if lastAppliedConfig, ok := old.Annotations[LastAppliedConfigAnnotation]; ok {
		err := json.Unmarshal([]byte(lastAppliedConfig), &oldConfig)
		if err != nil {
			klog.Errorf("unmarshal Statefulset: [%s/%s]'s applied config failed,error: %v", old.GetNamespace(), old.GetName(), err)
			return false
		}
		// oldConfig.Template.Annotations may include LastAppliedConfigAnnotation to keep backward compatiability
		// Please check detail in https://github.com/pingcap/tidb-operator/pull/1489
		tmpTemplate := oldConfig.Template.DeepCopy()
		delete(tmpTemplate.Annotations, LastAppliedConfigAnnotation)
		return apiequality.Semantic.DeepEqual(oldConfig.Replicas, new.Spec.Replicas) &&
			apiequality.Semantic.DeepEqual(*tmpTemplate, new.Spec.Template) &&
			apiequality.Semantic.DeepEqual(oldConfig.UpdateStrategy, new.Spec.UpdateStrategy)
	}
	return false
}

// templateEqual compares the new podTemplateSpec's spec with old podTemplateSpec's last applied config
func templateEqual(new *appsv1.StatefulSet, old *appsv1.StatefulSet) bool {
	oldStsSpec := appsv1.StatefulSetSpec{}
	lastAppliedConfig, ok := old.Annotations[LastAppliedConfigAnnotation]
	if ok {
		err := json.Unmarshal([]byte(lastAppliedConfig), &oldStsSpec)
		if err != nil {
			klog.Errorf("unmarshal PodTemplate: [%s/%s]'s applied config failed,error: %v", old.GetNamespace(), old.GetName(), err)
			return false
		}
		return apiequality.Semantic.DeepEqual(oldStsSpec.Template.Spec, new.Spec.Template.Spec)
	}
	return false
}

func IngressEqual(newIngress, oldIngres *extensionsv1beta1.Ingress) (bool, error) {
	oldIngressSpec := extensionsv1beta1.IngressSpec{}
	if lastAppliedConfig, ok := oldIngres.Annotations[LastAppliedConfigAnnotation]; ok {
		err := json.Unmarshal([]byte(lastAppliedConfig), &oldIngressSpec)
		if err != nil {
			klog.Errorf("unmarshal IngressSpec: [%s/%s]'s applied config failed,error: %v", oldIngres.GetNamespace(), oldIngres.GetName(), err)
			return false, err
		}
		return apiequality.Semantic.DeepEqual(oldIngressSpec, newIngress.Spec), nil
	}
	return false, nil
}

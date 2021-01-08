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
	"context"
	"fmt"
	v1 "github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/label"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
)

const (
	// ImagePullBackOff is the pod state of image pull failed
	ImagePullBackOff = "ImagePullBackOff"
	// ErrImagePull is the pod state of image pull failed
	ErrImagePull = "ErrImagePull"
)

type memberContext struct {
	Namespace    string
	Installation string
	CurShard     string
	CurReplica   string
	CurCluster   string
	Clusters     map[string]v1.Cluster
}

// updateStatefulSet is a template function to update the statefulset of components
func UpdateStatefulSet(client client.Client, object runtime.Object, newSet, oldSet *apps.StatefulSet) error {
	isOrphan := metav1.GetControllerOf(oldSet) == nil
	if newSet.Annotations == nil {
		newSet.Annotations = map[string]string{}
	}
	if oldSet.Annotations == nil {
		oldSet.Annotations = map[string]string{}
	}
	if !StatefulSetEqual(newSet, oldSet) || isOrphan {
		set := *oldSet
		// Retain the deprecated last applied pod template annotation for backward compatibility
		var podConfig string
		var hasPodConfig bool
		if oldSet.Spec.Template.Annotations != nil {
			podConfig, hasPodConfig = oldSet.Spec.Template.Annotations[LastAppliedConfigAnnotation]
		}
		set.Spec.Template = newSet.Spec.Template
		if hasPodConfig {
			if set.Spec.Template.Annotations == nil {
				set.Spec.Template.Annotations = map[string]string{}
			}
			set.Spec.Template.Annotations[LastAppliedConfigAnnotation] = podConfig
		}
		set.Annotations = newSet.Annotations
		v, ok := oldSet.Annotations[label.AnnStsLastSyncTimestamp]
		if ok {
			set.Annotations[label.AnnStsLastSyncTimestamp] = v
		}
		*set.Spec.Replicas = *newSet.Spec.Replicas
		set.Spec.UpdateStrategy = newSet.Spec.UpdateStrategy
		if isOrphan {
			set.OwnerReferences = newSet.OwnerReferences
			set.Labels = newSet.Labels
		}
		err := SetStatefulSetLastAppliedConfigAnnotation(&set)
		if err != nil {
			return err
		}

		return client.Update(context.Background(), &set)
	}
	return nil
}

// getConfigMapCHICommon creates new corev1.ConfigMap
func (m *DaisyMemberManager) getConfigMapCHICommon(di *v1.DaisyInstallation) *corev1.ConfigMap {
	cmLabel := label.New().Namespace(di.GetNamespace()).Instance(di.GetInstanceName())
	ctx := &memberContext{
		Namespace:    di.Namespace,
		Installation: di.Name,
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            CreateConfigMapCommonName(ctx),
			Namespace:       di.Namespace,
			Labels:          cmLabel.ConfigMapType(label.ConfigMapValueCommon).Labels(),
			OwnerReferences: []metav1.OwnerReference{GetOwnerRef(di)},
		},
		// Data contains several sections which are to be several xml config files
		Data: m.cfgGen.CreateConfigsCommon(),
	}
}

// getConfigMapCHICommonUsers creates new corev1.ConfigMap
func (m *DaisyMemberManager) getConfigMapCHICommonUsers(di *v1.DaisyInstallation) *corev1.ConfigMap {
	cmLabel := label.New().Namespace(di.GetNamespace()).Instance(di.GetInstanceName())
	ctx := &memberContext{
		Namespace:    di.Namespace,
		Installation: di.Name,
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            CreateConfigMapCommonUsersName(ctx),
			Namespace:       di.Namespace,
			Labels:          cmLabel.ConfigMapType(label.ConfigMapValueCommonUsers).Labels(),
			OwnerReferences: []metav1.OwnerReference{GetOwnerRef(di)},
		},
		// Data contains several sections which are to be several xml config files
		Data: m.cfgGen.CreateConfigsUsers(),
	}
}

// createConfigMapHost creates new corev1.ConfigMap
func (m *DaisyMemberManager) getConfigMapReplica(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) *corev1.ConfigMap {
	cmLabel := label.New().Namespace(ctx.Namespace).Instance(ctx.Installation)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            CreateConfigMapPodName(ctx),
			Namespace:       ctx.Namespace,
			Labels:          cmLabel.ConfigMapType(label.ConfigMapValueHost).Labels(),
			OwnerReferences: []metav1.OwnerReference{GetOwnerRef(di)},
		},
		Data: m.cfgGen.CreateConfigsHost(ctx, di, r),
	}
}

func getNewSetForDaisyCluster(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) (*apps.StatefulSet, error) {
	statefulSetName := CreateStatefulSetName(replica)
	serviceName := CreateStatefulSetServiceName(replica)

	// Create apps.StatefulSet object
	replicasNum := int32(1)
	revisionHistoryLimit := int32(10)
	replicaLabel := labelReplica(ctx, replica)
	// StatefulSet has additional label - ZK config fingerprint

	statefulSet := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            statefulSetName,
			Namespace:       di.GetNamespace(),
			Labels:          replicaLabel,
			OwnerReferences: []metav1.OwnerReference{GetOwnerRef(di)},
		},
		Spec: apps.StatefulSetSpec{
			Replicas:    &replicasNum,
			ServiceName: serviceName,
			Selector:    replicaLabel.LabelSelector(),

			// IMPORTANT
			// Template is to be setup later
			Template: corev1.PodTemplateSpec{},

			// IMPORTANT
			// VolumeClaimTemplates are to be setup later
			VolumeClaimTemplates: nil,

			PodManagementPolicy: apps.OrderedReadyPodManagement,
			UpdateStrategy: apps.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}

	setupStatefulSetPodTemplate(ctx, statefulSet, replica)
	//setupStatefulSetVolumeClaimTemplates(statefulSet, replica)

	return statefulSet, nil
}

// newVolumeForConfigMap returns corev1.Volume object with defined name
func newVolumeForConfigMap(name string) corev1.Volume {
	var defaultMode int32 = 0644
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
				DefaultMode: &defaultMode,
			},
		},
	}
}

// newVolumeMount returns corev1.VolumeMount object with name and mount path
func newVolumeMount(name, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
	}
}

// setupStatefulSetPodTemplate performs PodTemplate setup of StatefulSet
func setupStatefulSetPodTemplate(ctx *memberContext, statefulSet *apps.StatefulSet, replica *v1.Replica) {
	// Process Pod Template
	podTemplate := getPodTemplate(replica)
	statefulSetApplyPodTemplate(statefulSet, podTemplate, replica)

	// Post-process StatefulSet
	ensureStatefulSetIntegrity(statefulSet, replica)
	personalizeStatefulSetTemplate(ctx, statefulSet, replica)
}

func ensureStatefulSetIntegrity(statefulSet *apps.StatefulSet, replica *v1.Replica) {
	ensureClickHouseContainer(statefulSet, replica)
	//ensureNamedPortsSpecified(statefulSet, host)
}

func ensureClickHouseContainer(statefulSet *apps.StatefulSet, _ *v1.Replica) {
	if _, ok := getClickHouseContainer(statefulSet); !ok {
		// No ClickHouse container available
		addContainer(
			&statefulSet.Spec.Template.Spec,
			newDefaultDaisyContainer(),
		)
	}
}

func personalizeStatefulSetTemplate(ctx *memberContext, statefulSet *apps.StatefulSet, replica *v1.Replica) {
	//statefulSetName := CreateStatefulSetName(replica)

	// Ensure pod created by this StatefulSet has alias 127.0.0.1
	statefulSet.Spec.Template.Spec.HostAliases = []corev1.HostAlias{
		{
			IP:        "127.0.0.1",
			Hostnames: []string{CreatePodHostname(replica)},
		},
	}

	// Setup volumes based on ConfigMaps into Pod Template
	setupConfigMapVolumes(ctx, statefulSet)

	// In case we have default LogVolumeClaimTemplate specified - need to append log container to Pod Template
	//if host.Templates.LogVolumeClaimTemplate != "" {
	//	addContainer(&statefulSet.Spec.Template.Spec, corev1.Container{
	//		Name:  ClickHouseLogContainerName,
	//		Image: defaultBusyBoxDockerImage,
	//		Command: []string{
	//			"/bin/sh", "-c", "--",
	//		},
	//		Args: []string{
	//			"while true; do sleep 30; done;",
	//		},
	//	})
	//	log.V(1).Infof("setupStatefulSetPodTemplate() add log container for statefulSet %s", statefulSetName)
	//}
}

// CreateConfigMapPodName returns a name for a ConfigMap for ClickHouse pod
func CreateConfigMapPodName(ctx *memberContext) string {
	return newNameMacroReplacer(ctx).Replace(configMapReplicaNamePattern)
}

// CreateConfigMapCommonName returns a name for a ConfigMap for replica's common config
func CreateConfigMapCommonName(ctx *memberContext) string {
	return newNameMacroReplacer(ctx).Replace(configMapCommonNamePattern)
}

// CreateConfigMapCommonUsersName returns a name for a ConfigMap for replica's common config
func CreateConfigMapCommonUsersName(ctx *memberContext) string {
	return newNameMacroReplacer(ctx).Replace(configMapCommonUsersNamePattern)
}

// CreateInstallationServiceName creates a name of a Installation Service resource
func CreateInstallationServiceName(di *v1.DaisyInstallation) string {
	// Name can be generated either from default name pattern,
	// or from personal name pattern provided in ServiceTemplate

	// Start with default name pattern
	pattern := InstallationServiceNamePattern

	//TODO: ServiceTemplate may have personal name pattern specified

	// Create Service name based on name pattern available
	return newNameMacroReplacerInstallation(di).Replace(pattern)
}

// DaisyConfigNetworksHostRegexpTemplate: di-{di}-[^.]+\\d+-\\d+\\.{namespace}.svc.cluster.local$"
func CreatePodRegexp(di *v1.DaisyInstallation, template string) string {
	// TODO: DaisyConfigNetworksHostRegexpTemplate is defined in operator config:
	return newNameMacroReplacerInstallation(di).Replace(template)
}

func newNameMacroReplacerInstallation(di *v1.DaisyInstallation) *strings.Replacer {
	return strings.NewReplacer(
		macrosNamespace, di.Namespace,
		macrosInsName, di.Name,
	)
}

func newNameMacroReplacer(ctx *memberContext) *strings.Replacer {
	return strings.NewReplacer(
		macrosNamespace, ctx.Namespace,
		macrosInsName, ctx.Installation,
		macrosClusterName, ctx.CurCluster,
		macrosReplicaName, ctx.CurReplica,
	)
}

// CreatePodHostname returns a name of a Pod of a ClickHouse instance
func CreatePodHostname(replica *v1.Replica) string {
	// Pod has no own hostname - redirect to appropriate Service
	return CreateStatefulSetServiceName(replica)
}

// CreateStatefulSetServiceName returns a name of a StatefulSet-related Service for ClickHouse instance
func CreateStatefulSetServiceName(replica *v1.Replica) string {
	// Create Service name based on name pattern available
	return replica.Name
}

// setupConfigMapVolumes adds to each container in the Pod VolumeMount objects with
func setupConfigMapVolumes(ctx *memberContext, statefulSetObject *apps.StatefulSet) {
	configMapMacrosName := CreateConfigMapPodName(ctx)
	configMapCommonName := CreateConfigMapCommonName(ctx)
	configMapCommonUsersName := CreateConfigMapCommonUsersName(ctx)

	// Add all ConfigMap objects as Volume objects of type ConfigMap
	statefulSetObject.Spec.Template.Spec.Volumes = append(
		statefulSetObject.Spec.Template.Spec.Volumes,
		newVolumeForConfigMap(configMapCommonName),
		newVolumeForConfigMap(configMapCommonUsersName),
		newVolumeForConfigMap(configMapMacrosName),
	)

	// And reference these Volumes in each Container via VolumeMount
	// So Pod will have ConfigMaps mounted as Volumes
	for i := range statefulSetObject.Spec.Template.Spec.Containers {
		// Convenience wrapper
		container := &statefulSetObject.Spec.Template.Spec.Containers[i]
		// Append to each Container current VolumeMount's to VolumeMount's declared in template
		container.VolumeMounts = append(
			container.VolumeMounts,
			newVolumeMount(configMapCommonName, dirPathCommonConfig),
			newVolumeMount(configMapCommonUsersName, dirPathUsersConfig),
			newVolumeMount(configMapMacrosName, dirPathHostConfig),
		)
	}
}

func getClickHouseContainer(statefulSet *apps.StatefulSet) (*corev1.Container, bool) {
	if len(statefulSet.Spec.Template.Spec.Containers) > 0 {
		return &statefulSet.Spec.Template.Spec.Containers[0], true
	} else {
		return nil, false
	}
}

// getPodTemplate gets Pod Template to be used to create StatefulSet
func getPodTemplate(replica *v1.Replica) *v1.DaisyPodTemplate {
	statefulSetName := CreateStatefulSetName(replica)

	// Which pod template would be used - either explicitly defined in or a default one
	//podTemplate, ok := replica.GetPodTemplate()
	//if ok {
	//	// Host references known PodTemplate
	//	// Make local copy of this PodTemplate, in order not to spoil the original common-used template
	//	podTemplate = podTemplate.DeepCopy()
	//	log.V(1).Infof("getPodTemplate() statefulSet %s use custom template %s", statefulSetName, podTemplate.Name)
	//} else {
	// Host references UNKNOWN PodTemplate, will use default one
	podTemplate := newDefaultPodTemplate(statefulSetName)
	//	log.V(1).Infof("getPodTemplate() statefulSet %s use default generated template", statefulSetName)
	//}

	// Here we have local copy of Pod Template, to be used to create StatefulSet
	// Now we can customize this Pod Template for particular host

	//c.labeler.prepareAffinity(podTemplate, host)

	return podTemplate
}

// newDefaultPodTemplate returns default Pod Template to be used with StatefulSet
func newDefaultPodTemplate(name string) *v1.DaisyPodTemplate {
	podTemplate := &v1.DaisyPodTemplate{
		Name: name,
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{},
			Volumes:    []corev1.Volume{},
		},
	}

	addContainer(
		&podTemplate.Spec,
		newDefaultDaisyContainer(),
	)

	return podTemplate
}

// addContainer adds container to DaisyPodTemplate
func addContainer(podSpec *corev1.PodSpec, container corev1.Container) {
	podSpec.Containers = append(podSpec.Containers, container)
}

// newDefaultDaisyContainer returns default ClickHouse Container
func newDefaultDaisyContainer() corev1.Container {
	return corev1.Container{
		Name:  ClickHouseContainerName,
		Image: defaultClickHouseDockerImage,
		Ports: []corev1.ContainerPort{
			{
				Name:          daisyDefaultHTTPPortName,
				ContainerPort: daisyDefaultHTTPPortNumber,
			},
			{
				Name:          daisyDefaultTCPPortName,
				ContainerPort: daisyDefaultTCPPortNumber,
			},
			{
				Name:          daisyDefaultInterserverHTTPPortName,
				ContainerPort: daisyDefaultInterserverHTTPPortNumber,
			},
		},
		LivenessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ping",
					Port: intstr.Parse(daisyDefaultHTTPPortName),
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
		ReadinessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/?" +
						"user=" + url.QueryEscape(CHUsername) +
						"&password=" + url.QueryEscape(CHPassword) +
						"&query=" +
						// SELECT throwIf(count()=0) FROM system.clusters WHERE cluster='all-sharded' AND is_local
						url.QueryEscape(
							fmt.Sprintf(
								"SELECT throwIf(count()=0) FROM system.clusters WHERE cluster='%s' AND is_local",
								allShardsOneReplicaClusterName,
							),
						),
					Port: intstr.Parse(daisyDefaultHTTPPortName),
					HTTPHeaders: []corev1.HTTPHeader{
						{
							Name:  "Accept",
							Value: "*/*",
						},
					},
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		},
	}
}

// statefulSetApplyPodTemplate fills StatefulSet.Spec.Template with data from provided ChiPodTemplate
func statefulSetApplyPodTemplate(
	statefulSet *apps.StatefulSet,
	template *v1.DaisyPodTemplate,
	replica *v1.Replica,
) {
	// StatefulSet's pod template is not directly compatible with ChiPodTemplate,
	// we need to extract some fields from ChiPodTemplate and apply on StatefulSet
	statefulSet.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: template.Name,
			Labels: MergeStringMaps(
				//c.labeler.getLabelsHostScope(replica, true),
				statefulSet.Labels,
				template.ObjectMeta.Labels,
			),
			Annotations: MergeStringMaps(
				//c.labeler.getAnnotationsHostScope(replica),
				statefulSet.Annotations,
				template.ObjectMeta.Annotations,
			),
		},
		Spec: *template.Spec.DeepCopy(),
	}
}

// MergeStringMaps inserts (and overwrites) data into dst map object from src
func MergeStringMaps(dst, src map[string]string, keys ...string) map[string]string {
	if src == nil {
		// Nothing to merge from
		return dst
	}

	if dst == nil {
		dst = make(map[string]string)
	}

	// Place key->value pair from src into dst

	if len(keys) == 0 {
		// No explicitly specified keys to merge, just merge the whole src
		for key := range src {
			dst[key] = src[key]
		}
	} else {
		// We have explicitly specified list of keys to merge from src
		for _, key := range keys {
			if value, ok := src[key]; ok {
				dst[key] = value
			}
		}
	}

	return dst
}

// CreateStatefulSetName creates a name of a StatefulSet for ClickHouse instance
func CreateStatefulSetName(replica *v1.Replica) string {
	// Name can be generated either from default name pattern,
	// or from personal name pattern provided in PodTemplate

	// Create StatefulSet name based on name pattern available
	return replica.Name
}

func labelReplica(ctx *memberContext, replica *v1.Replica) label.Label {
	return label.New().Instance(ctx.Installation).Cluster(ctx.CurCluster).ShardName(ctx.CurShard).ReplicaName(replica.Name)
}

func getPodLabelForReplica(l label.Label) label.Label {
	//TODO: add more pod specific labels
	return l
}

// ExtractLastOrdinal returns the last ordinal number of replica or shard name, -1 means invalid name and no match ordinal can be extracted
func ExtractLastOrdinal(name string) int {
	var r int = -1
	var err error
	if len(name) > 0 {
		p := strings.Split(name, "-")
		if len(p) >= 3 {
			if r, err = strconv.Atoi(p[len(p)-1]); err != nil {
				return r
			}
		}
	}
	return r
}

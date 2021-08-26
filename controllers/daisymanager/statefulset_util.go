package daisymanager

import (
	"context"
	"fmt"
	"net/url"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/k8s"
	"github.com/daisy/daisy-operator/pkg/label"
)

// updateStatefulSet is a template function to update the statefulset of components
func UpdateStatefulSet(client client.Client, object runtime.Object, newSet, oldSet *apps.StatefulSet) error {
	isOrphan := metav1.GetControllerOf(oldSet) == nil
	if newSet.Annotations == nil {
		newSet.Annotations = map[string]string{}
	}
	if oldSet.Annotations == nil {
		oldSet.Annotations = map[string]string{}
	}
	if !k8s.StatefulSetEqual(newSet, oldSet) || isOrphan {
		set := *oldSet
		// Retain the deprecated last applied pod template annotation for backward compatibility
		var podConfig string
		var hasPodConfig bool
		if oldSet.Spec.Template.Annotations != nil {
			podConfig, hasPodConfig = oldSet.Spec.Template.Annotations[k8s.LastAppliedConfigAnnotation]
		}
		set.Spec.Template = newSet.Spec.Template
		if hasPodConfig {
			if set.Spec.Template.Annotations == nil {
				set.Spec.Template.Annotations = map[string]string{}
			}
			set.Spec.Template.Annotations[k8s.LastAppliedConfigAnnotation] = podConfig
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
		err := k8s.SetStatefulSetLastAppliedConfigAnnotation(&set)
		if err != nil {
			return err
		}

		return client.Update(context.Background(), &set)
	}
	return nil
}

func getStatefulSetForReplica(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) (*apps.StatefulSet, error) {
	statefulSetName := replica.Name
	serviceName := CreateStatefulSetServiceName(replica)

	// Create apps.StatefulSet object
	replicasNum := int32(1)
	revisionHistoryLimit := int32(10)
	replicaLabel := labelReplica(ctx, replica)
	// StatefulSet has additional label - ZK config fingerprint

	statefulSet := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            statefulSetName,
			Namespace:       ctx.Namespace,
			Labels:          replicaLabel,
			OwnerReferences: []metav1.OwnerReference{k8s.GetOwnerRef(di)},
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
	setupStatefulSetVolumeClaimTemplates(ctx, statefulSet, replica)

	return statefulSet, nil
}

// statefulSetAppendPVCTemplate appends to StatefulSet.Spec.VolumeClaimTemplates new entry with data from provided 'src' ChiVolumeClaimTemplate
func statefulSetAppendPVCTemplate(
	ctx *memberContext,
	r *v1.Replica,
	statefulSet *apps.StatefulSet,
	volumeClaimTemplate *v1.VolumeClaimTemplate,
) {
	// Ensure VolumeClaimTemplates slice is in place
	if statefulSet.Spec.VolumeClaimTemplates == nil {
		statefulSet.Spec.VolumeClaimTemplates = make([]corev1.PersistentVolumeClaim, 0, 0)
	}

	// Check whether this VolumeClaimTemplate is already listed in statefulSet.Spec.VolumeClaimTemplates
	for i := range statefulSet.Spec.VolumeClaimTemplates {
		// Convenience wrapper
		volumeClaimTemplates := &statefulSet.Spec.VolumeClaimTemplates[i]
		if volumeClaimTemplates.Name == volumeClaimTemplate.Name {
			// This VolumeClaimTemplate is already listed in statefulSet.Spec.VolumeClaimTemplates
			// No need to add it second time
			return
		}
	}

	// VolumeClaimTemplate is not listed in statefulSet.Spec.VolumeClaimTemplates - let's add it
	l := labelReplica(ctx, r)
	persistentVolumeClaim := corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: volumeClaimTemplate.Name,
			// TODO
			//  this has to wait until proper disk inheritance procedure will be available
			// UPDATE
			//  we are close to proper disk inheritance
			// Right now we hit the following error:
			// "Forbidden: updates to statefulset spec for fields other than 'replicas', 'template', and 'updateStrategy' are forbidden"
			Labels: l.Labels(),
		},
		Spec: *volumeClaimTemplate.Spec.DeepCopy(),
	}
	volumeMode := corev1.PersistentVolumeFilesystem
	persistentVolumeClaim.Spec.VolumeMode = &volumeMode

	// Append copy of PersistentVolumeClaimSpec
	statefulSet.Spec.VolumeClaimTemplates = append(statefulSet.Spec.VolumeClaimTemplates, persistentVolumeClaim)
}

// setupStatefulSetApplyVolumeMounts applies `volumeMounts` of a `container`
func setupStatefulSetApplyVolumeMounts(ctx *memberContext, statefulSet *apps.StatefulSet, r *v1.Replica) {
	// Deal with `volumeMounts` of a `container`, located by the path:
	// .spec.templates.podTemplates.*.spec.containers.volumeMounts.*
	// VolumeClaimTemplates, that are directly referenced in Containers' VolumeMount object(s)
	// are appended to StatefulSet's Spec.VolumeClaimTemplates slice
	for i := range statefulSet.Spec.Template.Spec.Containers {
		// Convenience wrapper
		container := &statefulSet.Spec.Template.Spec.Containers[i]
		for j := range container.VolumeMounts {
			// Convenience wrapper
			volumeMount := &container.VolumeMounts[j]
			if volumeClaimTemplate, ok := ctx.GetVolumeClaimTemplate(volumeMount.Name); ok {
				// Found VolumeClaimTemplate to mount by VolumeMount
				statefulSetAppendPVCTemplate(ctx, r, statefulSet, volumeClaimTemplate)
			}
		}
	}
}

// setupStatefulSetApplyVolumeMount applies .templates.volumeClaimTemplates.* to a StatefulSet
func setupStatefulSetApplyVolumeMount(
	ctx *memberContext,
	r *v1.Replica,
	statefulSet *apps.StatefulSet,
	containerName string,
	volumeMount corev1.VolumeMount,
) error {
	log := log.WithValues("StatefulSet", statefulSet.Name, "Container", containerName, "VolumeMount", volumeMount.Name)
	// Sanity checks

	// 1. mountPath has to be reasonable
	if volumeMount.MountPath == "" {
		// No mount path specified
		return nil
	}

	volumeClaimTemplateName := volumeMount.Name

	// 2. volumeClaimTemplateName has to be reasonable
	if volumeClaimTemplateName == "" {
		// No VolumeClaimTemplate specified
		return nil
	}

	// 3. Specified (by volumeClaimTemplateName) VolumeClaimTemplate has to be available as well
	if _, ok := ctx.GetVolumeClaimTemplate(volumeClaimTemplateName); !ok {
		// Incorrect/unknown .templates.VolumeClaimTemplate specified
		log.V(1).Info("Can not find volumeClaimTemplate. Volume claim can not be mounted")
		return nil
	}

	// 4. Specified container has to be available
	container := k8s.GetContainerByName(statefulSet, containerName)
	if container == nil {
		log.V(1).Info("Can not find container. Volume claim can not be mounted")
		return nil
	}

	// Looks like all components are in place

	// Mount specified (by volumeMount.Name) VolumeClaimTemplate into volumeMount.Path (say into '/var/lib/clickhouse')
	//
	// A container wants to have this VolumeClaimTemplate mounted into `mountPath` in case:
	// 1. This VolumeClaimTemplate is NOT already mounted in the container with any VolumeMount (to avoid double-mount of a VolumeClaimTemplate)
	// 2. And specified `mountPath` (say '/var/lib/clickhouse') is NOT already mounted with any VolumeMount (to avoid double-mount/rewrite into single `mountPath`)

	for i := range container.VolumeMounts {
		// Convenience wrapper
		existingVolumeMount := &container.VolumeMounts[i]

		// 1. Check whether this VolumeClaimTemplate is already listed in VolumeMount of this container
		if volumeMount.Name == existingVolumeMount.Name {
			// This .templates.VolumeClaimTemplate is already used in VolumeMount
			log.V(1).Info("setupStatefulSetApplyVolumeClaim(%s) container volumeClaimTemplateName already used")
			return nil
		}

		// 2. Check whether `mountPath` (say '/var/lib/clickhouse') is already mounted
		if volumeMount.MountPath == existingVolumeMount.MountPath {
			// `mountPath` (say /var/lib/clickhouse) is already mounted
			log.V(1).Info("setupStatefulSetApplyVolumeClaim container mountPath already used")
			return nil
		}
	}

	// This VolumeClaimTemplate is not used explicitly by name and `mountPath` (say /var/lib/clickhouse) is not used also.
	// Let's mount this VolumeClaimTemplate into `mountPath` (say '/var/lib/clickhouse') of a container
	if template, ok := ctx.GetVolumeClaimTemplate(volumeClaimTemplateName); ok {
		// Add VolumeClaimTemplate to StatefulSet
		statefulSetAppendPVCTemplate(ctx, r, statefulSet, template)
		// Add VolumeMount to ClickHouse container to `mountPath` point
		container.VolumeMounts = append(
			container.VolumeMounts,
			volumeMount,
		)
	}

	log.V(1).Info("setupStatefulSetApplyVolumeClaim container mounted",
		"VolumeMountPath", volumeMount.MountPath)

	return nil
}

// setupStatefulSetApplyVolumeClaimTemplates applies Data and Log VolumeClaimTemplates on all containers
func setupStatefulSetApplyVolumeClaimTemplates(ctx *memberContext, statefulSet *apps.StatefulSet, r *v1.Replica) {
	// Mount all named (data and log so far) VolumeClaimTemplates into all containers
	for i := range statefulSet.Spec.Template.Spec.Containers {
		// Convenience wrapper
		container := &statefulSet.Spec.Template.Spec.Containers[i]
		_ = setupStatefulSetApplyVolumeMount(ctx, r, statefulSet, container.Name, newVolumeMount(r.Templates.DataVolumeClaimTemplate, dirPathClickHouseData))
		_ = setupStatefulSetApplyVolumeMount(ctx, r, statefulSet, container.Name, newVolumeMount(r.Templates.LogVolumeClaimTemplate, dirPathClickHouseLog))
	}
}

// setupStatefulSetVolumeClaimTemplates performs VolumeClaimTemplate setup for Containers in PodTemplate of a StatefulSet
func setupStatefulSetVolumeClaimTemplates(ctx *memberContext, statefulSet *apps.StatefulSet, r *v1.Replica) {
	setupStatefulSetApplyVolumeMounts(ctx, statefulSet, r)
	setupStatefulSetApplyVolumeClaimTemplates(ctx, statefulSet, r)
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
	podTemplate := getPodTemplate(ctx, replica)
	statefulSetApplyPodTemplate(ctx, statefulSet, podTemplate, replica)

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
	if replica.Templates.LogVolumeClaimTemplate != "" {
		if _, ok := ctx.GetVolumeClaimTemplate(replica.Templates.LogVolumeClaimTemplate); ok {
			addContainer(&statefulSet.Spec.Template.Spec, corev1.Container{
				Name:  ClickHouseLogContainerName,
				Image: ctx.Config.ImageBusyBox,
				Command: []string{
					"/bin/sh", "-c", "--",
				},
				Args: []string{
					"while true; do sleep 30; done;",
				},
			})
			log.V(1).Info("setupStatefulSetPodTemplate() add log container for statefulSet", "StatefulSet", replica.Name)
		}
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
func getPodTemplate(ctx *memberContext, r *v1.Replica) *v1.DaisyPodTemplate {
	log := log.WithValues("StatefulSet", r.Name)
	statefulSetName := r.Name

	// Which pod template would be used - either explicitly defined in or a default one
	podTemplate, ok := ctx.GetDaisyPodTemplate(r.Templates.PodTemplate)
	if ok {
		// Host references known PodTemplate
		// Make local copy of this PodTemplate, in order not to spoil the original common-used template
		podTemplate = podTemplate.DeepCopy()
		log.V(1).Info("getPodTemplate() use specified template", "PodTemplate", podTemplate.Name)
	} else {
		// Host references UNKNOWN PodTemplate, will use default one
		podTemplate = newDefaultPodTemplate(statefulSetName)
		log.V(1).Info("getPodTemplate() use default generated template")
	}

	// Here we have local copy of Pod Template, to be used to create StatefulSet
	// Now we can customize this Pod Template for particular host
	//TODO: handle pod affinity labels
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
						// SELECT throwIf(count()=0) FROM system.clu	sters WHERE cluster='all-sharded' AND is_local
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
	ctx *memberContext,
	statefulSet *apps.StatefulSet,
	template *v1.DaisyPodTemplate,
	r *v1.Replica,
) {
	// StatefulSet's pod template is not directly compatible with ChiPodTemplate,
	// we need to extract some fields from ChiPodTemplate and apply on StatefulSet
	l := labelReplica(ctx, r)
	statefulSet.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name: template.Name,
			Labels: MergeStringMaps(
				l.Labels(),
				statefulSet.Labels,
			),
			Annotations: statefulSet.Annotations,
		},
		Spec: *template.Spec.DeepCopy(),
	}
}

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

package daisymanager

import (
	"context"
	"fmt"
	"github.com/daisy/daisy-operator/pkg/k8s"
	"github.com/daisy/daisy-operator/pkg/version"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
	"strconv"
	"strings"
	"time"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1 "github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/label"
)

const (
	// ImagePullBackOff is the pod state of image pull failed
	ImagePullBackOff = "ImagePullBackOff"
	// ErrImagePull is the pod state of image pull failed
	ErrImagePull = "ErrImagePull"
)

var log = logf.Log.WithName("daisy-member-manager").WithName("util")

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
			OwnerReferences: []metav1.OwnerReference{k8s.GetOwnerRef(di)},
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
			OwnerReferences: []metav1.OwnerReference{k8s.GetOwnerRef(di)},
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
			OwnerReferences: []metav1.OwnerReference{k8s.GetOwnerRef(di)},
		},
		Data: m.cfgGen.CreateConfigsHost(ctx, di, r),
	}
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

	// Create Service name based on name pattern available
	return newNameMacroReplacerInstallation(di).Replace(pattern)
}

// DaisyConfigNetworksHostRegexpTemplate: di-{di}-[^.]+\\d+-\\d+\\.{namespace}.svc.cluster.local$"
func CreatePodRegexp(di *v1.DaisyInstallation, template string) string {
	return newNameMacroReplacerInstallation(di).Replace(template)
}

func CreatePodFQDNsOfInstallation(di *v1.DaisyInstallation) []string {
	fqdns := make([]string, 0)
	di.LoopAllReplicas(func(r *v1.Replica) error {
		fqdns = append(fqdns, CreatePodFQDN(di.Namespace, r))
		return nil
	})

	return fqdns
}

func CreatePodFQDN(namespace string, r *v1.Replica) string {
	return fmt.Sprintf("%s.%s", CreatePodHostname(r), namespace)
}

func CreatePodFQDNsOfCluster(clusterName string, di *v1.DaisyInstallation) []string {
	fqdns := make([]string, 0)
	var cluster v1.Cluster
	var ok bool

	if cluster, ok = di.Spec.Configuration.Clusters[clusterName]; !ok {
		return fqdns
	}
	fn := func(name string, shard *v1.Shard, di *v1.DaisyInstallation, cluster *v1.Cluster) error {
		for _, r := range shard.Replicas {
			fqdns = append(fqdns, CreatePodHostname(&r))
		}
		return nil
	}
	cluster.LoopShards(di, fn)

	return fqdns
}

func CreatePodFQDNsOfShard(clusterName string, shardName string, di *v1.DaisyInstallation) []string {
	fqdns := make([]string, 0)
	var shard v1.Shard
	var ok bool

	if shard, ok = di.Spec.Configuration.Clusters[clusterName].Layout.Shards[shardName]; !ok {
		return fqdns
	}

	for _, r := range shard.Replicas {
		fqdns = append(fqdns, CreatePodHostname(&r))
	}

	return fqdns
}

func GetNormalPodFQDNsOfShard(clusterName string, shardName string, di *v1.DaisyInstallation) []string {
	fqdns := make([]string, 0)
	var shard v1.Shard
	var ok bool

	if shard, ok = di.Spec.Configuration.Clusters[clusterName].Layout.Shards[shardName]; !ok {
		return fqdns
	}

	for _, r := range shard.Replicas {
		if r.IsNormal(clusterName, shardName, di) {
			fqdns = append(fqdns, CreatePodHostname(&r))
		}
	}

	return fqdns
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

// CreateReplicaName return a name of a replica
func CreateReplicaName(shardName string, index int, skip sets.Int) string {
	return fmt.Sprintf("%s-%d", shardName, getNextIndex(index, skip))
}

func labelReplica(ctx *memberContext, replica *v1.Replica) label.Label {
	return label.New().Instance(ctx.Installation).Cluster(ctx.CurCluster).ShardName(ctx.CurShard).ReplicaName(replica.Name)
}

func labelInstallation(ctx *memberContext) label.Label {
	return label.New().Instance(ctx.Installation)
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

// Retry
func Retry(tries int, desc string, f func() error) error {
	var err error
	for try := 1; try <= tries; try++ {
		err = f()
		if err == nil {
			// All ok, no need to retry more
			if try > 1 {
				// Done, but after some retries, this is not 'clean'
				log.V(1).Info(fmt.Sprintf("DONE attempt %d of %d: %s", try, tries, desc))
			}
			return nil
		}

		if try < tries {
			// Try failed, need to sleep and retry
			seconds := try * 5
			log.V(1).Info(fmt.Sprintf("FAILED attempt %d of %d, sleep %d sec and retry: %s", try, tries, seconds, desc))
			select {
			case <-time.After(time.Duration(seconds) * time.Second):
			}
		} else if tries == 1 {
			// On single try do not put so much emotion. It just failed and user is not intended to retry
			log.V(1).Info(fmt.Sprintf("FAILED single try. No retries will be made for %s", desc))
		} else {
			// On last try no need to wait more
			log.V(1).Info(fmt.Sprintf("FAILED AND ABORT. All %d attempts: %s", tries, desc))
		}
	}

	return err
}

func fillStatus(ctx *memberContext, di *v1.DaisyInstallation) {
	di.Status.Version = version.Version
	di.Status.ClustersCount = ctx.Normalized.ClustersCount()
	di.Status.ShardsCount = ctx.Normalized.ShardsCount()
	di.Status.ReplicasCount = ctx.Normalized.ReplicasCount()
	di.Status.UpdatedReplicasCount = 0
	di.Status.DeletedReplicasCount = 0
	di.Status.AddedReplicasCount = 0
	di.Status.ReadyReplicas = 0
}

func (m *DaisyMemberManager) GetActualStatefulSet(ctx *memberContext, di *v1.DaisyInstallation) (k8s.StatefulSetList, error) {
	stss := apps.StatefulSetList{}
	ns := client.InNamespace(ctx.Namespace)
	matchLabels := client.MatchingLabels(labelInstallation(ctx).Labels())
	err := m.deps.Client.List(context.Background(), &stss, ns, matchLabels)
	sort.Slice(stss.Items, func(i, j int) bool {
		return stss.Items[i].Name < stss.Items[j].Name
	})
	return stss.Items, err
}

// UpdateInstallationStatus allows to safely ignore update status error
// while deleting daisy installation
func UpdateInstallationStatus(cli client.Client, di *v1.DaisyInstallation, tolerateAbsence bool) error {
	cur := v1.DaisyInstallation{}
	key := client.ObjectKey{
		Namespace: di.Namespace,
		Name:      di.Name,
	}
	if err := cli.Get(context.Background(), key, &cur); err != nil {
		if tolerateAbsence {
			return nil
		}
		log.V(1).Error(err, "Update Status fail, give up to update status")
		return err
	}

	// Update status of a real object
	//cur.Status = di.Status
	return cli.Status().Update(context.Background(), di)
}

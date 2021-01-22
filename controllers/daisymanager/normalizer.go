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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/util"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
	//"strconv"
)

// Normalizer
type Normalizer struct {
	di         *v1.DaisyInstallation
	normalized NormalizedSpec
	cfgMgr     *ConfigManager
	// Whether should insert default cluster if no cluster specified
	withDefaultCluster bool
}

// NewNormalizer
func NewNormalizer(mgr *ConfigManager) *Normalizer {
	return &Normalizer{
		cfgMgr: mgr,
	}
}

// GetInstallationFromTemplate produces ready-to-use DaisyInstallation object
func (n *Normalizer) GetInstallationFromTemplate(di *v1.DaisyInstallation, withDefaultCluster bool) (*NormalizedSpec, error) {
	// Whether should insert default cluster if no cluster specified
	n.withDefaultCluster = withDefaultCluster

	// What base should be used to create CHI
	if n.cfgMgr.Config().CHITemplate == nil {
		// No template specified - start with clear page
		n.di = new(v1.DaisyInstallation)
	} else {
		// Template specified - start with template
		n.di = n.cfgMgr.Config().CHITemplate.DeepCopy()
	}

	// At this moment n.chi is either empty CHI or a system-wide template
	// We need to apply templates

	var useTemplates []v1.UseTemplate
	if len(di.Spec.UseTemplates) > 0 {
		useTemplates = make([]v1.UseTemplate, len(di.Spec.UseTemplates))
		copy(useTemplates, di.Spec.UseTemplates)

		// UseTemplates must contain reasonable data, thus has to be normalized
		//n.normalizeUseTemplates(&useTemplates)
	}

	for i := range useTemplates {
		useTemplate := &useTemplates[i]
		var template *v1.DaisyInstallation = nil
		if template = n.cfgMgr.Config().FindTemplate(useTemplate, di.Namespace); template == nil {
			// Try to find in k8s
			if template = n.cfgMgr.FetchTemplate(useTemplate, di.Namespace); template == nil {
				log.V(1).Info("UNABLE to find template referenced in useTemplates. Skip it.", "Namespace", di.Namespace, "Template", useTemplate.Name)
			}
		}
		if template != nil {
			(&n.di.Spec).MergeFrom(&template.Spec, v1.MergeTypeOverrideByNonEmptyValues)
			log.V(2).Info("Merge template %s/%s referenced in useTemplates", "Namespace", di.Namespace, "Template", useTemplate.Name)
		}
	}

	// After all templates applied, place provided CHI on top of the whole stack
	n.di.MergeFrom(di, v1.MergeTypeOverrideByNonEmptyValues)
	n.normalized = NormalizedSpec{
		Spec:                 *n.di.Spec.DeepCopy(),
		VolumeClaimTemplates: make(map[string]*v1.VolumeClaimTemplate),
		PodTemplates:         make(map[string]*v1.DaisyPodTemplate),
	}

	return n.normalizeInstallation(n.di)
}

func (n *Normalizer) normalizeInstallation(di *v1.DaisyInstallation) (*NormalizedSpec, error) {
	// Walk over Spec datatype fields
	// TODO: get delete-slots from ObjectMeta
	//deleteSlots := helper.GetDeleteSlots(chi)

	//n.normalizeStop(&n.chi.Spec.Stop)
	n.normalizeConfiguration(di, &n.di.Spec.Configuration)
	n.normalizeTemplates(&di.Spec.Templates)

	//n.finalizeCHI()

	n.normalized.Spec = *n.di.Spec.DeepCopy()
	return &n.normalized, nil
}

// normalizeConfiguration normalizes .spec.configuration
func (n *Normalizer) normalizeConfiguration(di *v1.DaisyInstallation, conf *v1.Configuration) {
	n.normalizeConfigurationZookeeper(&conf.Zookeeper)

	n.normalizeConfigurationUsers(&conf.Users)
	n.normalizeConfigurationProfiles(&conf.Profiles)
	n.normalizeConfigurationQuotas(&conf.Quotas)
	n.normalizeConfigurationSettings(&conf.Settings)
	n.normalizeConfigurationFiles(&conf.Files)

	// Configuration.Clusters
	n.normalizeClusters(di)
}

// normalizeClusters normalizes clusters
func (n *Normalizer) normalizeClusters(di *v1.DaisyInstallation) {
	// We need to have at least one cluster available
	n.ensureCluster(di)

	// Normalize all clusters in this CHI
	n.di.EnumerateClusters(func(di *v1.DaisyInstallation, cluster *v1.Cluster) error {
		return n.normalizeCluster(di, cluster)
	})
}

// ensureCluster
func (n *Normalizer) ensureCluster(di *v1.DaisyInstallation) {
	// Introduce default cluster in case it is required
	if n.di.Spec.Configuration.Clusters == nil {
		n.di.Spec.Configuration.Clusters = make(map[string]v1.Cluster)
	}

	if len(n.di.Spec.Configuration.Clusters) == 0 {
		n.di.Spec.Configuration.Clusters["cluster"] = v1.Cluster{
			Name: fmt.Sprintf("cluster"),
		}
	}
}

// calcFingerprints calculates fingerprints for ClickHouse configuration data
//func (n *Normalizer) calcFingerprints(replica *v1.Replica) error {
//	replica.Config.ZookeeperFingerprint = util.Fingerprint(*replica.GetZookeeper())
//	replica.Config.SettingsFingerprint = util.Fingerprint(
//		fmt.Sprintf("%s%s",
//			util.Fingerprint(n.di.Spec.Configuration.Settings.AsSortedSliceOfStrings()),
//			util.Fingerprint(replica.Settings.AsSortedSliceOfStrings()),
//		),
//	)
//	replica.Config.FilesFingerprint = util.Fingerprint(
//		fmt.Sprintf("%s%s",
//			util.Fingerprint(
//				n.di.Spec.Configuration.Files.Filter(
//					nil,
//					[]v1.SettingsSection{v1.SectionUsers},
//					true,
//				).AsSortedSliceOfStrings(),
//			),
//			util.Fingerprint(
//				replica.Files.Filter(
//					nil,
//					[]v1.SettingsSection{v1.SectionUsers},
//					true,
//				).AsSortedSliceOfStrings(),
//			),
//		),
//	)
//
//	return nil
//}

// normalizeConfigurationZookeeper normalizes .spec.configuration.zookeeper
func (n *Normalizer) normalizeConfigurationZookeeper(zk *v1.ZookeeperConfig) {
	// In case no ZK port specified - assign default
	for i := range zk.Nodes {
		// Convenience wrapper
		node := &zk.Nodes[i]
		if node.Port == 0 {
			node.Port = zkDefaultPort
		}
	}

	// In case no ZK root specified - assign '/clickhouse/{namespace}/{chi name}'
	//if zk.Root == "" {
	//	zk.Root = fmt.Sprintf(zkDefaultRootTemplate, n.chi.Namespace, n.chi.Name)
	//}
}

// normalizeConfigurationUsers normalizes .spec.configuration.users
func (n *Normalizer) normalizeConfigurationUsers(users *v1.Settings) {

	if users == nil {
		// Do not know what to do in this case
		return
	}

	if *users == nil {
		*users = v1.NewSettings()
	}

	(*users).Normalize()

	// Extract username from path
	usernameMap := make(map[string]bool)
	for path := range *users {
		// Split 'admin/password'
		tags := strings.Split(path, "/")

		// Basic sanity check - need to have at least "username/something" pair
		if len(tags) < 2 {
			// Skip incorrect entry
			continue
		}

		username := tags[0]
		usernameMap[username] = true
	}

	// Ensure "must have" sections are in place, which are
	// 1. user/profile
	// 2. user/quota
	// 3. user/networks/ip and user/networks/host_regexp defaults to the installation pods
	// 4. user/password_sha256_hex

	usernameMap["default"] = true // we need default user here in order to secure host_regexp
	for username := range usernameMap {
		if _, ok := (*users)[username+"/profile"]; !ok {
			// No 'user/profile' section
			(*users)[username+"/profile"] = v1.NewScalarSetting(n.cfgMgr.Config().CHConfigUserDefaultProfile)
		}
		if _, ok := (*users)[username+"/quota"]; !ok {
			// No 'user/quota' section
			(*users)[username+"/quota"] = v1.NewScalarSetting(n.cfgMgr.Config().CHConfigUserDefaultQuota)
		}
		if _, ok := (*users)[username+"/networks/ip"]; !ok {
			// No 'user/networks/ip' section
			(*users)[username+"/networks/ip"] = v1.NewVectorSetting(n.cfgMgr.Config().CHConfigUserDefaultNetworksIP)
		}
		if _, ok := (*users)[username+"/networks/host_regexp"]; !ok {
			(*users)[username+"/networks/host_regexp"] = v1.NewScalarSetting(CreatePodRegexp(n.di, PodRegexpTemplate))
		}

		var pass = ""
		_pass, okPassword := (*users)[username+"/password"]
		if okPassword {
			pass = fmt.Sprintf("%v", _pass)
		} else if username != "default" {
			pass = n.cfgMgr.Config().CHConfigUserDefaultPassword
		}

		_, okPasswordSHA256 := (*users)[username+"/password_sha256_hex"]
		// if SHA256 is not set, initialize it from the password
		if pass != "" && !okPasswordSHA256 {
			pass_sha256 := sha256.Sum256([]byte(pass))
			(*users)[username+"/password_sha256_hex"] = v1.NewScalarSetting(hex.EncodeToString(pass_sha256[:]))
			okPasswordSHA256 = true
		}

		if okPasswordSHA256 {
			// ClickHouse does not start if both password and sha256 are defined
			if username == "default" {
				// Set remove password flag for default user that is empty in stock ClickHouse users.xml
				(*users)[username+"/password"] = v1.NewScalarSetting("_removed_")
			} else {
				delete(*users, username+"/password")
			}
		}
	}
}

// normalizeConfigurationProfiles normalizes .spec.configuration.profiles
func (n *Normalizer) normalizeConfigurationProfiles(profiles *v1.Settings) {

	if profiles == nil {
		// Do not know what to do in this case
		return
	}

	if *profiles == nil {
		*profiles = v1.NewSettings()
	}
	(*profiles).Normalize()
}

// normalizeConfigurationQuotas normalizes .spec.configuration.quotas
func (n *Normalizer) normalizeConfigurationQuotas(quotas *v1.Settings) {

	if quotas == nil {
		// Do not know what to do in this case
		return
	}

	if *quotas == nil {
		*quotas = v1.NewSettings()
	}

	(*quotas).Normalize()
}

// normalizeConfigurationSettings normalizes .spec.configuration.settings
func (n *Normalizer) normalizeConfigurationSettings(settings *v1.Settings) {

	if settings == nil {
		// Do not know what to do in this case
		return
	}

	if *settings == nil {
		*settings = v1.NewSettings()
	}

	(*settings).Normalize()
}

// normalizeConfigurationFiles normalizes .spec.configuration.files
func (n *Normalizer) normalizeConfigurationFiles(files *v1.Settings) {

	if files == nil {
		// Do not know what to do in this case
		return
	}

	if *files == nil {
		*files = v1.NewSettings()
	}

	(*files).Normalize()
}

// normalizeCluster normalizes cluster and returns deployments usage counters for this cluster
func (n *Normalizer) normalizeCluster(di *v1.DaisyInstallation, cluster *v1.Cluster) error {
	//cluster.FillShardReplicaSpecified()

	// Inherit from .spec.configuration.zookeeper
	cluster.InheritZookeeperFrom(n.di)
	// Inherit from .spec.configuration.files
	cluster.InheritFilesFrom(n.di)
	//// Inherit from .spec.defaults
	//cluster.InheritTemplatesFrom(n.chi)

	n.normalizeConfigurationZookeeper(&cluster.Zookeeper)
	n.normalizeConfigurationSettings(&cluster.Settings)
	n.normalizeConfigurationFiles(&cluster.Files)

	n.normalizeClusterLayoutShardsCountAndReplicasCount(&cluster.Layout)

	n.ensureClusterLayoutShards(di, cluster)
	//n.ensureClusterLayoutReplicas(&cluster.Layout)

	// Loop over all shards and replicas inside shards and fill structure
	cluster.EnumerateShards(di, func(name string, shard *v1.Shard, di *v1.DaisyInstallation, cluster *v1.Cluster) error {
		n.normalizeShard(shard, di, cluster)
		return nil
	})

	// TODO:for now it does not allow to delete shard as operator
	// does not migrate data for the shard to delete to other
	// shards. In future if data migration has been supported, delete
	// extra shard should be supported as well

	return nil
}

// normalizeClusterLayoutShardsCountAndReplicasCount ensures at least 1 shard and 1 replica counters
func (n *Normalizer) normalizeClusterLayoutShardsCountAndReplicasCount(layout *v1.Layout) {
	// Layout.ShardsCount and
	// Layout.ReplicasCount must represent max number of shards and replicas requested respectively

	// Deal with ShardsCount
	if layout.ShardsCount == 0 {
		// No ShardsCount specified - need to figure out

		// We need to have at least one Shard
		layout.ShardsCount = 1

		// Let's look for explicitly specified Shards in Layout.Shards
		if len(layout.Shards) > layout.ShardsCount {
			// We have some Shards specified explicitly, do not allow to delete shards for now
			layout.ShardsCount = len(layout.Shards)
		}

		// Let's look for explicitly specified Shards in Layout.Replicas
		//for i := range layout.Replicas {
		//	replica := &layout.Replicas[i]
		//
		//	if replica.ShardsCount > layout.ShardsCount {
		//		// We have Shards number specified explicitly in this replica
		//		layout.ShardsCount = replica.ShardsCount
		//	}
		//
		//	if len(replica.Hosts) > layout.ShardsCount {
		//		// We have some Shards specified explicitly
		//		layout.ShardsCount = len(replica.Hosts)
		//	}
		//}
	}

	// Deal with ReplicasCount
	if layout.ReplicasCount == 0 {
		// No ReplicasCount specified - need to figure out

		// We need to have at least one Replica
		layout.ReplicasCount = 1

		// Let's look for explicitly specified Replicas in Layout.Shards
		//for i := range layout.Shards {
		//	shard := &layout.Shards[i]
		//
		//	if shard.ReplicasCount > layout.ReplicasCount - len(slots[i]) {
		//		// We have Replicas number specified explicitly in this shard
		//		layout.ReplicasCount = shard.ReplicasCount - len(slots[i])
		//	}
		//
		//	if len(shard.Hosts) > layout.ReplicasCount - len(slots[i]) {
		//		// We have some Replicas specified explicitly
		//		layout.ReplicasCount = len(shard.Hosts) - len(slots[i])
		//	}
		//}
		//
		//// Let's look for explicitly specified Replicas in Layout.Replicas
		//// TODO: ignore slots when explicitly specified replicas
		//if len(layout.Replicas) > layout.ReplicasCount {
		//	// We have some Replicas specified explicitly
		//	layout.ReplicasCount = len(layout.Replicas)
		//}
	}
}

// ensureClusterLayoutShards ensures slice layout.Shards is in place
func (n *Normalizer) ensureClusterLayoutShards(di *v1.DaisyInstallation, cluster *v1.Cluster) {
	// Disposition of shards in slice would be
	// [explicitly specified shards 0..N, N+1..layout.ShardsCount-1 empty slots for to-be-filled shards]

	if cluster.Layout.Shards == nil {
		cluster.Layout.Shards = map[string]v1.Shard{}
	}

	// Some (may be all) shards specified, need to append space for unspecified shards
	for len(cluster.Layout.Shards) < cluster.Layout.ShardsCount {
		//TODO if regarding slots, the whole shard has been deleted, then there should not add new shard
		name := n.createShardName(di.Name, cluster.Name, len(cluster.Layout.Shards))
		cluster.Layout.Shards[name] = v1.Shard{
			Name: name,
		}
	}

	// TODO: if moving shard data has been supported in future, it should delete extra shards
}

func (n *Normalizer) createShardName(diName string, clusterName string, index int) string {
	return fmt.Sprintf("%s-%s-%d", diName, clusterName, index)
}

// ensureClusterLayoutReplicas ensures slice layout.Replicas is in place
//func (n *Normalizer) ensureClusterLayoutReplicas(layout *chiv1.ChiClusterLayout) {
//	// Disposition of replicas in slice would be
//	// [explicitly specified replicas 0..N, N+1..layout.ReplicasCount-1 empty slots for to-be-filled replicas]
//
//	// Some (may be all) replicas specified, need to append space for unspecified replicas
//	// TODO may be there is better way to append N slots to a slice
//	for len(layout.Replicas) < layout.ReplicasCount {
//		layout.Replicas = append(layout.Replicas, chiv1.ChiReplica{})
//	}
//}

// normalizeShard normalizes a shard - walks over all fields
func (n *Normalizer) normalizeShard(shard *v1.Shard, di *v1.DaisyInstallation, cluster *v1.Cluster) {
	//n.normalizeShardWeight(shard)
	// For each shard of this normalized cluster inherit from cluster
	shard.InheritSettingsFrom(cluster)
	n.normalizeConfigurationSettings(&shard.Settings)
	shard.InheritFilesFrom(cluster)
	n.normalizeConfigurationSettings(&shard.Files)
	//shard.InheritTemplatesFrom(cluster)
	// Normalize Replicas
	n.normalizeShardReplicasCount(shard, cluster.Layout.ReplicasCount, nil)
	n.normalizeReplicas(shard, di, cluster, -1)
	// Internal replication uses ReplicasCount thus it has to be normalized after shard ReplicaCount normalized
	n.normalizeShardInternalReplication(shard)
}

// normalizeShardReplicasCount ensures shard.ReplicasCount filled properly
func (n *Normalizer) normalizeShardReplicasCount(shard *v1.Shard, layoutReplicasCount int, set sets.Int) {
	//if shard.ReplicaCount > 0 {
	//	// Shard has explicitly specified number of replicas
	//	return
	//}

	// Here we have shard.ReplicasCount = 0, meaning that
	// shard does not have explicitly specified number of replicas - need to fill it

	// Look for explicitly specified Replicas first
	//if len(shard.Hosts) > 0 {
	//	// We have Replicas specified as a slice and no other replicas count provided,
	//	// this means we have explicitly specified replicas only and exact ReplicasCount is known
	//	shard.ReplicasCount = len(shard.Hosts)
	//	return
	//}

	// No shard.ReplicasCount specified, no replicas explicitly provided, so we have to
	// use ReplicasCount from layout
	shard.ReplicaCount = layoutReplicasCount

	// if set is not empty, it should exclude the replicas to be deleted
	if len(set) > 0 {
		shard.ReplicaCount = shard.ReplicaCount - len(set)
	}
}

// normalizeShardHosts normalizes all replicas of specified shard
func (n *Normalizer) normalizeReplicas(shard *v1.Shard, di *v1.DaisyInstallation, cluster *v1.Cluster, shardIndex int) {
	// Use hosts from HostsField
	//if shard.Replicas != nil && len(shard.Replicas)>0 {
	//	return
	//}

	if shard.Replicas == nil {
		shard.Replicas = map[string]v1.Replica{}
	}

	newReplicas := sets.NewString()
	for i := 0; i < shard.ReplicaCount; i++ {
		newReplicas.Insert(CreateReplicaName(shard.Name, i, nil))
	}

	for len(shard.Replicas) < shard.ReplicaCount {
		// We still have some assumed hosts in this shard - let's add it as replicaIndex
		replicaIndex := len(shard.Replicas)
		// Check whether we have this host in HostsField
		replica := v1.Replica{
			Name: CreateReplicaName(shard.Name, replicaIndex, nil),
		}
		//shard.Replicas[replica.Name] = replica
		n.normalizeReplica(&replica, shard, cluster, replicaIndex, nil)
		shard.Replicas[replica.Name] = replica
	}

	// Delete extra replica
	if len(shard.Replicas) > shard.ReplicaCount {
		for r := range shard.Replicas {
			if !newReplicas.Has(r) {
				delete(shard.Replicas, r)
			}
		}
	}
}

// normalizeShardInternalReplication ensures reasonable values in
// .spec.configuration.clusters.layout.shards.internalReplication
func (n *Normalizer) normalizeShardInternalReplication(shard *v1.Shard) {
	// Shards with replicas are expected to have internal replication on by default
	defaultInternalReplication := false
	if shard.ReplicaCount > 1 {
		defaultInternalReplication = true
	}
	shard.InternalReplication = util.CastStringBoolToStringTrueFalse(shard.InternalReplication, defaultInternalReplication)
}

// normalizeReplica normalizes a replica - walks over all fields
func (n *Normalizer) normalizeReplica(replica *v1.Replica, shard *v1.Shard, cluster *v1.Cluster, replicaIndex int, skip sets.Int) {
	// For each replica of this normalized cluster inherit from cluster
	replica.InheritSettingsFrom(shard)
	n.normalizeConfigurationSettings(&replica.Settings)
	replica.InheritFilesFrom(shard)
	n.normalizeConfigurationSettings(&replica.Files)
	(&replica.Templates).MergeFrom(&n.normalized.Spec.Defaults.Templates, v1.MergeTypeFillEmptyValues)
}

// normalizeTemplates normalizes .spec.templates
func (n *Normalizer) normalizeTemplates(templates *v1.Templates) {
	//for i := range templates.HostTemplates {
	//	hostTemplate := &templates.HostTemplates[i]
	//	n.normalizeHostTemplate(hostTemplate)
	//}

	for i := range templates.PodTemplates {
		podTemplate := &templates.PodTemplates[i]
		n.normalizePodTemplate(podTemplate)
	}

	for i := range templates.VolumeClaimTemplates {
		vcTemplate := &templates.VolumeClaimTemplates[i]
		n.normalizeVolumeClaimTemplate(vcTemplate)
	}

	//for i := range templates.ServiceTemplates {
	//	serviceTemplate := &templates.ServiceTemplates[i]
	//	n.normalizeServiceTemplate(serviceTemplate)
	//}
}

// normalizePodTemplate normalizes .spec.templates.podTemplates
func (n *Normalizer) normalizePodTemplate(template *v1.DaisyPodTemplate) {
	// Name

	//// Zone
	//if len(template.Zone.Values) == 0 {
	//	// In case no values specified - no key is reasonable
	//	template.Zone.Key = ""
	//} else if template.Zone.Key == "" {
	//	// We have values specified, but no key
	//	// Use default zone key in this case
	//	template.Zone.Key = "failure-domain.beta.kubernetes.io/zone"
	//} else {
	//	// We have both key and value(s) specified explicitly
	//}
	//
	//// Distribution
	//if template.Distribution == chiv1.PodDistributionOnePerHost {
	//	// Known distribution, all is fine
	//} else {
	//	// Default Pod Distribution
	//	template.Distribution = chiv1.PodDistributionUnspecified
	//}

	//// PodDistribution
	//for i := range template.PodDistribution {
	//	podDistribution := &template.PodDistribution[i]
	//	switch podDistribution.Type {
	//	case
	//		chiv1.PodDistributionUnspecified,
	//
	//		// AntiAffinity section
	//		chiv1.PodDistributionClickHouseAntiAffinity,
	//		chiv1.PodDistributionShardAntiAffinity,
	//		chiv1.PodDistributionReplicaAntiAffinity:
	//		if podDistribution.Scope == "" {
	//			podDistribution.Scope = chiv1.PodDistributionScopeCluster
	//		}
	//	case
	//		chiv1.PodDistributionAnotherNamespaceAntiAffinity,
	//		chiv1.PodDistributionAnotherClickHouseInstallationAntiAffinity,
	//		chiv1.PodDistributionAnotherClusterAntiAffinity:
	//		// PodDistribution is known
	//	case
	//		chiv1.PodDistributionMaxNumberPerNode:
	//		// PodDistribution is known
	//		if podDistribution.Number < 0 {
	//			podDistribution.Number = 0
	//		}
	//	case
	//		// Affinity section
	//		chiv1.PodDistributionNamespaceAffinity,
	//		chiv1.PodDistributionClickHouseInstallationAffinity,
	//		chiv1.PodDistributionClusterAffinity,
	//		chiv1.PodDistributionShardAffinity,
	//		chiv1.PodDistributionReplicaAffinity,
	//		chiv1.PodDistributionPreviousTailAffinity:
	//		// PodDistribution is known
	//
	//	case chiv1.PodDistributionCircularReplication:
	//		// Shortcut section
	//		// All shortcuts have to be expanded
	//
	//		// PodDistribution is known
	//
	//		if podDistribution.Scope == "" {
	//			podDistribution.Scope = chiv1.PodDistributionScopeCluster
	//		}
	//
	//		// TODO need to support multi-cluster
	//		cluster := &n.chi.Spec.Configuration.Clusters[0]
	//
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type:  chiv1.PodDistributionShardAntiAffinity,
	//			Scope: podDistribution.Scope,
	//		})
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type:  chiv1.PodDistributionReplicaAntiAffinity,
	//			Scope: podDistribution.Scope,
	//		})
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type:   chiv1.PodDistributionMaxNumberPerNode,
	//			Scope:  podDistribution.Scope,
	//			Number: cluster.Layout.ReplicasCount,
	//		})
	//
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type: chiv1.PodDistributionPreviousTailAffinity,
	//		})
	//
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type: chiv1.PodDistributionNamespaceAffinity,
	//		})
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type: chiv1.PodDistributionClickHouseInstallationAffinity,
	//		})
	//		template.PodDistribution = append(template.PodDistribution, chiv1.ChiPodDistribution{
	//			Type: chiv1.PodDistributionClusterAffinity,
	//		})
	//
	//	default:
	//		// PodDistribution is not known
	//		podDistribution.Type = chiv1.PodDistributionUnspecified
	//	}
	//}

	//// Spec
	//template.Spec.Affinity = n.mergeAffinity(template.Spec.Affinity, n.newAffinity(template))
	//
	//// In case we have hostNetwork specified, we need to have ClusterFirstWithHostNet DNS policy, because of
	//// https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy
	//// which tells:  For Pods running with hostNetwork, you should explicitly set its DNS policy “ClusterFirstWithHostNet”.
	//if template.Spec.HostNetwork {
	//	template.Spec.DNSPolicy = v1.DNSClusterFirstWithHostNet
	//}

	// Introduce PodTemplate into Index
	// Ensure map is in place
	if n.normalized.PodTemplates == nil {
		n.normalized.PodTemplates = make(map[string]*v1.DaisyPodTemplate)
	}

	n.normalized.PodTemplates[template.Name] = template
}

// normalizeVolumeClaimTemplate normalizes .spec.templates.volumeClaimTemplates
func (n *Normalizer) normalizeVolumeClaimTemplate(template *v1.VolumeClaimTemplate) {
	// Check name
	// Check PVCReclaimPolicy
	//if !template.PVCReclaimPolicy.IsValid() {
	//	template.PVCReclaimPolicy = v1.PVCReclaimPolicyDelete
	//}
	// Check Spec

	// Ensure map is in place
	if n.normalized.VolumeClaimTemplates == nil {
		n.normalized.VolumeClaimTemplates = make(map[string]*v1.VolumeClaimTemplate)
	}
	n.normalized.VolumeClaimTemplates[template.Name] = template
}

func getNextIndex(index int, skip sets.Int) int {
	if len(skip) == 0 {
		return index
	}
	tmp := skip.List()

	index += 1
	g := tmp[0]
	c := tmp[0] + 1
	if g >= index {
		return index - 1
	}

	for _, v := range tmp {
		if v > c {
			if g+v-c >= index {
				return c + index - g - 1
			}
			g += v - c
		}
		c = v + 1
	}

	return c + index - g - 1
}

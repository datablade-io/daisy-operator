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

package v1

import (
	"github.com/daisy/daisy-operator/pkg/label"
	"github.com/daisy/daisy-operator/pkg/version"
)

func (di *DaisyInstallation) GetInstanceName() string {
	labels := di.ObjectMeta.GetLabels()
	// Keep backward compatibility for helm.
	// This introduce a hidden danger that change this label will trigger rolling-update of most of the components
	// TODO: disallow mutation of this label or adding this label with value other than the cluster name in ValidateUpdate()
	if inst, ok := labels[label.InstanceLabelKey]; ok {
		return inst
	}
	return di.Name
}

// EnumerateClusters
func (di *DaisyInstallation) EnumerateClusters(
	f func(di *DaisyInstallation, cluster *Cluster) error,
) []error {
	res := make([]error, 0)
	for key, cluster := range di.Spec.Configuration.Clusters {
		if len(cluster.Name) < 1 {
			cluster.Name = key
		}
		res = append(res, f(di, &cluster))
		di.Spec.Configuration.Clusters[key] = cluster
	}

	return res
}

func (di *DaisyInstallation) LoopClusters(
	f func(name string, cluster *Cluster) error,
) []error {
	res := make([]error, 0)

	for name, cluster := range di.Spec.Configuration.Clusters {
		res = append(res, f(name, &cluster))
	}

	return res
}

// MergeFrom
func (di *DaisyInstallation) MergeFrom(from *DaisyInstallation, _type MergeType) {
	if from == nil {
		return
	}

	// Copy metadata for now
	di.TypeMeta = from.TypeMeta
	di.ObjectMeta = from.ObjectMeta

	// Do actual merge for Spec
	di.Spec = *from.Spec.DeepCopy()

	// Copy Status for now
	di.Status = from.Status
}

func (di *DaisyInstallation) IsAllClusterReady() bool {
	//TODO: add logic here
	for name := range di.Spec.Configuration.Clusters {
		if !di.Status.Clusters[name].Health {
			return false
		}
	}
	return true
}

func (di *DaisyInstallation) FillStatus() {
	di.Status.Version = version.Version
	di.Status.ClustersCount = di.ClustersCount()
	di.Status.ShardsCount = di.ShardsCount()
	di.Status.ReplicasCount = di.ReplicasCount()
	di.Status.UpdatedReplicasCount = 0
	di.Status.DeletedReplicasCount = 0
	di.Status.AddedReplicasCount = 0
	di.Status.ReadyReplicas = 0
}

func (di *DaisyInstallation) ClustersCount() int {
	return len(di.Spec.Configuration.Clusters)
}

func (di *DaisyInstallation) ShardsCount() int {
	count := 0
	di.LoopClusters(func(name string, cluster *Cluster) error {
		count += len(cluster.Layout.Shards)
		return nil
	})
	return count
}

func (di *DaisyInstallation) ReplicasCount() int {
	count := 0

	di.LoopClusters(func(name string, cluster *Cluster) error {
		cluster.LoopShards(di, func(
			name string, shard *Shard, di *DaisyInstallation, cluster *Cluster) error {
			count += len(shard.Replicas)
			return nil
		})
		return nil
	})
	return count
}

// LoopAllReplicas exec fn func for each replica
func (di *DaisyInstallation) LoopAllReplicas(fn func(r *Replica) error) {
	di.LoopClusters(func(name string, cluster *Cluster) error {
		cluster.LoopShards(di, func(
			name string, shard *Shard, di *DaisyInstallation, cluster *Cluster) error {
			for _, replica := range shard.Replicas {
				fn(&replica)
			}
			return nil
		})
		return nil
	})
}

const (
	StatusInProgress  = "InProgress"
	StatusCompleted   = "Completed"
	StatusTerminating = "Terminating"
)

func (s *DaisyInstallationStatus) Reset() {
	s.State = StatusInProgress
	s.UpdatedReplicasCount = 0
	s.AddedReplicasCount = 0
	s.DeletedReplicasCount = 0
}

func (s *DaisyInstallationStatus) GetReplicaStatus(r *Replica) *ReplicaStatus {
	for _, clusterStatus := range s.Clusters {
		for _, shardStatus := range clusterStatus.Shards {
			for _, repliaStatus := range shardStatus.Replicas {
				if r.Name == repliaStatus.Name {
					return repliaStatus.DeepCopy()
				}
			}
		}
	}

	return nil
}

func (s *DaisyInstallationStatus) SetReplicaStatus(clusterName, shardName, replicaName string, status ReplicaStatus) {
	var ok bool
	if _, ok = s.Clusters[clusterName]; ok {
		if _, ok = s.Clusters[clusterName].Shards[shardName]; ok {
			shard := s.Clusters[clusterName].Shards[shardName]
			if shard.Replicas == nil {
				shard.Replicas = make(map[string]ReplicaStatus)
			}
			shard.Replicas[replicaName] = status
			s.Clusters[clusterName].Shards[shardName] = shard
		}
	}
}

func (cStatus *ClusterStatus) IsAllShardReady() bool {
	for _, shardStatus := range cStatus.Shards {
		if !shardStatus.Health {
			return false
		}
	}
	return true
}

func (s *ShardStatus) IsAllReplicaReady() bool {
	for _, replica := range s.Replicas {
		if replica.Phase != NormalPhase {
			return false
		}
	}
	return true
}

func (c *Cluster) EnumerateShards(di *DaisyInstallation,
	f func(name string, shard *Shard, di *DaisyInstallation, cluster *Cluster) error,
) []error {
	res := make([]error, 0)

	for name, shard := range c.Layout.Shards {
		res = append(res, f(name, &shard, di, c))
		c.Layout.Shards[name] = shard
	}

	return res
}

func (c *Cluster) LoopShards(di *DaisyInstallation,
	f func(name string, shard *Shard, di *DaisyInstallation, cluster *Cluster) error,
) []error {
	res := make([]error, 0)

	for name, shard := range c.Layout.Shards {
		res = append(res, f(name, &shard, di, c))
	}

	return res
}

func (cluster *Cluster) InheritZookeeperFrom(di *DaisyInstallation) {
	if cluster.Zookeeper.IsEmpty() {
		(&cluster.Zookeeper).MergeFrom(&di.Spec.Configuration.Zookeeper, MergeTypeFillEmptyValues)
	}
}

func (cluster *Cluster) InheritSettingsFrom(di *DaisyInstallation) {
	(&cluster.Settings).MergeFrom(di.Spec.Configuration.Settings)
}

func (cluster *Cluster) InheritFilesFrom(di *DaisyInstallation) {
	(&cluster.Files).MergeFromCB(di.Spec.Configuration.Files, func(path string, _ *Setting) bool {
		if section, err := getSectionFromPath(path); err == nil {
			if section == SectionHost {
				return true
			}
		}

		return false
	})
}

func (zkc *ZookeeperConfig) IsEmpty() bool {
	return len(zkc.Nodes) == 0
}

func (zkc *ZookeeperConfig) MergeFrom(from *ZookeeperConfig, _type MergeType) {
	if from == nil {
		return
	}

	if !from.IsEmpty() {
		// Append Nodes from `from`
		if zkc.Nodes == nil {
			zkc.Nodes = make([]ZookeeperNode, 0)
		}
		for fromIndex := range from.Nodes {
			fromNode := &from.Nodes[fromIndex]

			// Try to find equal entry
			equalFound := false
			for toIndex := range zkc.Nodes {
				toNode := &zkc.Nodes[toIndex]
				if toNode.Equal(fromNode) {
					// Received already have such a node
					equalFound = true
					break
				}
			}

			if !equalFound {
				// Append Node from `from`
				zkc.Nodes = append(zkc.Nodes, *fromNode.DeepCopy())
			}
		}
	}

	if from.SessionTimeoutMs > 0 {
		zkc.SessionTimeoutMs = from.SessionTimeoutMs
	}
	if from.OperationTimeoutMs > 0 {
		zkc.OperationTimeoutMs = from.OperationTimeoutMs
	}
	if from.Root != "" {
		zkc.Root = from.Root
	}
	if from.Identity != "" {
		zkc.Identity = from.Identity
	}
}

func (zkNode *ZookeeperNode) Equal(to *ZookeeperNode) bool {
	if to == nil {
		return false
	}

	return (zkNode.Host == to.Host) && (zkNode.Port == to.Port)
}

func (shard *Shard) InheritSettingsFrom(cluster *Cluster) {
	(&shard.Settings).MergeFrom(cluster.Settings)
}

func (shard *Shard) InheritFilesFrom(cluster *Cluster) {
	(&shard.Files).MergeFrom(cluster.Files)
}

func (replica *Replica) InheritSettingsFrom(shard *Shard) {
	(&replica.Settings).MergeFrom(shard.Settings)
}

func (replica *Replica) InheritFilesFrom(shard *Shard) {
	(&replica.Files).MergeFrom(shard.Files)
}

func (replica *Replica) IsReady(clusterName string, shardName string, di *DaisyInstallation) bool {
	var status ReplicaStatus
	var ok bool

	//status := di.Status.GetReplicaStatus(replica)

	if status, ok = di.Status.Clusters[clusterName].Shards[shardName].Replicas[replica.Name]; !ok {
		return false
	}

	if status.Phase == ReadyPhase {
		return true
	}
	return false
}

func (replica *Replica) IsSync(clusterName string, shardName string, di *DaisyInstallation) bool {
	var status ReplicaStatus
	var ok bool

	//status := di.Status.GetReplicaStatus(replica)

	if status, ok = di.Status.Clusters[clusterName].Shards[shardName].Replicas[replica.Name]; !ok {
		return false
	}

	if status.State == Sync {
		return true
	}
	return false
}

func (replica *Replica) CanDeleteAllPVCs() bool {
	//TODO: add logic when support PVC
	return true
}

func (replica *Replica) IsNormal(clusterName string, shardName string, di *DaisyInstallation) bool {
	var status ReplicaStatus
	var ok bool

	//status := di.Status.GetReplicaStatus(replica)

	if status, ok = di.Status.Clusters[clusterName].Shards[shardName].Replicas[replica.Name]; !ok {
		return false
	}

	if status.Phase == NormalPhase {
		return true
	}
	return false
}

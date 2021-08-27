package daisymanager

import (
	"github.com/daisy/daisy-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NormalizedSpec struct {
	Spec                 v1.DaisyInstallationSpec
	VolumeClaimTemplates map[string]*v1.VolumeClaimTemplate
	PodTemplates         map[string]*v1.DaisyPodTemplate
}

type memberContext struct {
	Namespace    string
	Installation string
	CurShard     string
	CurReplica   string
	CurCluster   string
	Clusters     map[string]v1.Cluster
	Normalized   NormalizedSpec
	owner        metav1.OwnerReference
	Config       *v1.DaisyOperatorConfigurationSpec
}

func (ctx *memberContext) GetVolumeClaimTemplate(name string) (*v1.VolumeClaimTemplate, bool) {
	if ctx.Normalized.VolumeClaimTemplates == nil {
		return nil, false
	} else {
		template, ok := ctx.Normalized.VolumeClaimTemplates[name]
		return template, ok
	}
}

func (ctx *memberContext) GetDaisyPodTemplate(name string) (*v1.DaisyPodTemplate, bool) {
	if ctx.Normalized.PodTemplates == nil {
		return nil, false
	} else {
		template, ok := ctx.Normalized.PodTemplates[name]
		return template, ok
	}
}

func (n *NormalizedSpec) ClustersCount() int {
	return len(n.Spec.Configuration.Clusters)
}

func (n *NormalizedSpec) ShardsCount() int {
	count := 0
	n.LoopClusters(func(name string, cluster *v1.Cluster) error {
		count += len(cluster.Layout.Shards)
		return nil
	})
	return count
}

func (n *NormalizedSpec) ReplicasCount() int {
	count := 0

	n.LoopClusters(func(name string, cluster *v1.Cluster) error {
		for _, shard := range cluster.Layout.Shards {
			count += len(shard.Replicas)
		}

		return nil
	})
	return count
}

func (n *NormalizedSpec) LoopClusters(
	f func(name string, cluster *v1.Cluster) error,
) []error {
	res := make([]error, 0)

	for name, cluster := range n.Spec.Configuration.Clusters {
		res = append(res, f(name, &cluster))
	}

	return res
}

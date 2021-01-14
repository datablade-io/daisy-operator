package daisymanager

import (
	"context"
	"fmt"
	"github.com/daisy/daisy-operator/pkg/k8s"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/label"
)

func getKey(obj metav1.Object) client.ObjectKey {
	return client.ObjectKey{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func fillLayout(clusterName string, di *v1.DaisyInstallation) {
	layout := di.Spec.Configuration.Clusters[clusterName].Layout
	shards := make(map[string]v1.Shard)

	for i := 0; i < layout.ShardsCount; i++ {
		shardName := fmt.Sprintf("%s-%s-%d", di.Name, clusterName, i)
		shard := v1.Shard{
			Name:     shardName,
			Replicas: make(map[string]v1.Replica),
		}
		for j := 0; j < layout.ReplicasCount; j++ {
			replicaName := CreateReplicaName(shardName, j, nil)
			shard.Replicas[replicaName] = v1.Replica{
				Name: replicaName,
			}
		}
		shards[shardName] = shard
	}
	layout.Shards = shards
	cluster := di.Spec.Configuration.Clusters[clusterName]
	cluster.Layout = layout
	di.Spec.Configuration.Clusters[clusterName] = cluster
}

func scaleInByShard(di *v1.DaisyInstallation, clusterName string, num int) {
	cluster := di.Spec.Configuration.Clusters[clusterName]
	cluster.Layout.ShardsCount -= num
	di.Spec.Configuration.Clusters[clusterName] = cluster
}

func scaleByReplica(di *v1.DaisyInstallation, clusterName string, num int) {
	cluster := di.Spec.Configuration.Clusters[clusterName]
	cluster.Layout.ReplicasCount += num
	di.Spec.Configuration.Clusters[clusterName] = cluster
}

type checker struct {
	installation *v1.DaisyInstallation
	ctx          memberContext
	g            *GomegaWithT
}

func newChecker(di *v1.DaisyInstallation, g *GomegaWithT) *checker {
	return &checker{
		installation: di,
		ctx: memberContext{
			Namespace:    di.Namespace,
			Installation: di.Name,
		},
		g: g,
	}
}

func (c *checker) verifyReplicas(sets []apps.StatefulSet) {
	for _, set := range sets {
		c.checkStatefulSet(&set)
	}
}

func (c *checker) checkStatefulSet(set *apps.StatefulSet) {
	c.checkLabels(set.Labels)
	c.checkStatefulSetSpec(set)
}

func (c *checker) checkServiceCount(cli client.Client, expect int) {
	svcs := corev1.ServiceList{}
	c.g.Expect(cli.List(context.Background(), &svcs, client.InNamespace(c.ctx.Namespace))).
		Should(Succeed())
	c.g.Expect(len(svcs.Items)).To(Equal(expect))
}

func (c *checker) checkConfigMapCount(cli client.Client, expect int) {
	cms := corev1.ConfigMapList{}
	c.g.Expect(cli.List(context.Background(), &cms, client.InNamespace(c.ctx.Namespace))).
		Should(Succeed())
	c.g.Expect(len(cms.Items)).To(Equal(expect))
}

func (c *checker) checkStatefulSetCount(cli client.Client, expect int) {
	sets := apps.StatefulSetList{}
	c.g.Expect(cli.List(context.Background(), &sets, client.InNamespace(c.ctx.Namespace))).
		Should(Succeed())
	c.g.Expect(len(sets.Items)).To(Equal(expect))
}

func (c *checker) checkStatefulSetSpec(set *apps.StatefulSet) {
	spec := set.Spec
	c.g.Expect(*spec.Replicas).To(Equal(int32(1)))
	c.g.Expect(*spec.RevisionHistoryLimit).To(Equal(int32(10)))
	c.g.Expect(spec.Selector.MatchLabels).To(Equal(set.Labels))
	c.g.Expect(spec.ServiceName).To(Equal(set.Name))
	c.g.Expect(spec.Template.Labels).To(Equal(set.Labels))
}

func (c *checker) checkLabels(l label.Label) {
	c.g.Expect(l[label.NameLabelKey]).To(Equal(label.DaisyApp))
	c.g.Expect(l[label.InstanceLabelKey]).To(Equal(c.ctx.Installation))
	c.g.Expect(l[label.ManagedByLabelKey]).To(Equal(label.DaisyOperator))
	//c.g.Expect(l[label.ComponentLabelKey]).To(Equal(label.ReplicaLabelVal))
}

func (c *checker) checkOwnerRef(refs []metav1.OwnerReference) {
	for _, ref := range refs {
		if ref.Name == c.ctx.Installation {
			return
		}
	}
	c.g.Expect(refs).To(Equal(nil))
}

func updateAllStsToReady(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
	sets := apps.StatefulSetList{}
	g.Expect(cli.List(context.Background(), &sets, client.InNamespace(di.Namespace))).
		Should(Succeed())

	// update sts and Sync status
	for _, set := range sets.Items {
		set.Status.ReadyReplicas = *set.Spec.Replicas
		set.Status.CurrentReplicas = *set.Spec.Replicas
		set.Status.UpdatedReplicas = *set.Spec.Replicas
		set.Status.ObservedGeneration = set.ObjectMeta.Generation
		g.Expect(cli.Status().Update(context.Background(), &set)).Should(Succeed())
	}
}

func prepareResourceForInstallation(m *DaisyMemberManager, di *v1.DaisyInstallation, toComplete bool, g *GomegaWithT) {
	err := k8s.RequeueErrorf("")
	for err != nil && k8s.IsRequeueError(err) {
		err = m.Sync(nil, di)
	}

	if toComplete {
		updateAllStsToReady(m.deps.Client, di, g)
		g.Expect(m.Sync(di, di)).Should(Succeed())
		g.Expect(di.Status.State).To(Equal(v1.StatusCompleted))
	}
}

func runSync(m *DaisyMemberManager, old, cur *v1.DaisyInstallation, notOnce bool) {
	if !notOnce {
		m.Sync(old, cur)
	} else {
		err := k8s.RequeueErrorf("")
		for err != nil && k8s.IsRequeueError(err) {
			err = m.Sync(old, cur)
			old = cur
		}
	}
}

package daisymanager

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/examples/client-go/pkg/client/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/daisy"
	"github.com/daisy/daisy-operator/pkg/k8s"
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

func (c *checker) verifyReplicas(cli client.Client) {
	sets := apps.StatefulSetList{}
	err := cli.List(context.Background(), &sets, client.InNamespace("default"))
	c.g.Expect(err).Should(BeNil())
	for _, set := range sets.Items {
		c.checkStatefulSet(&set)
	}
}

func (c *checker) verifyAnyReplica(cli client.Client, fn func(set *apps.StatefulSet, g *GomegaWithT)) {
	sets := apps.StatefulSetList{}
	err := cli.List(context.Background(), &sets, client.InNamespace("default"))
	c.g.Expect(err).Should(BeNil())
	c.g.Expect(len(sets.Items) > 0).Should(BeTrue())
	fn(&sets.Items[0], c.g)
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
	c.g.ExpectWithOffset(1, cli.List(context.Background(), &cms, client.InNamespace(c.ctx.Namespace))).
		Should(Succeed())
	c.g.ExpectWithOffset(1, len(cms.Items)).To(Equal(expect))
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
		err = m.Sync(di)
	}

	if toComplete {
		updateAllStsToReady(m.deps.Client, di, g)
		g.Expect(m.Sync(di)).Should(Succeed())
		g.Expect(di.Status.State).To(Equal(v1.StatusCompleted))
	}
}

func runSync(m *DaisyMemberManager, cur *v1.DaisyInstallation, notOnce bool) {
	if !notOnce {
		m.Sync(cur)
	} else {
		err := k8s.RequeueErrorf("")
		for err != nil && k8s.IsRequeueError(err) {
			err = m.Sync(cur)
		}
	}
}

func newFakeDaisyMemberManager(initObjs ...runtime.Object) (dmm *DaisyMemberManager, fakeClient client.Client) {
	// Register operator types with the runtime scheme.

	_ = clientgoscheme.AddToScheme(scheme.Scheme)

	if err := v1.AddToScheme(scheme.Scheme); err != nil {
		fmt.Printf("error: %v", err)
	}

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClientWithScheme(scheme.Scheme, initObjs...)

	fakeDeps := &Dependencies{
		Client:   cl,
		Log:      ctrl.Log.WithName("daisy_member_manager_test"),
		Recorder: record.NewFakeRecorder(100),
	}

	var cfgPath = "../../config/manager/config/config.yaml"
	if path, err := os.Getwd(); err == nil {
		cfgPath = fmt.Sprintf("%s/%s", path, cfgPath)
	}
	//fmt.Printf("path is %s", cfgPath)

	cfgMgr, err := NewConfigManager(fakeDeps.Client, cfgPath)
	if err != nil {
		return nil, nil
	}
	schemer, _ := NewFakeSchemer(
		cfgMgr.Config().CHUsername,
		cfgMgr.Config().CHPassword,
		cfgMgr.Config().CHPort,
		fakeDeps.Log.WithName("schemer"))
	dmm = &DaisyMemberManager{
		deps:       fakeDeps,
		normalizer: NewNormalizer(cfgMgr),
		cfgMgr:     cfgMgr,
		cfgGen:     nil,
		schemer:    schemer,
	}

	return dmm, fakeDeps.Client
}

func NewFakeSchemer(username, password string, port int, logger logr.Logger) (*Schemer, *daisy.FakeConnection) {
	fakeConnection := daisy.NewFakeConnection()
	return &Schemer{
		Username: username,
		Password: password,
		Port:     port,
		Log:      logger.WithName("schemer"),
		Pool: &daisy.FakeConnectionPool{
			Conn: fakeConnection,
		},
	}, fakeConnection
}

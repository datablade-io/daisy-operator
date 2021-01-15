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
	"k8s.io/apimachinery/pkg/runtime"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/daisy/daisy-operator/api/v1"
)

func TestDaisyMemberManager_normalize(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	type args struct {
		di *v1.DaisyInstallation
	}
	tests := []struct {
		name string
		args args
		want *v1.DaisyInstallation
	}{
		{
			name: "2 shards, 2 replicas cluster",
			args: args{
				di: newTestInstallation(2, 2),
			},
			want: nil,
		},
		{
			name: "simple settings",
			args: args{
				di: addSettings(newTestInstallation(1, 1)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newFakeDaisyMemberManager()
			got := m.normalize(tt.args.di)
			if got == nil {
				t.Errorf("normalize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDaisyMemberManager_Sync(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	type args struct {
		di *v1.DaisyInstallation
	}
	tests := []struct {
		name string
		args args
		want *v1.DaisyInstallation
	}{
		{
			name: "2 shards, 2 replicas cluster",
			args: args{
				di: addSettings(newTestInstallation(2, 2)),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			di := tt.args.di
			c := newChecker(di, g)
			m, cli := newFakeDaisyMemberManager()
			prepareResourceForInstallation(m, di, false, g)

			//verify sts
			sets := apps.StatefulSetList{}
			di.GetName()
			err := cli.List(context.Background(), &sets, client.InNamespace("default"))
			g.Expect(err).Should(BeNil())
			c.verifyReplicas(sets.Items)
			g.Expect(len(sets.Items)).Should(Equal(4), "Replicas of cluster should be 4 ")
			g.Expect(di.Status.ReadyReplicas).To(Equal(0))
			g.Expect(di.Status.State).To(Equal(v1.StatusInProgress))
			//verify status
			g.Expect(di.Status.ClustersCount).To(Equal(1))
			g.Expect(di.Status.ReplicasCount).To(Equal(4))
			g.Expect(di.Status.ShardsCount).To(Equal(2))

			//verify services
			c.checkServiceCount(cli, 5)

			//verify configmaps
			c.checkConfigMapCount(cli, 6)

			// scale down
			cur := di.DeepCopy()
			scaleInByShard(cur, "cluster", 1)
			err = m.Sync(di, cur)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(cur.Status.ShardsCount).To(Equal(2))
		})
	}
}

func TestDaisyMemberManager_Delete(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name     string
		initObjs []runtime.Object
		init     func(*DaisyMemberManager, *v1.DaisyInstallation, bool, *GomegaWithT)
		update   func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT)
		di       *v1.DaisyInstallation
		verify   func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT)
	}{
		{
			name: "delete 1 shard 1 replica",
			initObjs: []runtime.Object{
				newTestInstallation(1, 1),
			},
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.checkConfigMapCount(m.deps.Client, 3)
			},
			di: newTestInstallation(1, 1),
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.checkConfigMapCount(cli, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager(tt.initObjs...)
			old := v1.DaisyInstallation{}
			di := tt.di

			tt.init(m, di, false, g)
			tt.update(m, di, g)
			err := cli.Get(context.Background(), getKey(di), &old)
			g.Expect(err).ShouldNot(HaveOccurred())

			err = m.deleteDaisyInstallation(di)
			g.Expect(err).ShouldNot(HaveOccurred())

			tt.verify(cli, di, g)
		})
	}
}

func TestDaisyMemberManager_StatusUpdate(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name         string
		initObjs     []runtime.Object
		init         func(*DaisyMemberManager, *v1.DaisyInstallation, bool, *GomegaWithT)
		update       func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT)
		old          *v1.DaisyInstallation
		di           *v1.DaisyInstallation
		notSyncOnce  bool
		expectStatus *v1.DaisyInstallationStatus
		verify       func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT)
	}{
		{
			name: "update status to Completed, when sts become ready",
			initObjs: []runtime.Object{
				newTestInstallation(2, 2),
			},
			old:  newTestInstallation(2, 2),
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
			},
			di: newTestInstallation(2, 2),
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				g.Expect(di.Status.ReadyReplicas).To(Equal(4))
				g.Expect(di.Status.State).To(Equal(v1.StatusCompleted))
			},
		},
		{
			name: "update status to InProgess, when sts become unavailable",
			initObjs: []runtime.Object{
				newTestInstallation(2, 2),
			},
			old:  newTestInstallation(2, 2),
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				sets := apps.StatefulSetList{}
				cli := m.deps.Client
				g.Expect(cli.List(context.Background(), &sets, client.InNamespace(di.Namespace))).
					Should(Succeed())
				sets.Items[0].Status.ReadyReplicas = int32(0)
				g.Expect(cli.Status().Update(context.Background(), &sets.Items[0])).Should(Succeed())
			},
			di: newTestInstallation(2, 2),
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				g.Expect(di.Status.ReadyReplicas).To(Equal(3))
				g.Expect(di.Status.State).To(Equal(v1.StatusInProgress))
			},
		},
		{
			name: "scale out, status changed to NotSync, InProgress",
			initObjs: []runtime.Object{
				newTestInstallation(1, 1),
			},
			old:  newTestInstallation(1, 1),
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				scaleByReplica(di, "cluster", 1)
			},
			di:          newTestInstallation(2, 2),
			notSyncOnce: true,
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.checkConfigMapCount(cli, 6)
				c.checkStatefulSetCount(cli, 4)
				g.Expect(di.Status.ReplicasCount).To(Equal(4))
				g.Expect(di.Status.ReadyReplicas).To(Equal(1))
				g.Expect(di.Status.State).To(Equal(v1.StatusInProgress))
				g.Expect(di.Status.Clusters["cluster"].
					Shards["test-installation-cluster-0"].
					Replicas["test-installation-cluster-0-0"].State).To(Equal(v1.Sync))
				g.Expect(di.Status.Clusters["cluster"].
					Shards["test-installation-cluster-0"].
					Replicas["test-installation-cluster-0-1"].State).To(Equal(v1.NotSync))
			},
		},
		{
			name: "scale out, eventually replica's state change to Sync",
			initObjs: []runtime.Object{
				newTestInstallation(1, 1),
			},
			old:  newTestInstallation(1, 1),
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				scaleByReplica(di, "cluster", 1)
				runSync(m, di, di, true)
				updateAllStsToReady(m.deps.Client, di, g)
			},
			di:          newTestInstallation(1, 2),
			notSyncOnce: true,
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.checkConfigMapCount(cli, 4)
				c.checkStatefulSetCount(cli, 2)
				g.Expect(di.Status.ReplicasCount).To(Equal(2))
				g.Expect(di.Status.State).To(Equal(v1.StatusCompleted))
				g.Expect(di.Status.Clusters["cluster"].
					Shards["test-installation-cluster-0"].
					Replicas["test-installation-cluster-0-0"].State).To(Equal(v1.Sync))
				g.Expect(di.Status.Clusters["cluster"].
					Shards["test-installation-cluster-0"].
					Replicas["test-installation-cluster-0-1"].State).To(Equal(v1.Sync))
			},
		},
		{
			name: "scale in, status changed to InProgress",
			initObjs: []runtime.Object{
				newTestInstallation(2, 2),
			},
			old:  newTestInstallation(2, 2),
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.checkConfigMapCount(m.deps.Client, 6)
				scaleByReplica(di, "cluster", -1)
			},
			di: newTestInstallation(2, 1),
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.checkConfigMapCount(cli, 4)
				c.checkServiceCount(cli, 3)
				c.checkStatefulSetCount(cli, 2)
				g.Expect(di.Status.ReadyReplicas).To(Equal(2))
				g.Expect(di.Status.State).To(Equal(v1.StatusCompleted))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager(tt.initObjs...)
			old := tt.old
			cur := tt.di

			tt.init(m, old, true, g)
			tt.update(m, old, g)
			cur.Status = *old.Status.DeepCopy()
			cur.Status.PrevSpec = old.Spec
			runSync(m, old, cur, tt.notSyncOnce)
			tt.verify(cli, cur, g)
		})
	}
}

func TestDaisyMemberManager_syncServiceForInstallation(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name    string
		di      *v1.DaisyInstallation
		wantErr bool
	}{
		{
			name:    "new service for installation",
			di:      newTestInstallation(1, 1),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newChecker(tt.di, g)
			m, cli := newFakeDaisyMemberManager()
			if err := m.syncServiceForInstallation(nil, tt.di); (err != nil) != tt.wantErr {
				t.Errorf("syncServiceForInstallation() error = %v, wantErr %v", err, tt.wantErr)
			}
			svc := corev1.Service{}
			err := cli.Get(context.Background(), client.ObjectKey{Name: fmt.Sprintf("daisy-%s", tt.di.Name), Namespace: tt.di.Namespace}, &svc)
			g.Expect(err).Should(BeNil())
			c.checkOwnerRef(svc.OwnerReferences)
		})
	}
}

func TestDaisyMemberManager_syncConfigMaps(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name   string
		di     *v1.DaisyInstallation
		verify func(cli client.Client, di *v1.DaisyInstallation)
	}{
		{
			name: "default common, cond.d, and host config",
			di:   newTestInstallation(2, 2),
			verify: func(cli client.Client, di *v1.DaisyInstallation) {
				cms := corev1.ConfigMapList{}
				err := cli.List(context.Background(), &cms, client.InNamespace(di.Namespace))
				g.Expect(err).Should(BeNil())
				g.Expect(len(cms.Items)).Should(Equal(2))
			},
		},
		{
			name: "replica specific config: files",
			di:   addSettings(newTestInstallation(2, 2)),
			verify: func(cli client.Client, di *v1.DaisyInstallation) {
				cms := corev1.ConfigMapList{}
				err := cli.List(context.Background(), &cms, client.InNamespace(di.Namespace))
				g.Expect(err).Should(BeNil())
				g.Expect(len(cms.Items)).To(Equal(2))
				var commonCfg, userCfg corev1.ConfigMap
				for _, cfg := range cms.Items {
					if cfg.Name == "di-test-installation-common-configd" {
						commonCfg = cfg
					} else {
						userCfg = cfg
					}
				}
				g.Expect(len(commonCfg.Data)).Should(Equal(4))
				g.Expect(len(userCfg.Data)).Should(Equal(3))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager()
			di := m.normalize(tt.di)
			m.cfgGen = NewConfigSectionsGenerator(NewConfigGenerator(di), m.cfgMgr.Config())
			err := m.syncConfigMaps(di, false)
			g.Expect(err).To(BeNil())
			tt.verify(cli, di)
		})
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

	cfgMgr, err := NewConfigManager(fakeDeps.Client, "")
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

func newTestInstallation(shardNum int, replicaNum int) *v1.DaisyInstallation {
	return &v1.DaisyInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaisyCluster",
			APIVersion: "daisy.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-installation",
			Namespace: corev1.NamespaceDefault,
			UID:       types.UID("test"),
		},
		Spec: v1.DaisyInstallationSpec{
			Configuration: v1.Configuration{
				Clusters: map[string]v1.Cluster{
					"cluster": {
						Name: "cluster",
						Layout: v1.Layout{
							ShardsCount:   shardNum,
							ReplicasCount: replicaNum,
						},
					},
				},
			},
		},
	}
}

func addSettings(di *v1.DaisyInstallation) *v1.DaisyInstallation {
	di.Spec.Configuration.Users.MergeFrom(v1.Settings{
		"test/password": v1.NewScalarSetting("qwerty"),
		"test/networks/ip": v1.NewVectorSetting([]string{
			"127.0.0.1/32", "192.168.74.1/24",
		}),
		"test/profile": v1.NewScalarSetting("test_profile"),
		"test/quota":   v1.NewScalarSetting("test_quota"),
	})

	di.Spec.Configuration.Profiles.MergeFrom(v1.Settings{
		"test_profile/max_memory_usage": v1.NewScalarSetting("1000000000"),
		"test_profile/readonly":         v1.NewScalarSetting("1"),
		"readonly/readonly":             v1.NewScalarSetting("1"),
	})

	di.Spec.Configuration.Quotas.MergeFrom(v1.Settings{
		"test_quota/interval/duration": v1.NewScalarSetting("3600"),
	})

	di.Spec.Configuration.Settings.MergeFrom(v1.Settings{
		"compression/case/method":    v1.NewScalarSetting("zstd"),
		"disable_internal_dns_cache": v1.NewScalarSetting("1"),
	})

	di.Spec.Configuration.Files.MergeFrom(v1.Settings{
		"COMMON/dict1.xml": v1.NewScalarSetting(`|
        <yandex>
            <!-- ref to file /etc/clickhouse-data/config.d/source1.csv -->
        </yandex>`),
		"COMMON/source1.csv": v1.NewScalarSetting(`|
        a1,b1,c1,d1
        a2,b2,c2,d2`),
		"HOST/users_prefixed_1.file": v1.NewScalarSetting(`|
		<yandex>
		<!-- users_prefixed_1.file -->
		</yandex>`),
	})

	return di
}

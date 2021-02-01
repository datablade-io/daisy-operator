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
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/util"
)

func TestDaisyMemberManager_normalize(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	type args struct {
		di *v1.DaisyInstallation
	}
	tests := []struct {
		name   string
		args   args
		want   *v1.DaisyInstallation
		verify func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT)
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
		{
			name: "default templates",
			args: args{
				di: addDefaults(newTestInstallation(1, 1)),
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				r01 := di.Spec.Configuration.Clusters["cluster"].
					Layout.Shards["test-installation-cluster-0"].Replicas["test-installation-cluster-0-0"]
				g.Expect(r01.Templates.LogVolumeClaimTemplate).To(Equal("log-tp01"))
				g.Expect(r01.Templates.DataVolumeClaimTemplate).To(Equal("data-tp01"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager()
			got := m.normalize(tt.args.di)
			if got == nil {
				t.Errorf("normalize() = %v, want %v", got, tt.want)
			}
			if tt.verify != nil {
				di := &v1.DaisyInstallation{
					Spec: got.Spec,
				}
				tt.verify(cli, di, g)
			}
		})
	}
}

func TestDaisyMemberManager_UseTemplates(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	type expect struct {
		tplName v1.TemplateNames
		podTp   v1.DaisyPodTemplate
		dataTp  corev1.PersistentVolumeClaim
		logTp   corev1.PersistentVolumeClaim
	}
	pvc2G := newPVC("default-storage-template-2Gi", "2Gi", "")
	pvc2G.Spec.StorageClassName = nil
	defaultPVC := newPVC("default-volume-claim-template", "2Gi", "")
	defaultPVC.Spec.StorageClassName = nil
	tests := []struct {
		name     string
		initObjs []runtime.Object
		di       *v1.DaisyInstallation
		expect   expect
		verify   func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT)
	}{
		{
			name:     "no templates, choose useTemplates",
			initObjs: append(tpsToObj(newTestTemplates())),
			di: modifyDefaults(addUseTemplates(newTestInstallation(1, 1)),
				"pod1-tp1",
				"vol2-data-tp2",
				"vol1-log-tp1",
			),
			expect: expect{
				tplName: v1.TemplateNames{
					PodTemplate:             "pod1-tp1",
					DataVolumeClaimTemplate: "vol2-data-tp2",
					LogVolumeClaimTemplate:  "vol1-log-tp1",
				},
				podTp:  newPodTemplate("", "daisy1-v1"),
				dataTp: newPVC("vol2-data-tp2", "10Gi", "expand"),
				logTp:  newPVC("vol1-log-tp1", "100M", "standard"),
			},
		},
		{
			name:     "both templates and useTemplates specified, use templates",
			initObjs: append(tpsToObj(newTestTemplates())),
			di: modifyDefaults(addTemplates(addUseTemplates(newTestInstallation(1, 1))),
				"pod-tp01",
				"data-tp01",
				"log-tp01",
			),
			expect: expect{
				tplName: v1.TemplateNames{
					PodTemplate:             "pod-tp01",
					DataVolumeClaimTemplate: "data-tp01",
					LogVolumeClaimTemplate:  "log-tp01",
				},
				podTp:  newPodTemplate("pod-tp01", "daisy"),
				dataTp: newPVC("data-tp01", "5Gi", "standard"),
				logTp:  newPVC("log-tp01", "100M", "standard"),
			},
		},
		{
			name:     "ignore non-exist useTemplates",
			initObjs: append(tpsToObj(newTestTemplates())),
			di: modifyDefaults(
				withUseTemplate(addUseTemplates(newTestInstallation(1, 1)),
					v1.UseTemplate{Name: "tp3", Namespace: "ns1", UseType: "None"},
				),
				"pod1-tp1",
				"vol2-data-tp2",
				"vol1-log-tp1",
			),
			expect: expect{
				tplName: v1.TemplateNames{
					PodTemplate:             "pod1-tp1",
					DataVolumeClaimTemplate: "vol2-data-tp2",
					LogVolumeClaimTemplate:  "vol1-log-tp1",
				},
				podTp:  newPodTemplate("", "daisy1-v1"),
				dataTp: newPVC("vol2-data-tp2", "10Gi", "expand"),
				logTp:  newPVC("vol1-log-tp1", "100M", "standard"),
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				g.Expect(len(di.Status.PrevSpec.Templates.PodTemplates)).Should(Equal(4))
				g.Expect(len(di.Status.PrevSpec.Templates.VolumeClaimTemplates)).Should(Equal(8))
			},
		},
		{
			name:     "use file templates",
			initObjs: append(tpsToObj(newTestTemplates())),
			di: modifyDefaults(
				withUseTemplate(newTestInstallation(1, 1),
					v1.UseTemplate{Name: "01-default-volumeclaimtemplate", Namespace: "", UseType: "None"},
					v1.UseTemplate{Name: "default-storage-template-2Gi", Namespace: "", UseType: "None"},
				),
				"default-oneperhost-pod-template",
				"default-storage-template-2Gi",
				"default-volume-claim-template",
			),
			expect: expect{
				tplName: v1.TemplateNames{
					PodTemplate:             "default-oneperhost-pod-template",
					DataVolumeClaimTemplate: "default-storage-template-2Gi",
					LogVolumeClaimTemplate:  "default-volume-claim-template",
				},
				podTp: v1.DaisyPodTemplate{
					Name: "default-oneperhost-pod-template",
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "clickhouse",
								Image: "yandex/clickhouse-server:19.3.7",
							},
						},
					},
				},
				dataTp: pvc2G,
				logTp:  defaultPVC,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager(tt.initObjs...)
			di := tt.di
			runSync(m, di, true)

			c := newChecker(di, g)
			r01 := di.Status.PrevSpec.Configuration.Clusters["cluster"].
				Layout.Shards["test-installation-cluster-0"].Replicas["test-installation-cluster-0-0"]
			g.Expect(r01.Templates).To(BeEquivalentTo(tt.expect.tplName))
			c.verifyAnyReplica(cli, func(set *apps.StatefulSet, g *GomegaWithT) {
				tt.expect.dataTp.Labels = set.Labels
				tt.expect.logTp.Labels = set.Labels
				podSpec := set.Spec.Template.Spec
				g.ExpectWithOffset(2, podSpec.Containers[0].Image).To(Equal(tt.expect.podTp.Spec.Containers[0].Image))
				g.ExpectWithOffset(2, util.Diff(&set.Spec.VolumeClaimTemplates[0], &tt.expect.dataTp)).To(BeEmpty())
				g.ExpectWithOffset(2, util.Diff(&set.Spec.VolumeClaimTemplates[1], &tt.expect.logTp)).To(BeEmpty())
			})
			if tt.verify != nil {
				tt.verify(cli, di, g)
			}
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
				runSync(m, di, true)
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
			cur.ObjectMeta = old.ObjectMeta
			runSync(m, cur, tt.notSyncOnce)
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
			ctx := memberContext{
				Namespace:    tt.di.Namespace,
				Installation: tt.di.Name,
			}
			if err := m.syncServiceForInstallation(&ctx, tt.di); (err != nil) != tt.wantErr {
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
				g.Expect(len(commonCfg.Data)).Should(Equal(8))
				g.Expect(len(userCfg.Data)).Should(Equal(5))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager()
			di := m.normalize(tt.di)
			forCfg := *tt.di.DeepCopy()
			forCfg.Spec = di.Spec
			m.cfgGen = NewConfigSectionsGenerator(NewConfigGenerator(&forCfg), m.cfgMgr.Config())
			err := m.syncConfigMaps(&forCfg, false)
			g.Expect(err).To(BeNil())

			tt.verify(cli, tt.di)
		})
	}
}

func TestDaisyMemberManager_handleVolumeExpansion(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	//es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	di := newTestInstallation(1, 1)
	sampleStorageClass := storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{
		Name: "sample-sc"}}

	sampleClaim := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "sample-claim"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr(sampleStorageClass.Name),
			Resources: corev1.ResourceRequirements{Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			}}}}
	sts := apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sample-sset"},
		Spec: apps.StatefulSetSpec{
			Replicas:             pointer.Int32Ptr(3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{sampleClaim},
		},
	}
	resizedSset := *sts.DeepCopy()
	resizedSset.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("3Gi")
	pvcsWithSize := func(size ...string) []corev1.PersistentVolumeClaim {
		var pvcs []corev1.PersistentVolumeClaim
		for i, s := range size {
			pvcs = append(pvcs, withStorageReq(corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: fmt.Sprintf("sample-claim-sample-sset-%d", i)},
				Spec:       sampleClaim.Spec,
			}, s))
		}
		return pvcs
	}

	type args struct {
		di                   *v1.DaisyInstallation
		cur                  *apps.StatefulSet
		old                  *apps.StatefulSet
		validateStorageClass bool
	}
	tests := []struct {
		name         string
		args         args
		runtimeObjs  []runtime.Object
		expectedPVCs []corev1.PersistentVolumeClaim
		want         bool
		wantErr      bool
	}{
		{
			name: "no pvc to resize",
			args: args{
				di:                   di,
				cur:                  &sts,
				old:                  &sts,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),
		},
		{
			name: "all pvcs should be resized",
			args: args{
				di:                   di,
				cur:                  &resizedSset,
				old:                  &sts,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("1Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
			want:         true,
		},
		{
			name: "2 pvcs left to resize",
			args: args{
				di:                   di,
				cur:                  &resizedSset,
				old:                  &sts,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("3Gi", "1Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),
			want:         true,
		},
		{
			name: "one pvc is missing: resize what's there, don't error out",
			args: args{
				di:                   di,
				cur:                  &resizedSset,
				old:                  &sts,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("3Gi", "1Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
			want:         true,
		},
		{
			name: "storage decrease is not supported: error out",
			args: args{
				di:                   di,
				cur:                  &sts,         // 1Gi
				old:                  &resizedSset, // 3Gi
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("3Gi", "3Gi")), withVolumeExpansion(sampleStorageClass)),
			expectedPVCs: pvcsWithSize("3Gi", "3Gi"),
			wantErr:      true,
		},
		{
			name: "volume expansion not supported: error out",
			args: args{
				di:                   di,
				cur:                  &resizedSset,
				old:                  &sts,
				validateStorageClass: true,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("1Gi", "1Gi", "1Gi")), &sampleStorageClass), // no expansion
			expectedPVCs: pvcsWithSize("1Gi", "1Gi", "1Gi"),                                         // not resized
			want:         false,
			wantErr:      true,
		},
		{
			name: "volume expansion not supported but no storage class validation: attempt to resize",
			args: args{
				di:                   di,
				cur:                  &resizedSset,
				old:                  &sts,
				validateStorageClass: false,
			},
			runtimeObjs:  append(pvcsToObj(pvcsWithSize("1Gi", "1Gi", "1Gi")), &sampleStorageClass), // no expansion
			expectedPVCs: pvcsWithSize("3Gi", "3Gi", "3Gi"),                                         // still resized
			want:         true,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager(tt.runtimeObjs...)
			got, err := m.handleVolumeExpansion(tt.args.di, tt.args.cur, tt.args.old, tt.args.validateStorageClass)
			if (err != nil) != tt.wantErr {
				t.Errorf("handleVolumeExpansion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var pvcs corev1.PersistentVolumeClaimList
			g.Expect(cli.List(context.Background(), &pvcs)).Should(Succeed())
			g.Expect(len(pvcs.Items)).To(Equal(len(tt.expectedPVCs)))

			for i, expectedPVC := range tt.expectedPVCs {
				g.Expect(util.Diff(&pvcs.Items[i], &expectedPVC)).Should(BeEmpty())
			}

			if got != tt.want {
				t.Errorf("handleVolumeExpansion() got = %v, want %v", got, tt.want)
			}
		})
	}
}

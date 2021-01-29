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
	"k8s.io/apimachinery/pkg/runtime"
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/daisy/daisy-operator/api/v1"
)

func TestDaisyMemberManager_Sync(t *testing.T) {
	g := NewGomegaWithT(t)
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	type args struct {
		di *v1.DaisyInstallation
	}
	tests := []struct {
		name   string
		args   args
		verify func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT)
	}{
		{
			name: "2 shards, 2 replicas cluster",
			args: args{
				di: addSettings(newTestInstallation(2, 2)),
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)

				//verify sts
				c.checkStatefulSetCount(cli, 4)
				c.verifyReplicas(cli)

				//verify services
				c.checkServiceCount(cli, 5)

				//verify configmaps
				c.checkConfigMapCount(cli, 6)

				// verify status
				g.Expect(di.Status.ReadyReplicas).To(Equal(0))
				g.Expect(di.Status.State).To(Equal(v1.StatusInProgress))
				//verify status
				g.Expect(di.Status.ClustersCount).To(Equal(1))
				g.Expect(di.Status.ReplicasCount).To(Equal(4))
				g.Expect(di.Status.ShardsCount).To(Equal(2))
			},
		},
		{
			name: "invalid default template",
			args: args{
				di: addDefaults(newTestInstallation(1, 1)),
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.verifyAnyReplica(cli, func(set *apps.StatefulSet, g *GomegaWithT) {
					g.Expect(len(set.Spec.VolumeClaimTemplates)).Should(Equal(0))
					g.Expect(len(set.Spec.Template.Spec.Containers)).To(Equal(1), "Should not add log container")
					g.Expect(set.Spec.Template.Spec.Containers[0].VolumeMounts).Should(
						ContainElements(
							corev1.VolumeMount{
								Name:      "di-test-installation-common-configd",
								MountPath: "/etc/clickhouse-server/config.d/",
							}))
				})
			},
		},
		{
			name: "valid templates",
			args: args{
				di: addTemplates(addDefaults(newTestInstallation(1, 1))),
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.verifyAnyReplica(cli, func(set *apps.StatefulSet, g *GomegaWithT) {
					g.Expect(len(set.Spec.VolumeClaimTemplates)).Should(Equal(2))
					g.Expect(set.Spec.Template.Spec.Containers[0].Image).Should(Equal("daisy-image:latest"))
					g.Expect(set.Spec.Template.Spec.Containers[0].Name).Should(Equal("daisy"))
					g.Expect(len(set.Spec.Template.Spec.Containers)).To(Equal(2))
					g.Expect(set.Spec.Template.Spec.Containers[0].VolumeMounts).Should(
						ContainElements(
							corev1.VolumeMount{
								Name:      "data-tp01",
								MountPath: "/var/lib/clickhouse",
							}))
					g.Expect(set.Spec.Template.Spec.Containers[0].VolumeMounts).Should(
						ContainElements(
							corev1.VolumeMount{
								Name:      "log-tp01",
								MountPath: "/var/log/clickhouse-server",
							}))
				})
			},
		},
		{
			name: "multiple useTemplates",
			args: args{
				di: addTemplates(addDefaults(newTestInstallation(1, 1))),
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				c := newChecker(di, g)
				c.verifyAnyReplica(cli, func(set *apps.StatefulSet, g *GomegaWithT) {
					g.Expect(len(set.Spec.VolumeClaimTemplates)).Should(Equal(2))
					g.Expect(set.Spec.Template.Spec.Containers[0].Image).Should(Equal("daisy-image:latest"))
					g.Expect(set.Spec.Template.Spec.Containers[0].Name).Should(Equal("daisy"))
					g.Expect(len(set.Spec.Template.Spec.Containers)).To(Equal(2))
					g.Expect(set.Spec.Template.Spec.Containers[0].VolumeMounts).Should(
						ContainElements(
							corev1.VolumeMount{
								Name:      "data-tp01",
								MountPath: "/var/lib/clickhouse",
							}))
					g.Expect(set.Spec.Template.Spec.Containers[0].VolumeMounts).Should(
						ContainElements(
							corev1.VolumeMount{
								Name:      "log-tp01",
								MountPath: "/var/log/clickhouse-server",
							}))
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			di := tt.args.di
			m, cli := newFakeDaisyMemberManager()
			prepareResourceForInstallation(m, di, false, g)

			if tt.verify != nil {
				tt.verify(cli, di, g)
			}
		})
	}
}

func TestDaisyMemberManager_Update(t *testing.T) {
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
		update   func(*v1.DaisyInstallation) *v1.DaisyInstallation
		expect   expect
		verify   func(set *apps.StatefulSet, g *GomegaWithT)
	}{
		{
			name: "no statefulset update when add pvc",
			initObjs: []runtime.Object{
				modifyDefaults(addUseTemplates(newTestInstallation(1, 1)),
					"pod1-tp1",
					"vol2-data-tp2",
					"vol1-log-tp1",
				),
			},

			di: newTestInstallation(1, 1),
			update: func(di *v1.DaisyInstallation) *v1.DaisyInstallation {
				di.Spec.Defaults.Templates.DataVolumeClaimTemplate = "log-tp01"
				return addTemplates(di)
			},
			verify: func(set *apps.StatefulSet, g *GomegaWithT) {
				g.ExpectWithOffset(2, set.Spec.VolumeClaimTemplates).To(BeNil())
			},
		},
		{
			name: "no statefulset update when delete pvc",
			initObjs: append(
				tpsToObj(newTestTemplates()),
				modifyDefaults(addUseTemplates(newTestInstallation(1, 1)),
					"pod1-tp1",
					"vol2-data-tp2",
					"vol1-log-tp1",
				)),
			di: modifyDefaults(addUseTemplates(newTestInstallation(1, 1)),
				"pod1-tp1",
				"vol2-data-tp2",
				"vol1-log-tp1",
			),
			update: func(di *v1.DaisyInstallation) *v1.DaisyInstallation {
				di.Spec.Defaults = v1.Defaults{}
				return di
			},
			verify: func(set *apps.StatefulSet, g *GomegaWithT) {
				g.ExpectWithOffset(2, len(set.Spec.VolumeClaimTemplates)).To(Equal(2))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager(tt.initObjs...)
			di := tt.di

			prepareResourceForInstallation(m, di, true, g)
			tt.update(di)
			runSync(m, di, true)

			c := newChecker(di, g)
			if tt.verify != nil {
				c.verifyAnyReplica(cli, tt.verify)
			}
		})
	}
}

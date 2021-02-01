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
	"testing"

	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/label"
)

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

			err = m.deleteDaisyInstallation(&old)
			g.Expect(err).ShouldNot(HaveOccurred())

			tt.verify(cli, di, g)
		})
	}
}

func TestDaisyMemberManager_DeleteSlots(t *testing.T) {
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
			name: "delete 1 replicas by delete-slot",
			initObjs: []runtime.Object{
				newTestInstallation(2, 2),
			},
			old: newTestInstallation(2, 2),
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				withDeleteSlots(di, "test-installation-cluster-0-0")
			},
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				// verify status
				status := di.Status.Clusters["cluster"].Shards["test-installation-cluster-0"]
				g.ExpectWithOffset(2, len(status.Replicas)).To(Equal(1))
				g.ExpectWithOffset(2, di.Status.ReadyReplicas).To(Equal(3))
				g.ExpectWithOffset(2, di.Status.State).To(Equal(v1.StatusCompleted))

				// verify replicas
				sets := apps.StatefulSetList{}
				ns := client.InNamespace(di.Namespace)
				matchLabels1 := client.MatchingLabels(label.New().Instance(di.Name).
					Cluster("cluster").ShardName("test-installation-cluster-0").Labels())
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns, matchLabels1)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(1))
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(3))
			},
		},
		{
			name: "delete 2 replicas by delete-slot",
			initObjs: []runtime.Object{
				newTestInstallation(2, 3),
			},
			old: newTestInstallation(2, 3),
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				withDeleteSlots(di, "test-installation-cluster-0-0", "test-installation-cluster-0-2")
			},
			notSyncOnce: true,
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				// verify status
				status := di.Status.Clusters["cluster"].Shards["test-installation-cluster-0"]
				g.ExpectWithOffset(2, len(status.Replicas)).To(Equal(1))
				g.ExpectWithOffset(2, di.Status.ReadyReplicas).To(Equal(4))
				g.ExpectWithOffset(2, di.Status.State).To(Equal(v1.StatusCompleted))

				// verify replicas
				sets := apps.StatefulSetList{}
				ns := client.InNamespace(di.Namespace)
				matchLabels1 := client.MatchingLabels(label.New().Instance(di.Name).
					Cluster("cluster").ShardName("test-installation-cluster-0").Labels())
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns, matchLabels1)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(1))
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(4))
			},
		},
		{
			name: "ignore delete slots when replicaCount = len(delete-slots)",
			initObjs: []runtime.Object{
				newTestInstallation(2, 1),
			},
			old: newTestInstallation(2, 1),
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				withDeleteSlots(di, "test-installation-cluster-0-0")
			},
			notSyncOnce: true,
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				// verify status
				status := di.Status.Clusters["cluster"].Shards["test-installation-cluster-0"]
				g.ExpectWithOffset(2, len(status.Replicas)).To(Equal(1))
				g.ExpectWithOffset(2, di.Status.ReadyReplicas).To(Equal(2))
				g.ExpectWithOffset(2, di.Status.State).To(Equal(v1.StatusCompleted))

				// verify replicas
				sets := apps.StatefulSetList{}
				ns := client.InNamespace(di.Namespace)
				matchLabels1 := client.MatchingLabels(label.New().Instance(di.Name).
					Cluster("cluster").ShardName("test-installation-cluster-0").Labels())
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns, matchLabels1)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(1))
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(2))
			},
		},
		{
			name: "auto recreate replica when remove delete-slot annotation",
			initObjs: []runtime.Object{
				newTestInstallation(2, 2),
			},
			old:  newTestInstallation(2, 2),
			init: prepareResourceForInstallation,
			update: func(m *DaisyMemberManager, di *v1.DaisyInstallation, g *GomegaWithT) {
				withDeleteSlots(di, "test-installation-cluster-0-0")
				runSync(m, di, true)
				delete(di.Annotations, DeleteSlotsAnn)
			},
			notSyncOnce: true,
			verify: func(cli client.Client, di *v1.DaisyInstallation, g *GomegaWithT) {
				// verify status
				status := di.Status.Clusters["cluster"].Shards["test-installation-cluster-0"]
				g.ExpectWithOffset(2, len(status.Replicas)).To(Equal(2))
				g.ExpectWithOffset(2, di.Status.ReadyReplicas).To(Equal(3))
				g.ExpectWithOffset(2, di.Status.ReplicasCount).To(Equal(4))
				g.ExpectWithOffset(2, di.Status.State).To(Equal(v1.StatusInProgress))

				// verify replicas
				sets := apps.StatefulSetList{}
				ns := client.InNamespace(di.Namespace)
				matchLabels1 := client.MatchingLabels(label.New().Instance(di.Name).
					Cluster("cluster").ShardName("test-installation-cluster-0").Labels())
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns, matchLabels1)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(2))
				g.ExpectWithOffset(2, cli.List(context.Background(), &sets, ns)).Should(Succeed())
				g.ExpectWithOffset(2, len(sets.Items)).Should(Equal(4))
			},
		},
		//TODO: add test for "remove replica one by one",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cli := newFakeDaisyMemberManager(tt.initObjs...)
			old := tt.old

			prepareResourceForInstallation(m, old, true, g)
			tt.update(m, old, g)
			runSync(m, old, tt.notSyncOnce)
			tt.verify(cli, old, g)
		})
	}
}

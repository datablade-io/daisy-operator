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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/daisy/daisy-operator/api/v1"
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

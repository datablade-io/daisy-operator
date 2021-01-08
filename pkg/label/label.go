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

package label

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
)

const (
	LabelService = "daisy.com/Service"
)

const (
	// The following labels are recommended by kubernetes https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/

	// ManagedByLabelKey is Kubernetes recommended label key, it represents the tool being used to manage the operation of an application
	// For resources managed by TiDB Operator, its value is always tidb-operator
	ManagedByLabelKey = "app.kubernetes.io/managed-by"
	// ComponentLabelKey is Kubernetes recommended label key, it represents the component within the architecture
	ComponentLabelKey = "app.kubernetes.io/component"
	// NameLabelKey is Kubernetes recommended label key, it represents the name of the application
	NameLabelKey = "app.kubernetes.io/name"
	// InstanceLabelKey is Kubernetes recommended label key, it represents a unique name identifying the instance of an application
	// It's set by helm when installing a release
	InstanceLabelKey = "app.kubernetes.io/instance"

	// NamespaceLabelKey is label key used in PV for easy querying
	NamespaceLabelKey = "app.kubernetes.io/namespace"
	// UsedByLabelKey indicate where it is used. for example, daisy has two services,
	// one for internal component access and the other for end-user
	UsedByLabelKey = "app.kubernetes.io/used-by"
	// ClusterIDLabelKey is cluster id label key
	ClusterIDLabelKey = "daisy.com/cluster-id"
	// StoreIDLabelKey is store id label key
	ShardIDLabelKey = "daisy.com/store-id"
	// MemberIDLabelKey is member id label key
	ReplicaIDLabelKey = "daisy.com/replica-id"
	//
	ConfigMapLabelKey         = "daisy.com/configmap"
	ConfigMapValueCommon      = "Common"
	ConfigMapValueCommonUsers = "CommonUsers"
	ConfigMapValueHost        = "Host"

	// AnnPodNameKey is pod name annotation key used in PV/PVC for synchronizing tidb cluster meta info
	AnnPodNameKey string = "daisy.com/pod-name"
	// AnnPVCDeferDeleting is pvc defer deletion annotation key used in PVC for defer deleting PVC
	AnnPVCDeferDeleting = "daisy.com/pvc-defer-deleting"
	// AnnPVCPodScheduling is pod scheduling annotation key, it represents whether the pod is scheduling
	AnnPVCPodScheduling = "daisy.com/pod-scheduling"
	// AnnStsSyncTimestamp is sts annotation key to indicate the last timestamp the operator sync the sts
	AnnStsLastSyncTimestamp = "tidb.pingcap.com/sync-timestamp"

	// DaisyApp is Name label value indicates the application
	DaisyApp string = "daisy"
	// ShardLabelVal is Shard label value
	ShardLabelVal string = "shard"
	// ReplicaLabelVal is Replica label value
	ReplicaLabelVal string = "replica"
	// DaisyOperator is ManagedByLabelKey label value
	DaisyOperator string = "daisy-operator"
)

type Label map[string]string

// New initialize a new Label for components of tidb cluster
func New() Label {
	return Label{
		NameLabelKey:      DaisyApp,
		ManagedByLabelKey: DaisyOperator,
	}
}

// Namespace adds namespace kv pair to label
func (l Label) Namespace(name string) Label {
	l[NamespaceLabelKey] = name
	return l
}

// Instance adds instance kv pair to label
func (l Label) Instance(name string) Label {
	l[InstanceLabelKey] = name
	return l
}

func (l Label) Cluster(cluster string) Label {
	l[ClusterIDLabelKey] = cluster
	return l
}

func (l Label) ShardName(name string) Label {
	l[ShardIDLabelKey] = name
	return l
}

func (l Label) ReplicaName(name string) Label {
	l[ReplicaIDLabelKey] = name
	return l
}

func (l Label) Service(svcType string) Label {
	l[LabelService] = svcType
	return l
}

// Component adds component kv pair to label
func (l Label) Component(name string) Label {
	l[ComponentLabelKey] = name
	return l
}

// Shard assigns shard to component key in label
func (l Label) Shard() Label {
	l.Component(ShardLabelVal)
	return l
}

// Replica assigns replica to component key in label
func (l Label) Replica() Label {
	l.Component(ReplicaLabelVal)
	return l
}

func (l Label) AddLabel(key string, val string) Label {
	if len(key) > 0 {
		l[key] = val
	}
	return l
}

// LabelSelector gets LabelSelector from label
func (l Label) LabelSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: l}
}

// Labels converts label to map[string]string
func (l Label) Labels() map[string]string {
	return l
}

// Copy copy the value of label to avoid pointer copy
func (l Label) Copy() Label {
	copyLabel := make(Label)
	for k, v := range l {
		copyLabel[k] = v
	}
	return copyLabel
}

// String converts label to a string
func (l Label) String() string {
	var arr []string

	for k, v := range l {
		arr = append(arr, fmt.Sprintf("%s=%s", k, v))
	}

	return strings.Join(arr, ",")
}

// IsManagedByTiDBOperator returns whether label is a Managed by daisy-operator
func (l Label) IsManagedByDaisyOperator() bool {
	return l[ManagedByLabelKey] == DaisyOperator
}

func (l Label) IsDaisyInstallationPod() bool {
	return l[NameLabelKey] == DaisyApp
}

func (l Label) ConfigMapType(val string) Label {
	l.AddLabel(ConfigMapLabelKey, val)
	return l
}

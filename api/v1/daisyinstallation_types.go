/*
Copyright 2020.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MemberPhase is the current state of member
type MemberPhase string

const (
	// MemberPhase
	// NormalPhase represents normal state of replica.
	NormalPhase MemberPhase = "Normal"
	// UpgradePhase represents the upgrade state of replica.
	UpgradePhase MemberPhase = "Upgrade"
	// ScalePhase represents the scaling state of replica.
	ScalePhase MemberPhase = "Scale"
	// ReadyPhase represents statefulset is ready state
	ReadyPhase MemberPhase = "Ready"

	// State
	// NotSync represent replica not sync distributed views and tables
	// Sync represent replica has synced distributed views and tables
	NotSync string = "NotSync"
	Sync    string = "Sync"
)

// DaisyInstallationSpec defines the desired state of DaisyInstallation
type DaisyInstallationSpec struct {
	//for validate configuration
	// +kubebuilder:validation:Optional
	Validate Validate `json:"validate,omitempty"              yaml:"validate"`

	// Specify a Service Account
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Specify the type of cluster
	ClusterType string `json:"clusterType,omitempty"              yaml:"clusterType"`

	// Configuration include most settings of the installation
	Configuration Configuration `json:"configuration"`

	// Indicates that the daisy cluster is paused and will not be processed by
	// the controller.
	// +kubebuilder:validation:Optional
	Paused bool `json:"paused,omitempty"`

	// Persistent volume reclaim policy applied to the PVs that consumed by TiDB cluster
	// +kubebuilder:default=Retain
	PVReclaimPolicy corev1.PersistentVolumeReclaimPolicy `json:"pvReclaimPolicy,omitempty"`

	// ImagePullPolicy of TiDB cluster Pods
	// +kubebuilder:default=IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images.
	// +kubebuilder:validation:Optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Base annotations of TiDB cluster Pods, components may add or override selectors upon this respectively
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Base tolerations of Daisy cluster Pods, components may add more tolerations upon this respectively
	// +kubebuilder:validation:Optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Templates used by Daisy cluster pod, pvc
	// +kubebuilder:validation:Optional
	Templates Templates `json:"templates,omitempty"              yaml:"templates"`

	// Default settings for Templates, DistributedDDL and other settings
	// +kubebuilder:validation:Optional
	Defaults Defaults `json:"defaults,omitempty"               yaml:"defaults"`

	// UseTemplates, templates used by Installation
	// +kubebuilder:validation:Optional
	UseTemplates []UseTemplate `json:"useTemplates,omitempty"           yaml:"useTemplates"`
}

type Validate struct {
	// switch for close volume expand valid
	// +kubebuilder:validation:Optional
	NoValidateStorageClass bool `json:"noValidateStorageClass,omitempty"           yaml:"noValidateStorageClass"`
}

// UseTemplate defines UseTemplates section
type UseTemplate struct {
	Name      string `json:"name"      yaml:"name"`
	Namespace string `json:"namespace" yaml:"namespace"`
	UseType   string `json:"useType"   yaml:"useType"`
}

// Defaults defines defaults section of .spec
type Defaults struct {
	ReplicasUseFQDN string `json:"replicasUseFQDN,omitempty" yaml:"replicasUseFQDN"`
	//DistributedDDL  ChiDistributedDDL `json:"distributedDDL,omitempty"  yaml:"distributedDDL"`
	Templates TemplateNames `json:"templates,omitempty"       yaml:"templates"`
}

// Templates defines templates section of .spec
type Templates struct {
	// Templates
	PodTemplates         []DaisyPodTemplate    `json:"podTemplates,omitempty"         yaml:"podTemplates"`
	VolumeClaimTemplates []VolumeClaimTemplate `json:"volumeClaimTemplates,omitempty" yaml:"volumeClaimTemplates"`
}

// VolumeClaimTemplate defines PersistentVolumeClaim Template, directly used by StatefulSet
type VolumeClaimTemplate struct {
	Name string                           `json:"name"          yaml:"name"`
	Spec corev1.PersistentVolumeClaimSpec `json:"spec"          yaml:"spec"`
}

// Configuration defines configuration section of .spec
type Configuration struct {
	Zookeeper ZookeeperConfig `json:"zookeeper,omitempty" yaml:"zookeeper"`
	Kafka     KafkaConfig     `json:"kafka,omitempty" yaml:"kafka"`
	Users     Settings        `json:"users,omitempty"     yaml:"users"`
	Profiles  Settings        `json:"profiles,omitempty"  yaml:"profiles"`
	Quotas    Settings        `json:"quotas,omitempty"    yaml:"quotas"`
	Settings  Settings        `json:"settings,omitempty"  yaml:"settings"`
	Files     Settings        `json:"files,omitempty"     yaml:"files"`

	//TODO: replace with service template in future
	HTTPPort int32 `json:"httpPort,omitempty"     yaml:"httpPort"`
	TCPPort  int32 `json:"tcpPort,omitempty"     yaml:"tcpPort"`

	Clusters map[string]Cluster `json:"clusters,omitempty"`
}

// Layout defines layout section of .spec.configuration.clusters
type Layout struct {
	Type              string           `json:"type,omitempty"`
	ShardsCount       int              `json:"shardsCount,omitempty"`
	ReplicasCount     int              `json:"replicasCount,omitempty"`
	Shards            map[string]Shard `json:"shards,omitempty"`
	ShardsSpecified   bool             `json:"-"`
	ReplicasSpecified bool             `json:"-"`
	Master            string           `json:"=,omitempty"`
}

// Cluster defines item of a clusters section of .configuration
type Cluster struct {
	Name      string          `json:"name"`
	Kafka     KafkaConfig     `json:"kafka,omitempty"`
	Zookeeper ZookeeperConfig `json:"zookeeper,omitempty"`
	Settings  Settings        `json:"settings,omitempty"`
	Files     Settings        `json:"files,omitempty"`
	Layout    Layout          `json:"layout"`
}

type Shard struct {
	Name                string             `json:"name,omitempty"`
	Weight              int                `json:"weight,omitempty"`
	InternalReplication string             `json:"internalReplication,omitempty"`
	Settings            Settings           `json:"settings,omitempty"`
	Files               Settings           `json:"files,omitempty"`
	ReplicaCount        int                `json:"replicaCount,omitempty"`
	Replicas            map[string]Replica `json:"replicas,omitempty"`
}

// Replica is either Up/Down/Offline/Tombstone
type Replica struct {
	// store id is also uint64, due to the same reason as pd id, we store id as string
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Role      string        `json:"role,omitempty"`
	Image     string        `json:"image,omitempty"`
	Config    ReplicaConfig `json:"-"`
	Settings  Settings      `json:"settings,omitempty"`
	Files     Settings      `json:"files,omitempty"`
	Templates TemplateNames `json:"templates,omitempty"`
}

// TemplateNames defines references to .spec.templates to be used on current level of cluster
type TemplateNames struct {
	PodTemplate             string `json:"podTemplate,omitempty"             yaml:"podTemplate"`
	DataVolumeClaimTemplate string `json:"dataVolumeClaimTemplate,omitempty" yaml:"dataVolumeClaimTemplate"`
	LogVolumeClaimTemplate  string `json:"logVolumeClaimTemplate,omitempty"  yaml:"logVolumeClaimTemplate"`
}

// ReplicaConfig defines additional data related to a replica
type ReplicaConfig struct {
	ZookeeperFingerprint string `json:"zookeeperfingerprint"`
	SettingsFingerprint  string `json:"settingsfingerprint"`
	FilesFingerprint     string `json:"filesfingerprint"`
}

// DaisyPodTemplate defines full Pod Template, directly used by StatefulSet
type DaisyPodTemplate struct {
	Name         string            `json:"name"                    yaml:"name"`
	GenerateName string            `json:"generateName,omitempty"  yaml:"generateName"`
	ObjectMeta   metav1.ObjectMeta `json:"metadata"                  yaml:"metadata"`
	Spec         corev1.PodSpec    `json:"spec"                      yaml:"spec"`
}

// DaisyInstallationStatus defines the observed state of DaisyInstallation
type DaisyInstallationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Version              string                   `json:"version"`
	ClustersCount        int                      `json:"clustersCount"`
	ShardsCount          int                      `json:"shardsCount"`
	ReplicasCount        int                      `json:"replicasCount"`
	State                string                   `json:"state"`
	Phase                MemberPhase              `json:"phase,omitempty"`
	UpdatedReplicasCount int                      `json:"updated"`
	AddedReplicasCount   int                      `json:"added"`
	DeletedReplicasCount int                      `json:"deleted"`
	ReadyReplicas        int                      `json:"readyReplicas"`
	PrevSpec             DaisyInstallationSpec    `json:"prevSpec,omitempty"`
	Clusters             map[string]ClusterStatus `json:"clusters,omitempty"`
	// Represents the latest available observations of a tidb cluster's state.
	// +kubebuilder:validation:Optional
	Conditions []DaisyClusterCondition `json:"conditions,omitempty"`

	ExpectStatefulSets []appsv1.StatefulSet `json:"-"`
}

//ClusterStatus is status of Daisy Cluster
type ClusterStatus struct {
	Health bool                   `json:"health"`
	Shards map[string]ShardStatus `json:"shards,omitempty"`
}

type ShardStatus struct {
	Synced            bool                      `json:"synced,omitempty"`
	Phase             MemberPhase               `json:"phase,omitempty"`
	Name              string                    `json:"name,omitempty"`
	Health            bool                      `json:"health"`
	ReplicaCount      int                       `json:"replicaCount,omitempty"`
	Replicas          map[string]ReplicaStatus  `json:"replicas,omitempty"`
	FailureReplicas   map[string]FailureReplica `json:"failureReplicas,omitempty"`
	TombstoneReplicas map[string]ReplicaStatus  `json:"tombstoneReplicas,omitempty"`
}

type ReplicaStatus struct {
	// store id is also uint64, due to the same reason as pd id, we store id as string
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Phase       MemberPhase               `json:"phase,omitempty"`
	Health      bool                      `json:"health"`
	StatefulSet *appsv1.StatefulSetStatus `json:"statefulSet,omitempty"`
	IP          string                    `json:"ip"`
	LeaderCount int32                     `json:"leaderCount"`
	State       string                    `json:"state"`
	//LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime"`
	// Last time the health transitioned from one to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	Image              string      `json:"image,omitempty"`
}

// DaisyClusterCondition describes the state of a daisy cluster at a certain point.
type DaisyClusterCondition struct {
	// Type of the condition.
	Type DaisyClusterConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	// +kubebuilder:validation:Optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	// +kubebuilder:validation:Optional
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

// TidbClusterConditionType represents a tidb cluster condition value.
type DaisyClusterConditionType string

const (
	// DaisyClusterReady indicates that the tidb cluster is ready or not.
	// This is defined as:
	// - All statefulsets are up to date (currentRevision == updateRevision).
	// - All Shards are healthy.
	// - All Replicas are healthy.
	DaisyClusterReady DaisyClusterConditionType = "Ready"
)

// FailureReplica is the failure replica information
type FailureReplica struct {
	StatefulSetName string      `json:"statefulSetName,omitempty"`
	StoreID         string      `json:"storeID,omitempty"`
	CreatedAt       metav1.Time `json:"createdAt,omitempty"`
}

// ZookeeperConfig defines zookeeper section of .spec.configuration
// Refers to
// https://clickhouse.yandex/docs/en/single/index.html?#server-settings_zookeeper
type ZookeeperConfig struct {
	Nodes              []ZookeeperNode `json:"nodes,omitempty"                yaml:"nodes"`
	SessionTimeoutMs   int             `json:"session_timeout_ms,omitempty"   yaml:"session_timeout_ms"`
	OperationTimeoutMs int             `json:"operation_timeout_ms,omitempty" yaml:"operation_timeout_ms"`
	Root               string          `json:"root,omitempty"                 yaml:"root"`
	Identity           string          `json:"identity,omitempty"             yaml:"identity"`
}

// ZookeeperNode defines item of nodes section of .spec.configuration.zookeeper
type ZookeeperNode struct {
	Host string `json:"host,omitempty" yaml:"host"`
	Port int32  `json:"port,omitempty" yaml:"port"`
}

// KafaConfig defines kafka section of .spec.configuration
// Refers to
// https://clickhouse.yandex/docs/en/single/index.html?#server-settings_zookeeper
type KafkaConfig struct {
	Nodes []ZookeeperNode `json:"nodes,omitempty"                yaml:"nodes"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="version",type="string",JSONPath=".status.version",description="Elasticsearch version"
// +kubebuilder:printcolumn:name="clusters",type="string",JSONPath=".status.clustersCount"
// +kubebuilder:printcolumn:name="shards",type="string",JSONPath=".status.shardsCount"
// +kubebuilder:printcolumn:name="replicas",type="string",JSONPath=".status.replicasCount"
// +kubebuilder:printcolumn:name="state",type="string",JSONPath=".status.state"
// +kubebuilder:printcolumn:name="phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="updated",type="string",JSONPath=".status.updated"
// +kubebuilder:printcolumn:name="added",type="string",JSONPath=".status.added"
// +kubebuilder:printcolumn:name="deleted",type="string",JSONPath=".status.deleted"

// DaisyInstallation is the Schema for the daisyinstallations API
type DaisyInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DaisyInstallationSpec   `json:"spec,omitempty"`
	Status DaisyInstallationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DaisyInstallationList contains a list of DaisyInstallation
type DaisyInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DaisyInstallation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DaisyInstallation{}, &DaisyInstallationList{})
}

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DaisyOperatorConfigurationSpec defines the desired state of DaisyOperatorConfiguration
// !!! IMPORTANT !!!
// !!! IMPORTANT !!!
// !!! IMPORTANT !!!
// !!! IMPORTANT !!!
// !!! IMPORTANT !!!
// Do not forget to update func (config *DaisyOperatorConfigurationSpec) String()
// Do not forget to update CRD spec
type DaisyOperatorConfigurationSpec struct {
	// Full path to the config file and folder where this DaisyOperatorConfigurationSpec originates from
	ConfigFilePath   string `json:"-"`
	ConfigFolderPath string `json:"-"`

	// WatchNamespaces where operator watches for events
	WatchNamespaces []string `json:"watchNamespaces" yaml:"watchNamespaces"`

	// Paths where to look for additional ClickHouse config .xml files to be mounted into Pod
	CHCommonConfigsPath string `json:"chCommonConfigsPath" yaml:"chCommonConfigsPath"`
	CHHostConfigsPath   string `json:"chHostConfigsPath"   yaml:"chHostConfigsPath"`
	CHUsersConfigsPath  string `json:"chUsersConfigsPath"  yaml:"chUsersConfigsPath"`
	// DaisyOperatorConfigurationSpec files fetched from these paths. Maps "file name->file content"
	CHCommonConfigs map[string]string `json:"-"`
	CHHostConfigs   map[string]string `json:"-"`
	CHUsersConfigs  map[string]string `json:"-"`

	// Path where to look for ClickHouseInstallation templates .yaml files
	CHITemplatesPath string `json:"chiTemplatesPath" yaml:"chiTemplatesPath"`
	// CHI template files fetched from this path. Maps "file name->file content"
	CHITemplateFiles map[string]string `json:"-"`
	// CHI template objects unmarshalled from CHITemplateFiles. Maps "metadata.name->object"
	CHITemplates []*DaisyInstallation `json:"-"`
	// ClickHouseInstallation template
	CHITemplate *DaisyInstallation `json:"-"`

	// Create/Update StatefulSet behavior - for how long to wait for StatefulSet to reach new Generation
	StatefulSetUpdateTimeout uint64 `json:"statefulSetUpdateTimeout" yaml:"statefulSetUpdateTimeout"`
	// Create/Update StatefulSet behavior - for how long to sleep while polling StatefulSet to reach new Generation
	StatefulSetUpdatePollPeriod uint64 `json:"statefulSetUpdatePollPeriod" yaml:"statefulSetUpdatePollPeriod"`

	// Rolling Create/Update behavior
	// StatefulSet create behavior - what to do in case StatefulSet can't reach new Generation
	OnStatefulSetCreateFailureAction string `json:"onStatefulSetCreateFailureAction" yaml:"onStatefulSetCreateFailureAction"`
	// StatefulSet update behavior - what to do in case StatefulSet can't reach new Generation
	OnStatefulSetUpdateFailureAction string `json:"onStatefulSetUpdateFailureAction" yaml:"onStatefulSetUpdateFailureAction"`

	// Default values for ClickHouse user configuration
	// 1. user/profile - string
	// 2. user/quota - string
	// 3. user/networks/ip - multiple strings
	// 4. user/password - string
	CHConfigUserDefaultProfile    string   `json:"chConfigUserDefaultProfile"    yaml:"chConfigUserDefaultProfile"`
	CHConfigUserDefaultQuota      string   `json:"chConfigUserDefaultQuota"      yaml:"chConfigUserDefaultQuota"`
	CHConfigUserDefaultNetworksIP []string `json:"chConfigUserDefaultNetworksIP" yaml:"chConfigUserDefaultNetworksIP"`
	CHConfigUserDefaultPassword   string   `json:"chConfigUserDefaultPassword"   yaml:"chConfigUserDefaultPassword"`

	CHConfigNetworksHostRegexpTemplate string `json:"chConfigNetworksHostRegexpTemplate" yaml:"chConfigNetworksHostRegexpTemplate"`
	// Username and Password to be used by operator to connect to ClickHouse instances for
	// 1. Metrics requests
	// 2. Schema maintenance
	// User credentials can be specified in additional ClickHouse config files located in `chUsersConfigsPath` folder
	CHUsername string `json:"chUsername" yaml:"chUsername"`
	CHPassword string `json:"chPassword" yaml:"chPassword"`
	CHPort     int    `json:"chPort"     yaml:"chPort"`

	Logtostderr      string `json:"logtostderr"      yaml:"logtostderr"`
	Alsologtostderr  string `json:"alsologtostderr"  yaml:"alsologtostderr"`
	V                string `json:"v"                yaml:"v"`
	Stderrthreshold  string `json:"stderrthreshold"  yaml:"stderrthreshold"`
	Vmodule          string `json:"vmodule"          yaml:"vmodule"`
	Log_backtrace_at string `json:"log_backtrace_at" yaml:"log_backtrace_at"`

	// Max number of concurrent reconciles in progress
	ReconcileThreadsNumber int `json:"reconcileThreadsNumber" yaml:"reconcileThreadsNumber"`

	// Image of BusyBox
	ImageBusyBox string `json:"imagebusybox" yaml:"imageBusyBox"`

	//
	// The end of DaisyOperatorConfigurationSpec
	//
	// !!! IMPORTANT !!!
	// !!! IMPORTANT !!!
	// !!! IMPORTANT !!!
	// !!! IMPORTANT !!!
	// !!! IMPORTANT !!!
	// Do not forget to update func (config *DaisyOperatorConfigurationSpec) String()
	// Do not forget to update CRD spec
}

// DaisyOperatorConfigurationStatus defines the observed state of DaisyOperatorConfiguration
type DaisyOperatorConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DaisyOperatorConfiguration is the Schema for the daisyoperatorconfigurations API
type DaisyOperatorConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DaisyOperatorConfigurationSpec   `json:"spec,omitempty"`
	Status DaisyOperatorConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DaisyOperatorConfigurationList contains a list of DaisyOperatorConfiguration
type DaisyOperatorConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DaisyOperatorConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DaisyOperatorConfiguration{}, &DaisyOperatorConfigurationList{})
}

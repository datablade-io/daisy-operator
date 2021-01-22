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
// +genclient:noStatus
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true

// DaisyTemplate is the Schema for the daisytemplates API
type DaisyTemplate DaisyInstallation

// +kubebuilder:object:root=true

// DaisyTemplateList contains a list of DaisyTemplate
type DaisyTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DaisyTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DaisyTemplate{}, &DaisyTemplateList{})
}

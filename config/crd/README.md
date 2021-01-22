# Important
The generated crd files under _crd/bases_ folder, need some extra steps to modify which includes in **patches/settings_in_daisyinstallations.yaml** 
Because controller-gen tool of kubebuilder will add "addtionProperties" nodes to CRDs generated, for settings of daisy server, a setting can be either map of string, or map of arrays, therefore it cannot work with "additionalProperties". 

To workaround this, all setting related fields like "users", "files", should be of type object 
and add "x-kubernetes-preserve-unknown-fields: true" property.
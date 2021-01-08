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

package controllers

import (
	"context"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sort"

	v1 "github.com/daisy/daisy-operator/api/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
	// log "k8s.io/klog"
)

type ConfigManager struct {
	client.Client
	chopConfigList *v1.DaisyOperatorConfigurationList

	// initConfigFilePath is path to the configuration file, which will be used as initial/seed
	// to build final config, which will be used/consumed by users
	initConfigFilePath string

	// fileConfig is a prepared file-based config
	fileConfig *v1.DaisyOperatorConfigurationSpec

	// crConfigs is a slice of prepared Custom Resource based configs
	crConfigs []*v1.DaisyOperatorConfigurationSpec

	// config is the final config,
	// built as merge of all available configs and it is ready to use/be consumed by users
	config *v1.DaisyOperatorConfigurationSpec

	// runtimeParams is set/map of runtime params, influencing configuration
	runtimeParams map[string]string
}

// NewConfigManager creates new ConfigManager
func NewConfigManager(
	client client.Client,
	initConfigFilePath string,
) (cm *ConfigManager, err error) {
	cm = &ConfigManager{
		Client:             client,
		initConfigFilePath: initConfigFilePath,
	}
	if err = cm.Init(); err != nil {
		return nil, err
	}
	return cm, nil
}

// Init reads config from all sources
func (cm *ConfigManager) Init() error {
	var err error

	// Get ENV vars
	cm.runtimeParams = cm.getEnvVarParams()
	cm.logEnvVarParams()

	// Get initial config from file
	cm.fileConfig, err = cm.getFileBasedConfig(cm.initConfigFilePath)
	if err != nil {
		return err
	}
	klog.V(1).Info("File-based ClickHouseOperatorConfigurations")
	cm.fileConfig.WriteToLog()

	// Get configs from all config Custom Resources
	watchedNamespace := cm.fileConfig.GetInformerNamespace()
	cm.getCRBasedConfigs(watchedNamespace)
	cm.logCRBasedConfigs()

	// Prepare one unified config from all available config pieces
	cm.buildUnifiedConfig()

	// From now on we have one unified CHOP config
	klog.V(1).Info("Unified (but not post-processed yet) CHOP config")
	cm.config.WriteToLog()

	// Finalize config by post-processing
	cm.config.Postprocess()

	// DaisyOperatorConfigurationSpec is ready
	klog.V(1).Info("Final CHOP config")
	cm.config.WriteToLog()

	return nil
}

// DaisyOperatorConfigurationSpec is an access wrapper
func (cm *ConfigManager) Config() *v1.DaisyOperatorConfigurationSpec {
	return cm.config
}

// getCRBasedConfigs reads all ClickHouseOperatorConfiguration objects in specified namespace
func (cm *ConfigManager) getCRBasedConfigs(namespace string) {
	// We need to have chop kube client available in order to fetch ClickHouseOperatorConfiguration objects
	if cm.Client == nil {
		return
	}

	// Get list of ClickHouseOperatorConfiguration objects
	var err error
	cm.chopConfigList = &v1.DaisyOperatorConfigurationList{}
	if err = cm.Client.List(context.Background(), cm.chopConfigList); err != nil {
		klog.V(1).Infof("Error read ClickHouseOperatorConfigurations %v", err)
		return
	}

	if cm.chopConfigList == nil {
		return
	}

	// Get sorted names of ClickHouseOperatorConfiguration objects from the list of objects
	var names []string
	for i := range cm.chopConfigList.Items {
		chOperatorConfiguration := &cm.chopConfigList.Items[i]
		names = append(names, chOperatorConfiguration.Name)
	}
	sort.Strings(names)

	// Build sorted slice of configs
	for _, name := range names {
		for i := range cm.chopConfigList.Items {
			// Convenience wrapper
			chOperatorConfiguration := &cm.chopConfigList.Items[i]
			if chOperatorConfiguration.Name == name {
				// Save location info into DaisyOperatorConfigurationSpec itself
				chOperatorConfiguration.Spec.ConfigFolderPath = namespace
				chOperatorConfiguration.Spec.ConfigFilePath = name

				cm.crConfigs = append(cm.crConfigs, &chOperatorConfiguration.Spec)
				continue
			}
		}
	}
}

// logCRBasedConfigs writes all ClickHouseOperatorConfiguration objects into log
func (cm *ConfigManager) logCRBasedConfigs() {
	for _, chOperatorConfiguration := range cm.crConfigs {
		klog.V(1).Infof("chop config %s/%s :", chOperatorConfiguration.ConfigFolderPath, chOperatorConfiguration.ConfigFilePath)
		chOperatorConfiguration.WriteToLog()
	}
}

// buildUnifiedConfig prepares one config from all accumulated parts
func (cm *ConfigManager) buildUnifiedConfig() {
	// Start with file config as a base
	cm.config = cm.fileConfig
	cm.fileConfig = nil

	// Merge all the rest CR-based configs into base config
	for _, chOperatorConfiguration := range cm.crConfigs {
		cm.config.MergeFrom(chOperatorConfiguration, v1.MergeTypeOverrideByNonEmptyValues)
	}
}

// IsConfigListed checks whether specified ClickHouseOperatorConfiguration is listed in list of ClickHouseOperatorConfiguration(s)
func (cm *ConfigManager) IsConfigListed(config *v1.DaisyOperatorConfiguration) bool {
	for i := range cm.chopConfigList.Items {
		chOperatorConfiguration := &cm.chopConfigList.Items[i]

		if config.Namespace == chOperatorConfiguration.Namespace &&
			config.Name == chOperatorConfiguration.Name &&
			config.ResourceVersion == chOperatorConfiguration.ResourceVersion {
			// Yes, this config already listed with the same resource version
			return true
		}
	}

	return false
}

// getFileBasedConfig creates DaisyOperatorConfigurationSpec object based on file specified
func (cm *ConfigManager) getFileBasedConfig(configFilePath string) (*v1.DaisyOperatorConfigurationSpec, error) {
	// In case we have config file specified - that's it
	if len(configFilePath) > 0 {
		// Config file explicitly specified as CLI flag
		if conf, err := cm.buildConfigFromFile(configFilePath); err == nil {
			return conf, nil
		} else {
			return nil, err
		}
	}

	// No file specified - look for ENV var config file path specification
	if len(os.Getenv(v1.CHOP_CONFIG)) > 0 {
		// Config file explicitly specified as ENV var
		if conf, err := cm.buildConfigFromFile(os.Getenv(v1.CHOP_CONFIG)); err == nil {
			return conf, nil
		} else {
			return nil, err
		}
	}

	// No ENV var specified - look into user's homedir
	// Try to find ~/.clickhouse-operator/config.yaml
	usr, err := user.Current()
	if err == nil {
		// OS user found. Parse ~/.clickhouse-operator/config.yaml file
		if conf, err := cm.buildConfigFromFile(filepath.Join(usr.HomeDir, ".clickhouse-operator", "config.yaml")); err == nil {
			// Able to build config, all is fine
			return conf, nil
		}
	}

	// No config file in user's homedir - look for global config in /etc/
	// Try to find /etc/clickhouse-operator/config.yaml
	if conf, err := cm.buildConfigFromFile("/etc/clickhouse-operator/config.yaml"); err == nil {
		// Able to build config, all is fine
		return conf, nil
	}

	// No config file found, use default one
	return cm.buildDefaultConfig()
}

// buildConfigFromFile returns DaisyOperatorConfigurationSpec struct built out of specified file path
func (cm *ConfigManager) buildConfigFromFile(configFilePath string) (*v1.DaisyOperatorConfigurationSpec, error) {
	// Read config file content
	yamlText, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}

	// Parse config file content into DaisyOperatorConfigurationSpec struct
	config := new(v1.DaisyOperatorConfigurationSpec)
	err = yaml.Unmarshal(yamlText, config)
	if err != nil {
		return nil, err
	}

	// Fill DaisyOperatorConfigurationSpec's paths
	config.ConfigFilePath, err = filepath.Abs(configFilePath)
	config.ConfigFolderPath = filepath.Dir(config.ConfigFilePath)

	return config, nil

}

// buildDefaultConfig returns default DaisyOperatorConfigurationSpec
func (cm *ConfigManager) buildDefaultConfig() (*v1.DaisyOperatorConfigurationSpec, error) {
	config := new(v1.DaisyOperatorConfigurationSpec)

	return config, nil
}

// getEnvVarParamNames return list of ENV VARS parameter names
func (cm *ConfigManager) getEnvVarParamNames() []string {
	// This list of ENV VARS is specified in operator .yaml manifest, section "kind: Deployment"
	return []string{
		v1.OPERATOR_POD_NODE_NAME,
		v1.OPERATOR_POD_NAME,
		v1.OPERATOR_POD_NAMESPACE,
		v1.OPERATOR_POD_IP,
		v1.OPERATOR_POD_SERVICE_ACCOUNT,

		v1.OPERATOR_CONTAINER_CPU_REQUEST,
		v1.OPERATOR_CONTAINER_CPU_LIMIT,
		v1.OPERATOR_CONTAINER_MEM_REQUEST,
		v1.OPERATOR_CONTAINER_MEM_LIMIT,

		v1.WATCH_NAMESPACE,
		v1.WATCH_NAMESPACES,
	}
}

// getEnvVarParams returns map[string]string of ENV VARS with some runtime parameters
func (cm *ConfigManager) getEnvVarParams() map[string]string {
	params := make(map[string]string)
	// Extract parameters from ENV VARS
	for _, varName := range cm.getEnvVarParamNames() {
		params[varName] = os.Getenv(varName)
	}

	return params
}

// logEnvVarParams writes runtime parameters into log
func (cm *ConfigManager) logEnvVarParams() {
	// Log params according to sorted names
	// So we need to
	// 1. Extract and sort names aka keys
	// 2. Walk over keys and log params

	// Sort names aka keys
	var keys []string
	for k := range cm.runtimeParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Walk over sorted names aka keys
	klog.V(1).Infof("Parameters num: %d\n", len(cm.runtimeParams))
	for _, k := range keys {
		klog.V(1).Infof("%s=%s\n", k, cm.runtimeParams[k])
	}
}

// GetRuntimeParam gets specified runtime param
func (cm *ConfigManager) GetRuntimeParam(name string) (string, bool) {
	_map := cm.getEnvVarParams()
	value, ok := _map[name]
	return value, ok
}

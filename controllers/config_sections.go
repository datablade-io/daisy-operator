// Copyright 2019 Altinity Ltd and/or its affiliates. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	v1 "github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/util"
)

// configSectionsGenerator
type configSectionsGenerator struct {
	// ClickHouse config generator
	configGenerator *ConfigGenerator
	// clickhouse-operator configuration
	config *v1.DaisyOperatorConfigurationSpec
}

// NewConfigSections
func NewConfigSectionsGenerator(
	configGenerator *ConfigGenerator,
	config *v1.DaisyOperatorConfigurationSpec,
) *configSectionsGenerator {
	return &configSectionsGenerator{
		configGenerator: configGenerator,
		config:          config,
	}
}

// CreateConfigsCommon
func (c *configSectionsGenerator) CreateConfigsCommon() map[string]string {
	commonConfigSections := make(map[string]string)
	// commonConfigSections maps section name to section XML config of the following sections:
	// 1. remote servers
	// 2. common settings
	// 3. common files
	util.IncludeNonEmpty(commonConfigSections, createConfigSectionFilename(configRemoteServers), c.configGenerator.GetRemoteServers())
	util.IncludeNonEmpty(commonConfigSections, createConfigSectionFilename(configSettings), c.configGenerator.GetSettings(nil))
	util.MergeStringMaps(commonConfigSections, c.configGenerator.GetFiles(v1.SectionCommon, true, nil))
	// Extra user-specified config files
	util.MergeStringMaps(commonConfigSections, c.config.CHCommonConfigs)

	return commonConfigSections
}

// CreateConfigsUsers
func (c *configSectionsGenerator) CreateConfigsUsers() map[string]string {
	commonUsersConfigSections := make(map[string]string)
	// commonUsersConfigSections maps section name to section XML config of the following sections:
	// 1. users
	// 2. quotas
	// 3. profiles
	// 4. user files
	util.IncludeNonEmpty(commonUsersConfigSections, createConfigSectionFilename(configUsers), c.configGenerator.GetUsers())
	util.IncludeNonEmpty(commonUsersConfigSections, createConfigSectionFilename(configQuotas), c.configGenerator.GetQuotas())
	util.IncludeNonEmpty(commonUsersConfigSections, createConfigSectionFilename(configProfiles), c.configGenerator.GetProfiles())
	util.MergeStringMaps(commonUsersConfigSections, c.configGenerator.GetFiles(v1.SectionUsers, false, nil))
	// Extra user-specified config files
	util.MergeStringMaps(commonUsersConfigSections, c.config.CHUsersConfigs)

	return commonUsersConfigSections
}

// CreateConfigsHost
func (c *configSectionsGenerator) CreateConfigsHost(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) map[string]string {
	// Prepare for this replica deployment config files map as filename->content
	hostConfigSections := make(map[string]string)
	util.IncludeNonEmpty(hostConfigSections, createConfigSectionFilename(configMacros), c.configGenerator.GetHostMacros(ctx, replica))
	util.IncludeNonEmpty(hostConfigSections, createConfigSectionFilename(configPorts), c.configGenerator.GetHostPorts(replica))
	util.IncludeNonEmpty(hostConfigSections, createConfigSectionFilename(configZookeeper), c.configGenerator.GetHostZookeeper(ctx, di, replica))
	util.IncludeNonEmpty(hostConfigSections, createConfigSectionFilename(configSettings), c.configGenerator.GetSettings(replica))
	util.MergeStringMaps(hostConfigSections, c.configGenerator.GetFiles(v1.SectionHost, true, replica))
	// Extra user-specified config files
	util.MergeStringMaps(hostConfigSections, c.config.CHHostConfigs)

	return hostConfigSections
}

// createConfigSectionFilename
func createConfigSectionFilename(section string) string {
	return "daisy-generated-" + section + ".xml"
}

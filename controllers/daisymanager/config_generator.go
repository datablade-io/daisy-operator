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

package daisymanager

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/util"
	xmlbuilder "github.com/daisy/daisy-operator/pkg/util/xml"
)

// ConfigGenerator generates ClickHouse configuration files content for specified di
// ClickHouse configuration files content is an XML ATM, so config generator provides set of Get*() functions
// which produces XML which are parts of ClickHouse configuration and can/should be used as ClickHouse config files.
type ConfigGenerator struct {
	di *v1.DaisyInstallation
}

// NewConfigGenerator returns new ConfigGenerator struct
func NewConfigGenerator(di *v1.DaisyInstallation) *ConfigGenerator {
	return &ConfigGenerator{
		di: di,
	}
}

// GetUsers creates data for "users.xml"
func (c *ConfigGenerator) GetUsers() string {
	return c.generateXMLConfig(c.di.Spec.Configuration.Users, configUsers)
}

// GetProfiles creates data for "profiles.xml"
func (c *ConfigGenerator) GetProfiles() string {
	return c.generateXMLConfig(c.di.Spec.Configuration.Profiles, configProfiles)
}

// GetQuotas creates data for "quotas.xml"
func (c *ConfigGenerator) GetQuotas() string {
	return c.generateXMLConfig(c.di.Spec.Configuration.Quotas, configQuotas)
}

// GetSettings creates data for "settings.xml"
func (c *ConfigGenerator) GetSettings(r *v1.Replica) string {
	if r == nil {
		return c.generateXMLConfig(c.di.Spec.Configuration.Settings, "")
	} else {
		return c.generateXMLConfig(r.Settings, "")
	}
}

// GetFiles creates data for custom common config files
func (c *ConfigGenerator) GetFiles(section v1.SettingsSection, includeUnspecified bool, r *v1.Replica) map[string]string {
	var files v1.Settings
	if r == nil {
		// We are looking into Common files
		files = c.di.Spec.Configuration.Files
	} else {
		files = r.Files
	}

	// Extract particular section from cm

	return files.GetSectionStringMap(section, includeUnspecified)
}

// GetHostZookeeper creates data for "zookeeper.xml"
func (c *ConfigGenerator) GetHostZookeeper(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) string {
	cluster := ctx.Clusters[ctx.CurCluster]
	zk := &cluster.Zookeeper

	if zk.IsEmpty() {
		// No Zookeeper nodes provided
		return ""
	}

	b := &bytes.Buffer{}
	// <yandex>
	//		<zookeeper>
	util.Iline(b, 0, "<"+xmlTagYandex+">")
	util.Iline(b, 4, "<zookeeper>")

	// Append Zookeeper nodes
	for i := range zk.Nodes {
		// Convenience wrapper
		node := &zk.Nodes[i]
		// <node>
		//		<host>HOST</host>
		//		<port>PORT</port>
		// </node>
		util.Iline(b, 8, "<node>")
		util.Iline(b, 8, "    <host>%s</host>", node.Host)
		util.Iline(b, 8, "    <port>%d</port>", node.Port)
		util.Iline(b, 8, "</node>")
	}

	// Append session_timeout_ms
	if zk.SessionTimeoutMs > 0 {
		util.Iline(b, 8, "<session_timeout_ms>%d</session_timeout_ms>", zk.SessionTimeoutMs)
	}

	// Append operation_timeout_ms
	if zk.OperationTimeoutMs > 0 {
		util.Iline(b, 8, "<operation_timeout_ms>%d</operation_timeout_ms>", zk.OperationTimeoutMs)
	}

	// Append root
	if len(zk.Root) > 0 {
		util.Iline(b, 8, "<root>%s</root>", zk.Root)
	}

	// Append identity
	if len(zk.Identity) > 0 {
		util.Iline(b, 8, "<identity>%s</identity>", zk.Identity)
	}

	// </zookeeper>
	util.Iline(b, 4, "</zookeeper>")

	//TODO: config "distribueted_ddl" section
	// <distributed_ddl>
	//      <path>/x/y/di.name/z</path>
	//      <profile>X</profile>
	//util.Iline(b, 4, "<distributed_ddl>")
	//util.Iline(b, 4, "    <path>%s</path>", c.getDistributedDDLPath())
	//if c.di.Spec.Defaults.DistributedDDL.Profile != "" {
	//	util.Iline(b, 4, "    <profile>%s</profile>", c.di.Spec.Defaults.DistributedDDL.Profile)
	//}
	////		</distributed_ddl>
	//// </yandex>
	//util.Iline(b, 4, "</distributed_ddl>")
	util.Iline(b, 0, "</"+xmlTagYandex+">")

	return b.String()
}

// GetHostKafka creates data for "kafka.xml"
func (c *ConfigGenerator) GetHostKafka(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) string {
	cluster := ctx.Clusters[ctx.CurCluster]
	kafka := &cluster.Kafka

	if kafka.IsEmpty() {
		// No Zookeeper nodes provided
		return ""
	}

	b := &bytes.Buffer{}

	// <yandex>
	//		<cluster_settings>
	util.Iline(b, 0, "<"+xmlTagYandex+">")
	util.Iline(b, 4, "<cluster_settings>")

	// <streaming_storage> settings
	util.Iline(b, 8, "<streaming_storage>")
	util.Iline(b, 12, "<kafka>")

	util.Iline(b, 16, "<default>true</default>")
	util.Iline(b, 16, "<cluster_name>%s</cluster_name>", cluster.Name)
	util.Iline(b, 16, "<cluster_id>%s</cluster_id>", cluster.Name)
	util.Iline(b, 16, "<security_protocol>PLAINTEXT</security_protocol>")
	util.Iline(b, 16, "<replication_factor>2</replication_factor>")
	// Append Kafka brokers
	var nodes []string
	for i := range kafka.Nodes {
		node := &kafka.Nodes[i]
		nodes = append(nodes, fmt.Sprintf("%s:%d", node.Host, node.Port))
	}
	util.Iline(b, 16, "<brokers>%s</brokers>", strings.Join(nodes, ","))

	util.Iline(b, 16, "<topic_metadata_refresh_interval_ms>300000</topic_metadata_refresh_interval_ms>")
	util.Iline(b, 16, "<message_max_bytes>1000000</message_max_bytes>")
	util.Iline(b, 16, "<statistic_internal_ms>30000</statistic_internal_ms>")
	util.Iline(b, 16, "<debug></debug>")

	util.Iline(b, 16, "<enable_idempotence>true</enable_idempotence>")
	util.Iline(b, 16, "<queue_buffering_max_messages>100000</queue_buffering_max_messages>")
	util.Iline(b, 16, "<queue_buffering_max_kbytes>1048576</queue_buffering_max_kbytes>")
	util.Iline(b, 16, "<queue_buffering_max_ms>5</queue_buffering_max_ms>")
	util.Iline(b, 16, "<message_send_max_retries>2</message_send_max_retries>")
	util.Iline(b, 16, "<retry_backoff_ms>100</retry_backoff_ms>")
	util.Iline(b, 16, "<compression_codec>snappy</compression_codec>")

	util.Iline(b, 16, "<message_timeout_ms>40000</message_timeout_ms>")
	util.Iline(b, 16, "<message_delivery_async_poll_ms>100</message_delivery_async_poll_ms>")
	util.Iline(b, 16, "<message_delivery_sync_poll_ms>10</message_delivery_sync_poll_ms>")

	util.Iline(b, 16, "<group_id>daisy</group_id>")
	util.Iline(b, 16, "<message_max_bytes>1000000</message_max_bytes>")
	util.Iline(b, 16, "<enable_auto_commit>true</enable_auto_commit>")
	util.Iline(b, 16, "<check_crcs>false</check_crcs>")
	util.Iline(b, 16, "<auto_commit_interval_ms>5000</auto_commit_interval_ms>")
	util.Iline(b, 16, "<fetch_message_max_bytes>1048576</fetch_message_max_bytes>")

	util.Iline(b, 16, "<queued_min_messages>1000000</queued_min_messages>")
	util.Iline(b, 16, "<queued_max_messages_kbytes>65536</queued_max_messages_kbytes>")
	util.Iline(b, 16, "<internal_pool_size>2</internal_pool_size>")

	util.Iline(b, 12, "</kafka>")
	util.Iline(b, 8, "</streaming_storage>")

	// <system_ddls> settings
	util.Iline(b, 8, "<system_ddls>")
	util.Iline(b, 12, "<name>__system_ddls</name>")
	util.Iline(b, 12, "<replication_factor>1</replication_factor>")
	util.Iline(b, 12, "<data_retention>168</data_retention>")
	util.Iline(b, 8, "</system_ddls>")

	// <system_catalogs> settings
	util.Iline(b, 8, "<system_catalogs>")
	util.Iline(b, 12, "<name>__system_catalogs</name>")
	util.Iline(b, 12, "<replication_factor>1</replication_factor>")
	//util.Iline(b, 12, "<client_side_compression>false</client_side_compression>")
	util.Iline(b, 8, "</system_catalogs>")

	// <system_node_metrics> settings
	util.Iline(b, 8, "<system_node_metrics>")
	util.Iline(b, 12, "<name>__system_node_metrics</name>")
	util.Iline(b, 12, "<replication_factor>1</replication_factor>")
	util.Iline(b, 8, "</system_node_metrics>")

	// <system_tasks> settings
	util.Iline(b, 8, "<system_tasks>")
	util.Iline(b, 12, "<name>__system_tasks</name>")
	util.Iline(b, 12, "<replication_factor>1</replication_factor>")
	util.Iline(b, 12, "<data_retention>24</data_retention>")
	util.Iline(b, 8, "</system_tasks>")

	//		</cluster_settings>
	// </yandex>
	util.Iline(b, 4, "</cluster_settings>")
	util.Iline(b, 0, "</"+xmlTagYandex+">")

	return b.String()
}

// GetRemoteServers creates "remote_servers.xml" content and calculates data generation parameters for other sections
func (c *ConfigGenerator) GetRemoteServers() string {
	b := &bytes.Buffer{}

	replicas := make(map[string]v1.Replica)

	// <yandex>
	//		<remote_servers>
	util.Iline(b, 0, "<"+xmlTagYandex+">")
	util.Iline(b, 4, "<remote_servers>")

	util.Iline(b, 8, "<!-- User-specified clusters -->")

	// Build each cluster XML
	for _, cluster := range c.di.Spec.Configuration.Clusters {
		// <my_cluster_name>
		util.Iline(b, 8, "<%s>", cluster.Name)
		// Build each shard XML
		for _, shard := range cluster.Layout.Shards {
			// <shard>
			//		<internal_replication>VALUE(true/false)</internal_replication>
			util.Iline(b, 12, "<shard>")
			util.Iline(b, 16, "<internal_replication>%s</internal_replication>", shard.InternalReplication)

			//		<weight>X</weight>
			if shard.Weight > 0 {
				util.Iline(b, 16, "<weight>%d</weight>", shard.Weight)
			}

			for replicaName, replica := range shard.Replicas {
				// <replica>
				//		<host>XXX</host>
				//		<port>XXX</port>
				// </replica>
				replicas[replicaName] = replica
				util.Iline(b, 16, "<replica>")
				util.Iline(b, 16, "    <host>%s</host>", c.getRemoteServersReplicaHostname(&replica))
				//TODO: to support customized tcp port, host.TCPPort
				util.Iline(b, 16, "    <port>%d</port>", daisyDefaultTCPPortNumber)
				util.Iline(b, 16, "</replica>")
			}
			// </shard>
			util.Iline(b, 12, "</shard>")
		}
		// </my_cluster_name>
		util.Iline(b, 8, "</%s>", cluster.Name)
	}

	util.Iline(b, 8, "<!-- Autogenerated clusters -->")

	// One Shard All Replicas

	// <my_cluster_name>
	//     <shard>
	//         <internal_replication>
	clusterName := oneShardAllReplicasClusterName
	util.Iline(b, 8, "<%s>", clusterName)
	util.Iline(b, 8, "    <shard>")
	util.Iline(b, 8, "        <internal_replication>true</internal_replication>")
	for _, replica := range replicas {
		util.Iline(b, 16, "<replica>")
		util.Iline(b, 16, "    <host>%s</host>", c.getRemoteServersReplicaHostname(&replica))
		//TODO: to support customized tcp port, host.TCPPort
		util.Iline(b, 16, "    <port>%d</port>", daisyDefaultTCPPortNumber)
		util.Iline(b, 16, "</replica>")
	}
	//     </shard>
	// </my_cluster_name>
	util.Iline(b, 8, "    </shard>")
	util.Iline(b, 8, "</%s>", clusterName)

	// All Shards One Replica

	// <my_cluster_name>
	clusterName = allShardsOneReplicaClusterName
	util.Iline(b, 8, "<%s>", clusterName)
	for _, replica := range replicas {
		// <shard>
		//     <internal_replication>
		util.Iline(b, 12, "<shard>")
		util.Iline(b, 12, "    <internal_replication>false</internal_replication>")

		// <replica>
		//		<host>XXX</host>
		//		<port>XXX</port>
		// </replica>
		util.Iline(b, 16, "<replica>")
		util.Iline(b, 16, "    <host>%s</host>", c.getRemoteServersReplicaHostname(&replica))
		//TODO: to support customized tcp port, host.TCPPort
		util.Iline(b, 16, "    <port>%d</port>", daisyDefaultTCPPortNumber)
		util.Iline(b, 16, "</replica>")

		// </shard>
		util.Iline(b, 12, "</shard>")
	}
	// </my_cluster_name>
	util.Iline(b, 8, "</%s>", clusterName)

	// 		</remote_servers>
	// </yandex>
	util.Iline(b, 0, "    </remote_servers>")
	util.Iline(b, 0, "</"+xmlTagYandex+">")

	return b.String()
}

// GetHostMacros creates "macros.xml" content
func (c *ConfigGenerator) GetHostMacros(ctx *memberContext, r *v1.Replica) string {
	b := &bytes.Buffer{}

	// <yandex>
	//     <macros>
	util.Iline(b, 0, "<"+xmlTagYandex+">")
	util.Iline(b, 0, "    <macros>")

	// <installation>di-name-macros-value</installation>
	util.Iline(b, 8, "<installation>%s</installation>", ctx.Installation)

	// <CLUSTER_NAME>cluster-name-macros-value</CLUSTER_NAME>
	// util.Iline(b, 8, "<%s>%[2]s</%[1]s>", replica.Address.ClusterName, c.getMacrosCluster(replica.Address.ClusterName))
	// <CLUSTER_NAME-shard>0-based shard index within cluster</CLUSTER_NAME-shard>
	// util.Iline(b, 8, "<%s-shard>%d</%[1]s-shard>", replica.Address.ClusterName, replica.Address.ShardIndex)

	// All Shards One Replica Cluster
	// <CLUSTER_NAME-shard>0-based shard index within all-shards-one-replica-cluster</CLUSTER_NAME-shard>
	// TODO: translate host.Address.CHIScopeIndex to %d in the pattern
	util.Iline(b, 8, "<%s-shard>%d</%[1]s-shard>", allShardsOneReplicaClusterName, 0)

	// <cluster> and <shard> macros are applicable to main cluster only. All aux clusters do not have ambiguous macros
	// <cluster></cluster> macro
	util.Iline(b, 8, "<cluster>%s</cluster>", ctx.CurCluster)
	// <shard></shard> macro
	util.Iline(b, 8, "<shard>%d</shard>", ExtractLastOrdinal(ctx.CurShard))
	// <replica>replica id = full deployment id</replica>
	// full deployment id is unique to identify replica within the cluster
	util.Iline(b, 8, "<replica>%s</replica>", r.Name)

	// 		</macros>
	// </yandex>
	util.Iline(b, 0, "    </macros>")
	util.Iline(b, 0, "</"+xmlTagYandex+">")

	return b.String()
}

func (c *ConfigGenerator) GetNodeRoles(ctx *memberContext, r *v1.Replica) string {
	b := &bytes.Buffer{}

	// <yandex>
	//     <macros>
	util.Iline(b, 0, "<"+xmlTagYandex+">")
	util.Iline(b, 4, "<cluster_settings>")

	// <node_identity></node_identity> macro
	util.Iline(b, 8, "<node_identity>%s</node_identity>", r.Name)

	util.Iline(b, 8, "<node_roles>")
	if r.Role == "master" {
		util.Iline(b, 12, "<role>ddl</role>")
		util.Iline(b, 12, "<role>catalog</role>")
		util.Iline(b, 12, "<role>placement</role>")
		util.Iline(b, 12, "<role>task</role>")
	} else {
		util.Iline(b, 12, "<role>catalog</role>")
	}
	util.Iline(b, 8, "</node_roles>")

	// 		</cluster_settings>
	// </yandex>
	util.Iline(b, 4, "</cluster_settings>")
	util.Iline(b, 0, "</"+xmlTagYandex+">")

	return b.String()
}

func noCustomPorts(r *v1.Replica) bool {
	//if r.TCPPort != chDefaultTCPPortNumber {
	//	return false
	//}
	//
	//if r.HTTPPort != chDefaultHTTPPortNumber {
	//	return false
	//}
	//
	//if r.InterserverHTTPPort != chDefaultInterserverHTTPPortNumber {
	//	return false
	//}

	return true
}

// GetHostPorts creates "ports.xml" content
func (c *ConfigGenerator) GetHostPorts(r *v1.Replica) string {

	if noCustomPorts(r) {
		return ""
	}
	return ""

	//b := &bytes.Buffer{}
	//
	//// <yandex>
	//util.Iline(b, 0, "<"+xmlTagYandex+">")
	//
	//if host.TCPPort != chDefaultTCPPortNumber {
	//	util.Iline(b, 4, "<tcp_port>%d</tcp_port>", host.TCPPort)
	//}
	//if host.HTTPPort != chDefaultHTTPPortNumber {
	//	util.Iline(b, 4, "<http_port>%d</http_port>", host.HTTPPort)
	//}
	//if host.InterserverHTTPPort != chDefaultInterserverHTTPPortNumber {
	//	util.Iline(b, 4, "<interserver_http_port>%d</interserver_http_port>", host.InterserverHTTPPort)
	//}
	//
	//// </yandex>
	//util.Iline(b, 0, "</"+xmlTagYandex+">")
	//
	//return b.String()
}

// generateXMLConfig creates XML using map[string]string definitions
func (c *ConfigGenerator) generateXMLConfig(settings v1.Settings, prefix string) string {
	if len(settings) == 0 {
		return ""
	}

	b := &bytes.Buffer{}
	// <yandex>
	// XML code
	// </yandex>
	util.Iline(b, 0, "<"+xmlTagYandex+">")
	xmlbuilder.GenerateXML(b, settings, prefix)
	util.Iline(b, 0, "</"+xmlTagYandex+">")

	return b.String()
}

//
// Paths and Names section
//

// getDistributedDDLPath returns string path used in <distributed_ddl><path>XXX</path></distributed_ddl>
func (c *ConfigGenerator) getDistributedDDLPath() string {
	return fmt.Sprintf(distributedDDLPathPattern, c.di.Name)
}

// getRemoteServersReplicaHostname returns hostname (podhostname + service or FQDN) for "remote_servers.xml"
// based on .Spec.Defaults.ReplicasUseFQDN
func (c *ConfigGenerator) getRemoteServersReplicaHostname(r *v1.Replica) string {
	//if util.IsStringBoolTrue(c.di.Spec.Defaults.ReplicasUseFQDN) {
	//	// In case .Spec.Defaults.ReplicasUseFQDN is set replicas would use FQDN pod hostname,
	//	// otherwise hostname+service name (unique within namespace) would be used
	//	// .my-dev-namespace.svc.cluster.local
	//	return CreatePodFQDN(host)
	//} else {
	return CreatePodHostname(r)
	//}
}

// getMacrosInstallation returns macros value for <installation-name> macros
func (c *ConfigGenerator) getMacrosInstallation(name string) string {
	return util.CreateStringID(name, 6)
}

// getMacrosCluster returns macros value for <cluster-name> macros
func (c *ConfigGenerator) getMacrosCluster(name string) string {
	return util.CreateStringID(name, 4)
}

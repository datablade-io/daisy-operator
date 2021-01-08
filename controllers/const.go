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

import "github.com/daisy/daisy-operator/api/v1"

const (
	xmlTagYandex = "yandex"
)

const (
	configMacros        = "macros"
	configPorts         = "ports"
	configProfiles      = "profiles"
	configQuotas        = "quotas"
	configRemoteServers = "remote_servers"
	configSettings      = "settings"
	configUsers         = "users"
	configZookeeper     = "zookeeper"
)

const (
	// dirPathCommonConfig specifies full path to folder, where generated common XML files for ClickHouse would be placed
	// for the following sections:
	// 1. remote servers
	// 2. operator-provided additional config files
	dirPathCommonConfig = "/etc/clickhouse-server/" + v1.CommonConfigDir + "/"

	// dirPathUsersConfig specifies full path to folder, where generated users XML files for ClickHouse would be placed
	// for the following sections:
	// 1. users
	// 2. quotas
	// 3. profiles
	// 4. operator-provided additional config files
	dirPathUsersConfig = "/etc/clickhouse-server/" + v1.UsersConfigDir + "/"

	// dirPathHostConfig specifies full path to folder, where generated host XML files for ClickHouse would be placed
	// for the following sections:
	// 1. macros
	// 2. zookeeper
	// 3. settings
	// 4. files
	// 5. operator-provided additional config files
	dirPathHostConfig = "/etc/clickhouse-server/" + v1.HostConfigDir + "/"

	// dirPathClickHouseData specifies full path of data folder where ClickHouse would place its data storage
	dirPathClickHouseData = "/var/lib/clickhouse"

	// dirPathClickHouseLog  specifies full path of data folder where ClickHouse would place its log files
	dirPathClickHouseLog = "/var/log/clickhouse-server"
)

const (
	// Default ClickHouse docker image to be used
	defaultClickHouseDockerImage = "yandex/clickhouse-server:latest"

	// Default BusyBox docker image to be used
	defaultBusyBoxDockerImage = "busybox"

	// Name of container within Pod with ClickHouse instance. Pod may have other containers included, such as monitoring
	ClickHouseContainerName    = "clickhouse"
	ClickHouseLogContainerName = "clickhouse-log"
)

// Name related constants
const (
	// macrosNamespace is a sanitized namespace name where ClickHouseInstallation runs
	macrosNamespace = "{namespace}"
	// macrosChiName is a sanitized DaisyInstallation name
	macrosInsName = "{di}"
	// macrosClusterName is a sanitized cluster name
	macrosClusterName = "{cluster}"
	// macrosReplicaName is a sanitized replica name
	macrosReplicaName = "{replica}"

	// InstallationServiceNamePattern is a template of DaisyInstallation Service name. "daisy-{chi}"
	InstallationServiceNamePattern = "daisy-" + macrosInsName

	// statefulSetServiceNamePattern is a template of hosts's StatefulSet's Service name. "chi-{chi}-{cluster}-{shard}-{host}"
	statefulSetServiceNamePattern = "di-" + macrosInsName + "-" + macrosClusterName + "-" + macrosReplicaName

	// configMapCommonNamePattern is a template of common settings for the CHI ConfigMap. "di-{di}-common-configd"
	configMapCommonNamePattern = "di-" + macrosInsName + "-common-configd"

	// configMapCommonUsersNamePattern is a template of common users settings for the CHI ConfigMap. "di-{di}-common-usersd"
	configMapCommonUsersNamePattern = "di-" + macrosInsName + "-common-usersd"

	// configMapDeploymentNamePattern is a template of macros ConfigMap. "di-{di}-deploy-confd-{cluster}-{shard}-{replica}"
	configMapReplicaNamePattern = "di-" + macrosInsName + "-deploy-confd-" + macrosReplicaName

	//PodRegexpTemplate
	PodRegexpTemplate = "{di}-[^.]+\\d+-\\d+\\.{namespace}\\.svc\\.cluster\\.local$"

	DaisyFinalizer = "daisy-operator-finalizer"

	chPortNumberMustBeAssignedLater = 0

	// ClickHouse open ports
	daisyDefaultTCPPortName               = "tcp"
	daisyDefaultTCPPortNumber             = int32(9000)
	daisyDefaultHTTPPortName              = "http"
	daisyDefaultHTTPPortNumber            = int32(8123)
	daisyDefaultInterserverHTTPPortName   = "interserver"
	daisyDefaultInterserverHTTPPortNumber = int32(9009)

	// Default value for ClusterIP service
	templateDefaultsServiceClusterIP = "None"
)

const (
	zkDefaultPort = 2181
	// zkDefaultRootTemplate specifies default ZK root - /clickhouse/{namespace}/{chi name}
	zkDefaultRootTemplate = "/clickhouse/%s/%s"

	CHUsername = "default"
	CHPassword = ""

	distributedDDLPathPattern = "/clickhouse/%s/task_queue/ddl"

	// Special auto-generated clusters. Each of these clusters lay over all replicas in CHI
	// 1. Cluster with one shard and all replicas. Used to duplicate data over all replicas.
	// 2. Cluster with all shards (1 replica). Used to gather/scatter data over all replicas.

	oneShardAllReplicasClusterName = "all-replicated"
	allShardsOneReplicaClusterName = "all-sharded"
)

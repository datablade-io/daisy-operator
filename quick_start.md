# Quick Start Guides

# Table of Contents
* [Daisy Operator Installation](#daisy-operator-installation)
* [Examples](#examples)
  * [Trivial Example](#trivial-example)
  * [Connect to ClickHouse Database](#connect-to-clickhouse-database)
  * [Simple Persistent Volume Example](#simple-persistent-volume-example)
  * [Custom Deployment with Specific ClickHouse Configuration](#custom-deployment-with-specific-clickhouse-configuration)
  * [Deploy a DistributedMergeTree Cluster](#deploy-a-distributedmergetree-cluster)

# Prerequisites
1. Operational Kubernetes cluster
1. Deploy 'cert-manager' in kubernetes cluster, please refer [install cert-manager](https://cert-manager.io/docs/installation/)
1. Properly configured `kubectl`
1. `curl`

# Daisy Operator Installation

Currently only availaible by Makefile. Please refer 'make deploy' task in [README.md](README.md) . the follow command deploys daisy-operator in he configured default Kubernetes cluster in ~/.kube/config .

```bash
# install crds
make install

# deploy operator
make deploy IMG=registry.foundary.zone:8360/dae/daisy-operator:v0.5
```

Install progress is as follows:
```text
namespace/daisy-operator-system created
customresourcedefinition.apiextensions.k8s.io/daisyinstallations.daisy.com created
customresourcedefinition.apiextensions.k8s.io/daisyoperatorconfigurations.daisy.com created
customresourcedefinition.apiextensions.k8s.io/daisytemplates.daisy.com created
role.rbac.authorization.k8s.io/daisy-operator-leader-election-role created
clusterrole.rbac.authorization.k8s.io/daisy-operator-manager-role created
clusterrole.rbac.authorization.k8s.io/daisy-operator-metrics-reader created
clusterrole.rbac.authorization.k8s.io/daisy-operator-proxy-role created
rolebinding.rbac.authorization.k8s.io/daisy-operator-leader-election-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/daisy-operator-manager-rolebinding created
clusterrolebinding.rbac.authorization.k8s.io/daisy-operator-proxy-rolebinding created
configmap/daisy-operator-etc-clickhouse-operator-confd-files created
configmap/daisy-operator-etc-clickhouse-operator-configd-files created
configmap/daisy-operator-etc-clickhouse-operator-templatesd-files created
configmap/daisy-operator-etc-clickhouse-operator-usersd-files created
configmap/daisy-operator-manager-config created
service/daisy-operator-controller-manager-metrics-service created
service/daisy-operator-webhook-service created
deployment.apps/daisy-operator-controller-manager created
certificate.cert-manager.io/daisy-operator-serving-cert created
issuer.cert-manager.io/daisy-operator-selfsigned-issuer created
mutatingwebhookconfiguration.admissionregistration.k8s.io/daisy-operator-mutating-webhook-configuration created
validatingwebhookconfiguration.admissionregistration.k8s.io/daisy-operator-validating-webhook-configuration created
```

Check `daisy-operator` is running:
```bash
kubectl get pods -n daisy-operator-system
```
```text
NAME                                                 READY   STATUS    RESTARTS   AGE
daisy-operator-controller-manager-57b75fc495-97rjc   2/2     Running   0          3m23s
```

# Examples

There are several ready-to-use [Daisy Installation examples](./config/samples). Below are few ones to start with.

## Create Custom Namespace
It is a good practice to have all components run in dedicated namespaces. Let's run examples in `test` namespace
```bash
kubectl create namespace test
```
```text
namespace/test created
```

## Trivial example

This is the trivial [1 shard 1 replica](./config/samples/01-simple-1shard-1repl.yaml) example.

**WARNING**: Do not use it for anything other than 'Hello, world!', it does not have persistent storage!
 
```bash
kubectl apply -n test -f https://raw.githubusercontent.com/datablade-io/daisy-operator/master/config/samples/01-simple-1shard-1repl.yaml
```
```text
daisyinstallation.daisy.com/simple01 created
```

Installation specification is straightforward and defines 1-replica cluster:
```yaml
apiVersion: "daisy.com/v1"
kind: "DaisyInstallation"
metadata:
  name: "simple01"
spec:
  pvReclaimPolicy: Retain
  configuration:
    users:
      default/networks/ip:
        - "0.0.0.0/0"
    settings:
      listen_host:
        - "0.0.0.0"
    clusters:
      cluster:
        name: cluster
        layout:
          shardsCount: 1
          replicasCount:  1
```

Once cluster is created, there are two checks to be made.

```bash
kubectl get pods -n test
```
```text
NAME                     READY   STATUS    RESTARTS   AGE
simple01-cluster-0-0-0   1/1     Running   0          2m46s
```

Watch out for 'Running' status. Also check services created by an operator:

```bash
kubectl get service -n test
```
```text
NAME                   TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)                         AGE
daisy-simple01         NodePort    10.104.53.147   <none>        8123:31835/TCP,9000:30931/TCP   3m14s
simple01-cluster-0-0   ClusterIP   None            <none>        9000/TCP,8123/TCP,9009/TCP      3m14s
```

Daisy is up and running!

## Connect to ClickHouse Database

There are two ways to connect to ClickHouse database

1. In case previous command `kubectl get service -n test` reported **PORT(S)** (31853 in our case) we can directly access Daisy with any hostname of nodes of kubernetes cluster (p54006v.hulk.shyc2.qihoo.net in our case) :
```bash
--host p54006v.hulk.shyc2.qihoo.net --port 30931 --user default 
```
```text
ClickHouse client version 21.7.1.1.
Connecting to p54006v.hulk.shyc2.qihoo.net:30931 as user default.
Connected to ClickHouse server version 21.6.6 revision 54448.
``` 
1. In case there is not **EXTERNAL-IP** available, we can access ClickHouse from inside Kubernetes cluster
```bash
kubectl -n test exec -it simple01-cluster-0-0-0  -- clickhouse-client
```
```text
ClickHouse client version 21.6.6.51 (official build).
Connecting to localhost:9000 as user default.
Connected to ClickHouse server version 21.6.6 revision 54448.
```

## Simple Persistent Volume Example

In case of having Dynamic Volume Provisioning available - ex.: we are able to use hostpath-provisioner in our kubernetes cluster in lab
Manifest is [available in examples](config/samples/03-pv-default.yaml). 

Given the Storage Class name for persistent volume is 'kubevirt-hostpath-provisioner', this deployment includes: 
1. Deployment specified
1. Pod template
1. VolumeClaim template

```yaml
apiVersion: "daisy.com/v1"
kind: "DaisyInstallation"
metadata:
  name: "simple01"
spec:
  defaults:
    templates:
      dataVolumeClaimTemplate: data-volume-template
      logVolumeClaimTemplate: log-volume-template
  pvReclaimPolicy: Retain
  configuration:
    users:
      default/networks/ip:
        - "0.0.0.0/0"
    settings:
      listen_host:
        - "0.0.0.0"
    clusters:
      cluster:
        name: cluster
        layout:
          shardsCount: 1
          replicasCount:  1
  templates:
    volumeClaimTemplates:
      - name: data-volume-template
        spec:
          storageClassName:  kubevirt-hostpath-provisioner
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
      - name: log-volume-template
        spec:
          storageClassName:  kubevirt-hostpath-provisioner
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 100Mi
```

## Custom Deployment with Specific ClickHouse Configuration

You can tell operator to configure your ClickHouse, as shown in the example below ([link to the manifest][05-settings-01-overview.yaml]):

1. Pod template: with cpu, mem quota, image of Daisy specified: registry.foundary.zone:8360/dae-dev/daisy-server:1.2.0
1. VolumeClaim template
1. add three usesrs 'test', 'readonly' and 'admin'
1. change daisy settings, add a dictionary
1. create a daisy cluster with 2 shards, 2 replicas with external zookeeper used

```yaml
apiVersion: "daisy.com/v1"
kind: "DaisyInstallation"
metadata:
  name: "simple01"
spec:
  defaults:
    templates:
      podTemplate: pod-template
      dataVolumeClaimTemplate: data-volume-template
      logVolumeClaimTemplate: log-volume-template
  pvReclaimPolicy: Retain
  configuration:
    zookeeper:
      nodes:
        - host: tob44.bigdata.lycc.qihoo.net
          port: 2181
      session_timeout_ms: 30000
      operation_timeout_ms: 10000
      root: "/simple01"
    users:
      # test user has 'password' specified, while admin user has 'password_sha256_hex' specified
      test/password: qwerty
      test/networks/ip:
        - "0.0.0.0/0"
      test/profile: test_profile
      test/quota: test_quota
      test/allow_databases/database:
        - "dbname1"
        - "dbname2"
        - "dbname3"
      # admin use has 'password_sha256_hex' so actual password value is not published
      admin/password_sha256_hex: 8bd66e4932b4968ec111da24d7e42d399a05cb90bf96f587c3fa191c56c401f8
      admin/networks/ip: "0.0.0.0/0"
      admin/profile: default
      admin/quota: default
      # readonly user has 'password' field specified, not 'password_sha256_hex' as admin user above
      readonly/password: readonly_password
      readonly/profile: readonly
      readonly/quota: default
    profiles:
      test_profile/max_memory_usage: "1000000000"
      test_profile/readonly: "1"
      readonly/readonly: "1"
    quotas:
      test_quota/interval/duration: "3600"
    settings:
      listen_host: "0.0.0.0"
      compression/case/method: zstd
      disable_internal_dns_cache: 1
    files:
      dict1.xml: |
        <yandex>
            <!-- ref to file /etc/clickhouse-data/config.d/source1.csv -->
        </yandex>
      source1.csv: |
        a1,b1,c1,d1
        a2,b2,c2,d2
    clusters:
      cluster:
        name: cluster
        layout:
          shardsCount: 2
          replicasCount: 2
  templates:
    podTemplates:
      - name: pod-template
        metadata:
          name: "simple01"
        spec:
          containers:
            - name: clickhouse
              imagePullPolicy: Always
              image: registry.foundary.zone:8360/dae-dev/daisy-server:1.2.0
              resources:
                limits:
                  memory: "512Mi"
                  cpu: "0.5"
                requests:
                  memory: "512Mi"
                  cpu: "0.5"
    volumeClaimTemplates:
      - name: data-volume-template
        spec:
          storageClassName:  kubevirt-hostpath-provisioner
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
      - name: log-volume-template
        spec:
          storageClassName:  kubevirt-hostpath-provisioner
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 100Mi
```

## Deploy a DistributedMergeTree Cluster

You can tell operator to configure your ClickHouse, as shown in the example below ([link to the manifest][config/samples/06-daisy-cluster-00.yaml]):

1. Before you start, you should have an external kafka cluster.
1. 'clusterType' should be 'DistributedMergeTree'
1. Pod template: with cpu, mem quota, image of Daisy specified: registry.foundary.zone:8360/dae-dev/daisy-server:dev-k8s
1. VolumeClaim template
1. add three usesrs 'test', 'readonly' and 'admin'
1. change daisy settings, add a dictionary
1. create a daisy cluster with 2 shards, 2 replicas with external kafka used

```yaml
apiVersion: "daisy.com/v1"
kind: "DaisyInstallation"
metadata:
  name: "simple06"
spec:
  clusterType: "DistributedMergeTree"
  defaults:
    templates:
      podTemplate: pod-template
      dataVolumeClaimTemplate: data-volume-template
      logVolumeClaimTemplate: log-volume-template
  pvReclaimPolicy: Retain
  configuration:
    kafka:
      nodes:
        - host: tob44.bigdata.lycc.qihoo.net
          port: 9092
    settings:
      listen_host: "0.0.0.0"
    clusters:
      cluster:
        name: cluster
        layout:
          shardsCount: 2
          replicasCount: 2
  templates:
    podTemplates:
      - name: pod-template
        metadata:
          name: "simple01"
        spec:
          containers:
            - name: clickhouse
              imagePullPolicy: Always
              image: registry.foundary.zone:8360/dae-dev/daisy-server:dev-k8s
              resources:
                limits:
                  memory: "512Mi"
                  cpu: "0.5"
                requests:
                  memory: "512Mi"
                  cpu: "0.5"
              ports:
                - containerPort: 8123
                  name: http
                  protocol: TCP
                - containerPort: 9000
                  name: tcp
                  protocol: TCP
                - containerPort: 9009
                  name: interserver
                  protocol: TCP
              livenessProbe:
                failureThreshold: 3
                httpGet:
                  path: /ping
                  port: 8123
                  scheme: HTTP
                initialDelaySeconds: 10
                periodSeconds: 10
                successThreshold: 1
                timeoutSeconds: 1
              readinessProbe:
                failureThreshold: 3
                httpGet:
                  path: /ping
                  port: 8123
                  scheme: HTTP
                initialDelaySeconds: 12
                periodSeconds: 10
                successThreshold: 1
                timeoutSeconds: 1
    volumeClaimTemplates:
      - name: data-volume-template
        spec:
          storageClassName:  kubevirt-hostpath-provisioner
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 1Gi
      - name: log-volume-template
        spec:
          storageClassName:  kubevirt-hostpath-provisioner
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 100Mi
```
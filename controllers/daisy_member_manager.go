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
	"fmt"

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/label"
	"github.com/daisy/daisy-operator/pkg/util"

	"github.com/go-logr/logr"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager implements the logic for syncing replicas and shards of daisycluster.
type Manager interface {
	// Sync	implements the logic for syncing daisycluster.
	Sync(old, cur *v1.DaisyInstallation) error
}

type Dependencies struct {
	Log      logr.Logger
	Client   client.Client
	Recorder record.EventRecorder
}

// DaisyMemberManager implements Manager.
type DaisyMemberManager struct {
	deps       *Dependencies
	normalizer *Normalizer
	cfgMgr     *ConfigManager
	cfgGen     *configSectionsGenerator
	//failover                 Failover
	//scaler                   Scaler
	//upgrader                 Upgrader
	statefulSetIsUpgradingFn func(client.Client, *apps.StatefulSet, *v1.DaisyInstallation) (bool, error)
}

// NewTiKVMemberManager returns a *tikvMemberManager
func NewDaisyMemberManager(deps *Dependencies, initConfigFilePath string) (Manager, error) {
	cfgMgr, err := NewConfigManager(deps.Client, initConfigFilePath)
	if err != nil {
		return nil, err
	}
	m := &DaisyMemberManager{
		deps:       deps,
		normalizer: NewNormalizer(cfgMgr),
		cfgMgr:     cfgMgr,
		cfgGen:     nil,
		//failover: failover,
		//scaler:   scaler,
		//upgrader: upgrader,
	}
	m.statefulSetIsUpgradingFn = daisyStatefulSetIsUpgrading
	return m, nil
}

// Sync fulfills the manager.Manager interface
func (m *DaisyMemberManager) Sync(old, cur *v1.DaisyInstallation) error {
	ns := cur.GetNamespace()
	dcName := cur.GetName()

	log := m.deps.Log.WithValues("Namespace", ns, "Installation", dcName)

	if old == nil && cur == nil {
		return nil
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if !cur.ObjectMeta.DeletionTimestamp.IsZero() {
		return m.deleteDaisyInstallation(cur)
	} else {
		m.deps.Log.V(3).Info("Registering finalizer for Directory Service")

		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !util.InArray(DaisyFinalizer, cur.GetFinalizers()) {
			cur.SetFinalizers(append(cur.GetFinalizers(), DaisyFinalizer))
			//if err := m.deps.Client.Update(context.Background(), cur); err != nil {
			//	return err
			//}
		}
	}

	var err error

	cur = m.normalize(cur)
	m.cfgGen = NewConfigSectionsGenerator(NewConfigGenerator(cur), m.cfgMgr.Config())

	// No spec change and not to delete, it must be status change
	if old != nil && apiequality.Semantic.DeepEqual(old.Spec, cur.Spec) {
		log.V(2).Info("Spec is the same, it should be status change", "oldSpec", old.Spec, "newSpec", cur.Spec)
	}

	// Handle create & update scenarios
	cur.Status.Reset()
	ctx := &memberContext{
		Namespace:    ns,
		Installation: dcName,
	}
	if err = m.syncServiceForInstallation(ctx, cur); err != nil {
		log.Error(err, "Sync configmaps fail")
		return err
	}

	if err = m.syncConfigMaps(cur, false); err != nil {
		log.Error(err, "sync configmaps fail in first time")
		return err
	}

	if err = m.syncConfiguration(cur, &cur.Spec.Configuration); err != nil {
		if !IsRequeueError(err) {
			log.Error(err, "Sync configuration fail")
		}
		return err
	}

	//TODO: update status of installation
	// TODO: create service for cluster memeber
	//svcList := m.getMemeberService()

	if err = SetInstallationLastAppliedConfigAnnotation(cur); err != nil {
		return err
	}

	if err = m.syncConfigMaps(cur, true); err != nil {
		log.Error(err, "sync configmaps fail in 2nd time")
		return err
	}

	if m.IsReady(cur) {
		cur.Status.State = v1.StatusCompleted
	} else {
		if cur.Status.State == v1.StatusCompleted {
			cur.Status.State = v1.StatusInProgress
		}
	}

	return err
}

func (m *DaisyMemberManager) deleteDaisyInstallation(di *v1.DaisyInstallation) error {
	log := m.deps.Log.WithValues("Namespace", di.Namespace, "Installation", di.Name, "Action", "Delete")

	log.V(2).Info("start")
	defer log.V(2).Info("end")

	var err error

	// The object is being deleted
	if util.InArray(DaisyFinalizer, di.GetFinalizers()) {
		// our finalizer is present, so lets handle any external dependency
		di = m.normalize(di)

		// Exclude this CHI from monitoring
		//w.c.deleteWatch(chi.Namespace, chi.Name)

		// Delete all clusters
		di.Status.State = v1.StatusTerminating
		ctx := &memberContext{
			Namespace:    di.GetNamespace(),
			Installation: di.GetInstanceName(),
			Clusters:     di.Spec.Configuration.Clusters,
		}

		for name, cluster := range di.Spec.Configuration.Clusters {
			ctx.CurCluster = name
			if err := m.deleteCluster(ctx, di, &cluster); err != nil {
				return err
			}
		}

		// Delete ConfigMap(s)
		if err = m.deleteConfigMaps(ctx); err != nil {
			log.Error(err, "Delete ConfigMaps failed")
		}

		// Delete Service
		if err := m.deleteServiceForInstallation(ctx, di); err != nil {
			return err
		}

		// remove our finalizer from the list and update it.
		di.SetFinalizers(util.RemoveFromArray(DaisyFinalizer, di.GetFinalizers()))
		if err := m.deps.Client.Update(context.Background(), di); err != nil {
			return err
		}

		di.Status.State = v1.StatusCompleted
		if err := m.deps.Client.Status().Update(context.Background(), di); err != nil {
			log.Error(err, "Fail to update status")
			return err
		}
	}

	return nil
}

func (m *DaisyMemberManager) syncConfigMaps(di *v1.DaisyInstallation, update bool) error {
	// ConfigMap common for all resources in daisy installation
	// contains several sections, mapped as separated Config files,
	// such as remote servers, zookeeper setup, etc
	configMapCommon := m.getConfigMapCHICommon(di)
	if err := m.syncConfigMap(di, configMapCommon, update); err != nil {
		return err
	}

	// ConfigMap common for all users resources in CHI
	configMapUsers := m.getConfigMapCHICommonUsers(di)
	if err := m.syncConfigMap(di, configMapUsers, update); err != nil {
		return err
	}

	return nil
}

func (m *DaisyMemberManager) syncConfigMap(di *v1.DaisyInstallation, cm *corev1.ConfigMap, update bool) error {
	log := m.deps.Log

	log.V(2).Info("syncConfigMap() - start")
	defer log.V(2).Info("syncConfigMap() - end")

	// Check whether this object already exists in k8s
	var err error
	curConfigMap := corev1.ConfigMap{}
	if err = m.deps.Client.Get(context.Background(),
		client.ObjectKey{Namespace: cm.Namespace, Name: cm.Name},
		&curConfigMap); err != nil {

		if errors.IsNotFound(err) {
			// ConfigMap not found - even during Update process - try to create it
			if err = m.deps.Client.Create(context.Background(), cm); err != nil {
				m.deps.Recorder.Eventf(di, corev1.EventTypeWarning, "Created", "Create ConfigMap %q failed", cm.Name)
				log.Error(err, "Create ConfigMap failed", "ConfigMap", cm.Name)
			} else {
				m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Created", "Created ConfigMap %q", cm.Name)
				log.Info("Created ConfigMap", "ConfigMap", cm.Name)
			}
		} else {
			log.Error(err, "Sync ConfigMap failed", "ConfigMap", cm.Name)
			return err
		}
	}

	// We have ConfigMap - try to update it
	if !update {
		return err
	}

	if err = m.deps.Client.Update(context.Background(), cm); err != nil {
		m.deps.Recorder.Eventf(di, corev1.EventTypeWarning, "Update", "Update ConfigMap %q failed", cm.Name)
		log.Error(err, "Update ConfigMap failed", "ConfigMap", cm.Name)
	} else {
		m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Update", "Update ConfigMap %q", cm.Name)
		log.Info("Update ConfigMap", "ConfigMap", cm.Name)
	}

	return err
}

func (m *DaisyMemberManager) deleteConfigMaps(ctx *memberContext) error {
	var res []error

	commonKey := client.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      CreateConfigMapCommonName(ctx),
	}
	commonUserKey := client.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      CreateConfigMapCommonUsersName(ctx),
	}

	res = append(res, m.deleteConfigMap(commonKey))
	res = append(res, m.deleteConfigMap(commonUserKey))

	return errorutils.NewAggregate(res)
}

func (m *DaisyMemberManager) deleteConfigMap(key client.ObjectKey) error {
	log := m.deps.Log.WithValues("Action", "Delete")
	var err error

	cm := corev1.ConfigMap{}
	if err = m.deps.Client.Get(context.Background(), key, &cm); err == nil {
		if err = m.deps.Client.Delete(context.Background(), &cm); err == nil {
			log.Info("Delete ConfigMap", "ConfigMap", key.Name)
		} else {
			log.Error(err, "Delete ConfigMap failed", "ConfigMap", key.Name)
		}
	} else {
		if errors.IsNotFound(err) {
			log.Info("ConfigMap not found,  might be deleted already", "ConfigMap", key.Name)
			return nil
		}
	}
	return err
}

type getSvcFunc func() *corev1.Service

func (m *DaisyMemberManager) syncService(ctx *memberContext, di *v1.DaisyInstallation, getSvcFn getSvcFunc) error {
	ns := di.GetNamespace()
	diName := di.GetName()
	// sync service for daisy installation
	newSvc := getSvcFn()
	oldSvcTmp := &corev1.Service{}
	key := client.ObjectKey{
		Namespace: ns,
		Name:      newSvc.Name,
	}
	err := m.deps.Client.Get(context.Background(), key, oldSvcTmp)
	if errors.IsNotFound(err) {
		if err = SetServiceLastAppliedConfigAnnotation(newSvc); err != nil {
			return err
		}
		m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Created", "Created Service %q", newSvc.Name)
		return m.deps.Client.Create(context.Background(), newSvc)
	}

	if err != nil {
		return fmt.Errorf("syncService: failed to get svc %s for daisy installation %s/%s, error: %s",
			newSvc.Name, ns, diName, err)
	}

	oldSvc := oldSvcTmp.DeepCopy()

	equal, err := ServiceEqual(newSvc, oldSvc)
	if err != nil {
		return err
	}
	if !equal {
		svc := *oldSvc
		svc.Spec = newSvc.Spec
		// TODO add unit test
		err = SetServiceLastAppliedConfigAnnotation(&svc)
		if err != nil {
			return err
		}
		svc.Spec.ClusterIP = oldSvc.Spec.ClusterIP
		err = m.deps.Client.Update(context.Background(), &svc)
		return err
	}

	return nil
}

func (m *DaisyMemberManager) syncServiceForReplica(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) error {
	return m.syncService(ctx, di, func() *corev1.Service {
		return m.getNewReplicaService(ctx, di, replica)
	})
}

func (m *DaisyMemberManager) syncServiceForInstallation(ctx *memberContext, di *v1.DaisyInstallation) error {
	return m.syncService(ctx, di, func() *corev1.Service {
		return m.getNewInstallationService(di)
	})
}

// CreateServiceCHI creates new corev1.Service for specified CHI
func (m *DaisyMemberManager) getNewInstallationService(di *v1.DaisyInstallation) *corev1.Service {
	serviceName := CreateInstallationServiceName(di)
	log := m.deps.Log.WithValues("Service", serviceName)

	log.Info("get new installation Service")

	// Incorrect/unknown .templates.ServiceTemplate specified
	// Create default Service
	svcLabel := label.New().Instance(di.GetInstanceName())
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            serviceName,
			Namespace:       di.Namespace,
			Labels:          svcLabel.Labels(),
			OwnerReferences: []metav1.OwnerReference{GetOwnerRef(di)},
		},
		Spec: corev1.ServiceSpec{
			// ClusterIP: templateDefaultsServiceClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       daisyDefaultHTTPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultHTTPPortNumber,
					TargetPort: intstr.FromString(daisyDefaultHTTPPortName),
				},
				{
					Name:       daisyDefaultTCPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultTCPPortNumber,
					TargetPort: intstr.FromString(daisyDefaultTCPPortName),
				},
			},
			Selector: svcLabel.Labels(),
			Type:     corev1.ServiceTypeNodePort,
			//ExternalTrafficPolicy: corev1.ServiceExteTrafficPolicyTypeLocal,
		},
	}
}

// getNewReplicaService get a new corev1.Service for specified replica
func (m *DaisyMemberManager) getNewReplicaService(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) *corev1.Service {
	serviceName := CreateStatefulSetServiceName(replica)

	log := m.deps.Log.WithValues("replica", replica.Name)
	log.V(1).Info("get service for StatefulSet")

	// Incorrect/unknown .templates.ServiceTemplate specified
	// Create default Service
	svcSelect := label.New().Instance(ctx.Installation).Cluster(ctx.CurCluster).ShardName(ctx.CurShard).ReplicaName(replica.Name)
	svcLabel := svcSelect.Copy().Service("replica")
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            serviceName,
			Namespace:       ctx.Namespace,
			Labels:          svcLabel.Labels(),
			OwnerReferences: []metav1.OwnerReference{GetOwnerRef(di)},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       daisyDefaultTCPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultTCPPortNumber,
					TargetPort: intstr.FromString(daisyDefaultTCPPortName),
				},
				{
					Name:       daisyDefaultHTTPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultHTTPPortNumber,
					TargetPort: intstr.FromString(daisyDefaultHTTPPortName),
				},
				{
					Name:       daisyDefaultInterserverHTTPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultInterserverHTTPPortNumber,
					TargetPort: intstr.FromString(daisyDefaultInterserverHTTPPortName),
				},
			},
			Selector:                 svcSelect.Labels(),
			ClusterIP:                templateDefaultsServiceClusterIP,
			Type:                     "ClusterIP",
			PublishNotReadyAddresses: true,
		},
	}

}

// normalize method contruct a ClickHouseInstallation regarding the spec
func (m *DaisyMemberManager) normalize(di *v1.DaisyInstallation) *v1.DaisyInstallation {
	ns := di.GetNamespace()
	diName := di.GetName()

	log := m.deps.Log.WithValues("Namespace", ns, "Installation", diName)
	log.V(3).Info("normalize() - start")
	defer log.V(3).Info("normalize() - end")

	var withDefaultCluster bool

	// TODO: to simplify the logic
	if di == nil {
		di = &v1.DaisyInstallation{}
		withDefaultCluster = false
	} else {
		withDefaultCluster = true
	}

	di, err := m.normalizer.GetInstallationFromTemplate(di, withDefaultCluster)
	if err != nil {
		//TODO: record event for the failure
		log.Error(err, "FAILED to normalize DaisyInstallation")
	}

	return di
}

func (m *DaisyMemberManager) syncConfiguration(di *v1.DaisyInstallation, cfg *v1.Configuration) error {
	//TODO: add logic for Zookeeper, Users, Settings, Files etal
	ctx := &memberContext{
		Namespace:    di.GetNamespace(),
		Installation: di.GetInstanceName(),
		Clusters:     di.Spec.Configuration.Clusters,
	}

	for name, cluster := range cfg.Clusters {
		ctx.CurCluster = name
		if err := m.syncCluster(ctx, di, &cluster); err != nil {
			return err
		}
	}
	return nil
}

func (m *DaisyMemberManager) syncCluster(ctx *memberContext, di *v1.DaisyInstallation, cluster *v1.Cluster) error {
	log := m.deps.Log.WithValues("Cluster", ctx.CurCluster)

	if di.Status.Clusters == nil {
		di.Status.Clusters = map[string]v1.ClusterStatus{}
	}

	if _, ok := di.Status.Clusters[cluster.Name]; !ok {
		di.Status.Clusters[cluster.Name] = v1.ClusterStatus{
			Shards: map[string]v1.ShardStatus{},
		}
	}

	curNum := len(di.Status.Clusters[ctx.CurCluster].Shards)
	if curNum > len(cluster.Layout.Shards) {
		log.Info("Need to delete extra shards", "number", curNum-len(cluster.Layout.Shards))
	}

	for name, shard := range cluster.Layout.Shards {
		if err := m.syncShard(ctx, di, &shard); err != nil {
			if !IsRequeueError(err) {
				log.Error(err, "sync shard %s fail", "Shard", name)
			}
			return err
		}
	}
	return nil
}

func (m *DaisyMemberManager) syncShard(ctx *memberContext, di *v1.DaisyInstallation, shard *v1.Shard) error {
	ctx.CurShard = shard.Name
	log := m.deps.Log.WithValues("Shard", shard.Name)

	clusterStatus := di.Status.Clusters[ctx.CurCluster]
	if clusterStatus.Shards == nil {
		clusterStatus.Shards = make(map[string]v1.ShardStatus)
	}

	newStatus := v1.ShardStatus{
		Name:     shard.Name,
		Phase:    v1.ScalePhase,
		Replicas: map[string]v1.ReplicaStatus{},
	}

	if _, ok := clusterStatus.Shards[shard.Name]; ok {
		newStatus = clusterStatus.Shards[shard.Name]
		if len(newStatus.Replicas) != shard.ReplicaCount {
			newStatus.Phase = v1.ScalePhase
		} else {
			newStatus.Phase = v1.NormalPhase
			newStatus.Health = true
		}
	}
	clusterStatus.Shards[shard.Name] = newStatus

	di.Status.Clusters[ctx.CurCluster] = clusterStatus

	var err error
	for name, replica := range shard.Replicas {
		if err = m.syncReplica(ctx, di, &replica); err != nil {
			if !IsRequeueError(err) {
				log.Error(err, "sync replica %s fail, error %v", "replica", name)
			} else {
				log.Info("wait for replica running", "replica", name)
			}
			return err
		}
	}

	// delete extra replicas
	oldReplicas := di.Status.PrevSpec.Configuration.Clusters[ctx.CurCluster].Layout.Shards[ctx.CurShard].Replicas
	if len(oldReplicas) > len(shard.Replicas) {
		newReplicas := sets.NewString()
		for r := range shard.Replicas {
			newReplicas.Insert(r)
		}

		for o, old := range oldReplicas {
			if !newReplicas.Has(o) {
				m.deleteReplica(ctx, di, &old)
			}
		}
	}

	return nil
}

func (m *DaisyMemberManager) syncReplica(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) error {
	ctx.CurReplica = replica.Name
	if di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas == nil {
		shardStatus := di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard]
		shardStatus.Replicas = map[string]v1.ReplicaStatus{}
		di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard] = shardStatus
	}

	cm := m.getConfigMapReplica(ctx, di, replica)
	if err := m.syncConfigMap(di, cm, true); err != nil {
		return err
	}

	if err := m.syncStatefulSetForDaisyCluster(ctx, di, replica); err != nil {
		return err
	}

	//TODO: sync PVs

	return m.syncServiceForReplica(ctx, di, replica)
}

func (m *DaisyMemberManager) syncStatefulSetForDaisyCluster(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) error {
	log := m.deps.Log.WithValues("replica", replica.Name)

	ns := di.GetNamespace()
	dcName := di.GetName()
	key := client.ObjectKey{
		Namespace: ns,
		Name:      replica.Name,
	}
	oldSetTmp := &apps.StatefulSet{}
	var oldSet *apps.StatefulSet = nil
	err := m.deps.Client.Get(context.Background(), key, oldSetTmp)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("syncStatefulSetForDaisyCluster: failed to get sts %s for replica %s/%s, error: %s", replica.Name, ns, dcName, err)
	}
	setNotExist := errors.IsNotFound(err)

	if !setNotExist {
		oldSet = oldSetTmp.DeepCopy()
	}

	// TODO: enhance the status update here
	if err := m.syncDaisyClusterStatus(ctx, di, oldSet); err != nil {
		return err
	}

	if di.Spec.Paused {
		log.V(4).Info("daisy cluster is paused, skip syncing for daisy statefulset")
		return nil
	}

	newSet, err := getNewSetForDaisyCluster(ctx, di, replica)
	if err != nil {
		return err
	}
	if setNotExist {
		err = SetStatefulSetLastAppliedConfigAnnotation(newSet)
		if err != nil {
			return err
		}
		if err = m.deps.Client.Create(context.Background(), newSet); err != nil {
			return err
		}
		replicaStatus := v1.ReplicaStatus{}
		replicaStatus.StatefulSet = &apps.StatefulSetStatus{}
		shardStatus := di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard]
		if shardStatus.Replicas == nil {
			//unusual case
			shardStatus.Replicas = map[string]v1.ReplicaStatus{}
		}
		shardStatus.Replicas[replica.Name] = replicaStatus
		di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard] = shardStatus
		di.Status.AddedReplicasCount++
		m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Created", "Created StatefulSet %q", replica.Name)
		log.Info("Create Sts for replica")
		return RequeueErrorf("DaisyInstallation: [%s/%s], waiting for replica [%s] running", ns, dcName, replica.Name)
	}

	//if _, err := m.setStoreLabelsForTiKV(di); err != nil {
	//	return err
	//}

	// Scaling takes precedence over upgrading because:
	// - if a store fails in the upgrading, users may want to delete it or add
	//   new replicas
	// - it's ok to scale in the middle of upgrading (in statefulset controller
	//   scaling takes precedence over upgrading too)
	//if err := m.scaler.Scale(di, oldSet, newSet); err != nil {
	//	return err
	//}

	// Perform failover logic if necessary. Note that this will only update
	// TidbCluster status. The actual scaling performs in next sync loop (if a
	// new replica needs to be added).
	//if m.deps.CLIConfig.AutoFailover && di.Spec.TiKV.MaxFailoverCount != nil {
	//	if di.AllPodsStarted() && !di.AllStoresReady() {
	//		if err := m.failover.Failover(di); err != nil {
	//			return err
	//		}
	//	}
	//}

	//if !templateEqual(newSet, oldSet) || di.Status.Phase == v1.UpgradePhase {
	//	if err := m.upgrader.Upgrade(di, oldSet, newSet); err != nil {
	//		return err
	//	}
	//}

	// Update StatefulSet
	if !StatefulSetEqual(newSet, oldSet) {
		if err = UpdateStatefulSet(m.deps.Client, di, newSet, oldSet); err != nil {
			log.Error(err, "Update StatefulSet fail")
		}
		di.Status.UpdatedReplicasCount++
	} else {
		newSet = oldSet
	}

	// Update Status of StatefulSet
	if hasStatefulSetReachedGeneration(newSet) {
		di.Status.ReadyReplicas++
	}

	return err
}

func (m *DaisyMemberManager) syncDaisyClusterStatus(ctx *memberContext, di *v1.DaisyInstallation, set *apps.StatefulSet) error {
	//TODO add logic to update Daisy installation status
	if set == nil {
		// skip if not created yet
		return nil
	}
	replicaStatus := di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas[ctx.CurReplica]
	replicaStatus.StatefulSet = &set.Status
	replicaStatus.LastTransitionTime = metav1.Now()
	upgrading, err := m.statefulSetIsUpgradingFn(m.deps.Client, set, di)
	if err != nil {
		return err
	}
	// Scaling takes precedence over upgrading.
	if *set.Spec.Replicas != 1 {
		replicaStatus.Phase = v1.ScalePhase
	} else if upgrading {
		replicaStatus.Phase = v1.UpgradePhase
	} else {
		replicaStatus.Phase = v1.NormalPhase
	}

	di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas[ctx.CurReplica] = replicaStatus
	return nil
}

func (m *DaisyMemberManager) deleteServiceForInstallation(ctx *memberContext, di *v1.DaisyInstallation) error {
	log := m.deps.Log

	svcName := CreateInstallationServiceName(di)
	log.V(1).Info("Delete Service for replica", "Service", svcName)
	key := client.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      svcName,
	}
	m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Deleting", "Delete Installation Service %q", key.Name)
	return m.deleteService(key)
}

func (m *DaisyMemberManager) deleteService(key client.ObjectKey) error {
	log := m.deps.Log
	svc := &corev1.Service{}
	var err error
	if err = m.deps.Client.Get(context.Background(), key, svc); err == nil {
		return m.deps.Client.Delete(context.Background(), svc)
	}

	if errors.IsNotFound(err) {
		log.Info("Delete service completed, Service not found - already deleted", "error", err)
		return nil
	} else {
		log.V(1).Error(err, "error get Service")
		return err
	}
}

func (m *DaisyMemberManager) deleteCluster(ctx *memberContext, di *v1.DaisyInstallation, cluster *v1.Cluster) error {
	// Delete all shards
	log := m.deps.Log.WithValues("Cluster", cluster.Name, "Action", "Delete")

	for name, shard := range cluster.Layout.Shards {
		if err := m.deleteShard(ctx, di, &shard); err != nil {
			log.Error(err, "delete shard fail", "Shard", name)
			return err
		}
	}

	// Delete Cluster Service
	//_ = w.c.deleteServiceCluster(cluster)
	return nil
}

func (m *DaisyMemberManager) deleteShard(ctx *memberContext, di *v1.DaisyInstallation, shard *v1.Shard) error {
	ctx.CurShard = shard.Name
	log := m.deps.Log.WithValues("Shard", shard.Name)

	clusterStatus, ok := di.Status.Clusters[ctx.CurCluster]
	if ok {
		if clusterStatus.Shards == nil {
			clusterStatus.Shards = make(map[string]v1.ShardStatus)
		}

		newStatus := v1.ShardStatus{
			Name:     shard.Name,
			Phase:    v1.ScalePhase,
			Replicas: map[string]v1.ReplicaStatus{},
		}

		if _, ok := clusterStatus.Shards[shard.Name]; ok {
			newStatus = clusterStatus.Shards[shard.Name]
			if len(newStatus.Replicas) != shard.ReplicaCount {
				newStatus.Phase = v1.ScalePhase
			}
		}
		clusterStatus.Shards[shard.Name] = newStatus

		di.Status.Clusters[ctx.CurCluster] = clusterStatus
	}

	for name, replica := range shard.Replicas {
		if err := m.deleteReplica(ctx, di, &replica); err != nil {
			log.Error(err, "delete replica fail", "replica", name)
			return err
		}
	}
	return nil
}

func (m *DaisyMemberManager) deleteReplica(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) error {
	ctx.CurReplica = r.Name
	log := m.deps.Log.WithValues("replica", r.Name, "Action", "Delete")

	set := &apps.StatefulSet{}
	key := client.ObjectKey{
		Name:      r.Name,
		Namespace: ctx.Namespace,
	}
	if err := m.deps.Client.Get(context.Background(), key, set); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Delete replica completed StatefulSet not found - already deleted?")
			return nil
		}
		return err
	}

	// Each host consists of
	// 1. User-level objects - tables on the host
	//    We need to delete tables on the host in order to clean Zookeeper data.
	//    If just delete tables, Zookeeper will still keep track of non-existent tables
	// 2. Kubernetes-level objects - such as StatefulSet, PVC(s), ConfigMap(s), Service(s)
	// Need to delete all these items

	// Each host consists of
	// 1. Tables on host - we need to delete tables on the host in order to clean Zookeeper data
	// 2. StatefulSet
	// 3. PersistentVolumeClaim
	// 4. ConfigMap
	// 5. Service
	// Need to delete all these item

	var err error
	err = m.deleteTables(ctx, r)
	if err := m.deleteStatefulSetForReplica(ctx, di, r); err != nil {
		return err
	}
	di.Status.DeletedReplicasCount++

	//TODO: delete PVC
	//_ = c.deletePVC(host)

	cmKey := client.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      CreateConfigMapPodName(ctx),
	}

	if err = m.deleteConfigMap(cmKey); err != nil {
		return err
	}

	err = m.deleteServiceForReplica(ctx, di, r)

	// When deleting the whole CHI (not particular replica), daisy installation may already be unavailable, so update CHI tolerantly
	if err == nil {
		err = m.deps.Client.Status().Update(context.Background(), di)
		log.Info("Delete Replica completed")
	} else {
		log.Error(err, "Fail to Delete Replica")
	}

	return err
}

func (m *DaisyMemberManager) deleteStatefulSetForReplica(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) error {
	log := m.deps.Log.WithValues("replica", ctx.CurReplica)
	// IMPORTANT
	// StatefulSets do not provide any guarantees on the termination of pods when a StatefulSet is deleted.
	// To achieve ordered and graceful termination of the pods in the StatefulSet,
	// it is possible to scale the StatefulSet down to 0 prior to deletion.
	set := &apps.StatefulSet{}
	key := client.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      r.Name,
	}
	var err error
	if err = m.deps.Client.Get(context.Background(), key, set); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Delete replica completed, StatefulSet not found", "error", err)
			return nil
		} else {
			log.Error(err, "error get StatefulSet")
			return err
		}
	}

	m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Deleted", "Delete Replica %q", r.Name)
	return m.deps.Client.Delete(context.Background(), set)
}

func (m *DaisyMemberManager) deleteServiceForReplica(ctx *memberContext, di *v1.DaisyInstallation, r *v1.Replica) error {
	log := m.deps.Log.WithValues("replica", ctx.CurReplica)

	log.V(1).Info("Delete Service for replica")
	key := client.ObjectKey{
		Namespace: ctx.Namespace,
		Name:      r.Name,
	}
	m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Deleting", "Delete Replica Service %q", key.Name)
	return m.deleteService(key)
}

func (m *DaisyMemberManager) deleteTables(ctx *memberContext, r *v1.Replica) error {
	m.deps.Log.V(2).Info("Delete Tables for Replica", "replica", ctx.CurReplica)
	return nil
}

func (m *DaisyMemberManager) IsInitialized(di *v1.DaisyInstallation) bool {
	if di == nil {
		return false
	}
	return true
}

func (m *DaisyMemberManager) IsReady(di *v1.DaisyInstallation) bool {
	if di == nil {
		return false
	}

	//TODO: wait for all sub resource ready, then set State to StatusCompleted
	if di.Status.ReplicasCount == di.Status.ReadyReplicas {
		return true
	}
	return false
}

func daisyStatefulSetIsUpgrading(client.Client, *apps.StatefulSet, *v1.DaisyInstallation) (bool, error) {
	return false, nil
}

type FakeDaisyMemberManager struct {
	err error
}

func NewFakeDaisyMemberManager() *FakeDaisyMemberManager {
	return &FakeDaisyMemberManager{}
}

func (m *FakeDaisyMemberManager) SetSyncError(err error) {
	m.err = err
}

func (m *FakeDaisyMemberManager) Sync(di *v1.DaisyInstallation) error {
	if m.err != nil {
		return m.err
	}
	//if len(dc.Status.TiKV.Stores) != 0 {
	//	// simulate status update
	//	tc.Status.ClusterID = string(uuid.NewUUID())
	//}
	return nil
}

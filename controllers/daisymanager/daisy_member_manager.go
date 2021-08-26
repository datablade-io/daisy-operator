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

package daisymanager

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

	"github.com/daisy/daisy-operator/api/v1"
	"github.com/daisy/daisy-operator/pkg/k8s"
	"github.com/daisy/daisy-operator/pkg/label"
	"github.com/daisy/daisy-operator/pkg/util"
)

const (
	ActionAdd    SyncAction = "Add"
	ActionUpdate SyncAction = "Update"
	ActionDelete SyncAction = "Delete"
	ActionNone   SyncAction = "None"
)

type SyncAction string

type syncResult struct {
	Action SyncAction
}

// Manager implements the logic for syncing replicas and shards of daisycluster.
type Manager interface {
	// Sync	implements the logic for syncing daisycluster.
	Sync(cur *v1.DaisyInstallation) error
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
	schemer    *Schemer
	//failover                 Failover
	//scaler                   Scaler
	//upgrader                 Upgrader
	statefulSetIsUpgradingFn func(client.Client, *apps.StatefulSet, *v1.DaisyInstallation) (bool, error)
}

// NewTiKVMemberManager returns a *tikvMemberManager
func NewDaisyMemberManager(deps *Dependencies, cfgMgr *ConfigManager) (Manager, error) {
	//cfgMgr, err := NewConfigManager(deps.Client, initConfigFilePath)
	m := &DaisyMemberManager{
		deps:       deps,
		normalizer: NewNormalizer(cfgMgr),
		cfgMgr:     cfgMgr,
		cfgGen:     nil,
		schemer: NewSchemer(
			cfgMgr.Config().CHUsername,
			cfgMgr.Config().CHPassword,
			cfgMgr.Config().CHPort,
			deps.Log.WithName("schemer")),
		//failover: failover,
		//scaler:   scaler,
		//upgrader: upgrader,
	}
	m.statefulSetIsUpgradingFn = daisyStatefulSetIsUpgrading
	return m, nil
}

// Sync fulfills the manager.Manager interface
func (m *DaisyMemberManager) Sync(cur *v1.DaisyInstallation) error {
	ns := cur.GetNamespace()
	dcName := cur.GetName()

	log := m.deps.Log.WithName("DaisyMemeberManager").WithValues("Namespace", ns, "Installation", dcName)
	log.V(2).Info("Sync Start")
	defer log.V(2).Info("Sync End")

	if cur == nil {
		return nil
	}

	oldStatus := cur.Status.DeepCopy()
	// examine DeletionTimestamp to determine if object is under deletion
	if !cur.ObjectMeta.DeletionTimestamp.IsZero() {
		return m.deleteDaisyInstallation(cur)
	} else {
		m.deps.Log.V(3).Info("Registering finalizer for Daisy Installation")

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

	curNorm := m.normalize(cur)
	forCfg := cur.DeepCopy()
	forCfg.Spec = curNorm.Spec
	// TODO: refact ConfigGenerator to use Spec instead of DaisyInstallation
	m.cfgGen = NewConfigSectionsGenerator(NewConfigGenerator(forCfg), m.cfgMgr.Config())

	// Handle create & update scenarios
	ctx := &memberContext{
		Namespace:    ns,
		Installation: dcName,
		Normalized:   *curNorm,
		owner:        k8s.GetOwnerRef(cur),
		Config:       m.cfgMgr.Config(),
	}

	fillStatus(ctx, cur)
	cur.Status.Reset()
	if err = m.syncServiceForInstallation(ctx, cur); err != nil {
		log.Error(err, "Sync configmaps fail")
		return err
	}

	if err = m.syncConfigMaps(cur, false); err != nil {
		log.Error(err, "sync configmaps fail in first time")
		return err
	}

	if err = m.syncConfiguration(ctx, cur); err != nil {
		if !k8s.IsRequeueError(err) {
			log.Error(err, "Sync configuration fail")
		}
		return err
	}

	if err = k8s.SetInstallationLastAppliedConfigAnnotation(cur, &curNorm.Spec); err != nil {
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

	if !apiequality.Semantic.DeepEqual(oldStatus, cur.Status) {
		// after update the installation, the spec will be changed by Client, therefore backup the spec first
		status := cur.Status.DeepCopy()
		if err = m.deps.Client.Update(context.Background(), cur); err != nil {
			return err
		}
		log.Info("Update Status", "State", status.State)
		cur.Status = *status
		if err = UpdateInstallationStatus(m.deps.Client, cur, true); err != nil {
			return err
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
		norm := m.normalize(di)

		// Exclude this CHI from monitoring
		//w.c.deleteWatch(chi.Namespace, chi.Name)

		// Delete all clusters
		di.Status.State = v1.StatusTerminating
		ctx := &memberContext{
			Namespace:    di.GetNamespace(),
			Installation: di.GetInstanceName(),
			Clusters:     di.Spec.Configuration.Clusters,
			Normalized:   *norm,
		}

		for name, cluster := range norm.Spec.Configuration.Clusters {
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

		di.Status.State = v1.StatusCompleted
		if err := UpdateInstallationStatus(m.deps.Client, di, true); err != nil {
			if errors.IsConflict(err) {
				log.V(1).Info("Conflict while updating status")
				return k8s.RequeueErrorf("Conflict while updating status")
			} else {
				log.Error(err, "Fail to update status")
				return err
			}
		}

		// remove our finalizer from the list and update it.
		di.SetFinalizers(util.RemoveFromArray(DaisyFinalizer, di.GetFinalizers()))
		if err := m.deps.Client.Update(context.Background(), di); err != nil {
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
	ns := ctx.Namespace
	diName := ctx.Installation
	// sync service for daisy installation
	newSvc := getSvcFn()
	oldSvcTmp := &corev1.Service{}
	key := client.ObjectKey{
		Namespace: ns,
		Name:      newSvc.Name,
	}
	err := m.deps.Client.Get(context.Background(), key, oldSvcTmp)
	if errors.IsNotFound(err) {
		if err = k8s.SetServiceLastAppliedConfigAnnotation(newSvc); err != nil {
			return err
		}
		m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Created", "Created Service %q", newSvc.Name)
		newSvc.Labels["daisy.com/sync-state"] = "Synced"
		return m.deps.Client.Create(context.Background(), newSvc)
	}

	if err != nil {
		return fmt.Errorf("syncService: failed to get svc %s for daisy installation %s/%s, error: %s",
			newSvc.Name, ns, diName, err)
	}

	oldSvc := oldSvcTmp.DeepCopy()

	equal, err := k8s.ServiceEqual(newSvc, oldSvc)
	if err != nil {
		return err
	}
	if !equal {
		svc := *oldSvc
		svc.Spec = newSvc.Spec
		err = k8s.SetServiceLastAppliedConfigAnnotation(&svc)
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
			OwnerReferences: []metav1.OwnerReference{k8s.GetOwnerRef(di)},
		},
		Spec: corev1.ServiceSpec{
			// ClusterIP: templateDefaultsServiceClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       daisyDefaultHTTPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultHTTPPortNumber,
					TargetPort: intstr.FromInt(int(daisyDefaultHTTPPortNumber)),
				},
				{
					Name:       daisyDefaultTCPPortName,
					Protocol:   corev1.ProtocolTCP,
					Port:       daisyDefaultTCPPortNumber,
					TargetPort: intstr.FromInt(int(daisyDefaultTCPPortNumber)),
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
			OwnerReferences: []metav1.OwnerReference{k8s.GetOwnerRef(di)},
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
func (m *DaisyMemberManager) normalize(di *v1.DaisyInstallation) *NormalizedSpec {
	tmp := di.DeepCopy()
	ns := di.GetNamespace()
	diName := di.GetName()

	log := m.deps.Log.WithValues("Namespace", ns, "Installation", diName)
	log.V(3).Info("normalize() - start")
	defer log.V(3).Info("normalize() - end")

	var withDefaultCluster bool
	if di == nil {
		di = &v1.DaisyInstallation{}
		withDefaultCluster = false
	} else {
		withDefaultCluster = true
	}

	norm, err := m.normalizer.GetInstallationFromTemplate(tmp, withDefaultCluster)
	if err != nil {
		//TODO: record event for the failure
		log.Error(err, "FAILED to normalize DaisyInstallation")
	}

	return norm
}

func (m *DaisyMemberManager) syncConfiguration(ctx *memberContext, di *v1.DaisyInstallation) error {
	cfg := ctx.Normalized.Spec.Configuration
	ctx.Clusters = cfg.Clusters

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
			if !k8s.IsRequeueError(err) {
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
		if _, err = m.syncReplica(ctx, di, &replica); err != nil {
			if !k8s.IsRequeueError(err) {
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
				if err = m.deleteReplica(ctx, di, &old); err != nil {
					log.Error(err, "Delete replica fail", "Replica", old.Name)
					return err
				}
			}
		}
	}

	return nil
}

func (m *DaisyMemberManager) syncReplica(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) (*syncResult, error) {
	log := m.deps.Log.WithValues("Replica", replica.Name)
	stsSyncResult := &syncResult{Action: ActionNone}
	var err error

	ctx.CurReplica = replica.Name
	if di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas == nil {
		shardStatus := di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard]
		shardStatus.Replicas = map[string]v1.ReplicaStatus{}
		di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard] = shardStatus
	}

	cm := m.getConfigMapReplica(ctx, di, replica)
	if err = m.syncConfigMap(di, cm, true); err != nil {
		return stsSyncResult, err
	}

	if stsSyncResult, err = m.syncStatefulSet(ctx, di, replica); err != nil {
		return stsSyncResult, err
	}

	if err := m.syncServiceForReplica(ctx, di, replica); err != nil {
		return nil, err
	}

	if replica.IsReady(ctx.CurCluster, ctx.CurShard, di) &&
		!replica.IsSync(ctx.CurCluster, ctx.CurShard, di) {
		// TODO: in future use Job to sync tables to avoid block sync loop of controller
		if err := m.schemer.CreateTablesForReplica(ctx, di, replica); err != nil {
			log.Error(err, "Create Tables for Replica failed")
			return nil, err
		}
		status := di.Status.GetReplicaStatus(replica)
		status.State = v1.Sync
		di.Status.SetReplicaStatus(ctx.CurCluster, ctx.CurShard, replica.Name, *status)
		di.Status.ReadyReplicas++
	} else {
		log.Info("some other fork syncReplica")
	}

	return nil, nil
}

func (m *DaisyMemberManager) syncStatefulSet(ctx *memberContext, di *v1.DaisyInstallation, replica *v1.Replica) (*syncResult, error) {
	log := m.deps.Log.WithValues("replica", replica.Name)
	result := &syncResult{Action: ActionNone}

	ns := ctx.Namespace
	dcName := ctx.Installation
	key := client.ObjectKey{
		Namespace: ns,
		Name:      replica.Name,
	}
	oldSetTmp := &apps.StatefulSet{}
	var oldSet *apps.StatefulSet = nil

	//TODO: Check actual statefulsets match expectations before applying any changes
	err := m.deps.Client.Get(context.Background(), key, oldSetTmp)
	if err != nil && !errors.IsNotFound(err) {
		return result, fmt.Errorf("syncStatefulSet: failed to get sts %s for replica %s/%s, error: %s", replica.Name, ns, dcName, err)
	}
	setNotExist := errors.IsNotFound(err)

	if !setNotExist {
		oldSet = oldSetTmp.DeepCopy()
	}

	if di.Spec.Paused {
		log.V(4).Info("daisy cluster is paused, skip syncing for daisy statefulset")
		return result, nil
	}

	newSet, err := getStatefulSetForReplica(ctx, di, replica)
	if di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas[replica.Name].State == "Sync" {
		//copy := oldSet.DeepCopy()
		//copy.Labels["daisy.com/sync-state"] = "Synced"
		//copy.Spec.Template.Labels["daisy.com/sync-state"]  = "Synced"
		//m.deps.Client.Update(context.Background(), copy)
		podName := replica.Name + "-0"
		var pod = corev1.Pod{}
		m.deps.Client.Get(context.Background(), client.ObjectKey{
			Namespace: ns,
			Name:      podName,
		}, &pod)
		if pod.Labels != nil && pod.Labels["daisy.com/sync-state"] != "Synced" {
			pod.Labels["daisy.com/sync-state"] = "Synced"
			m.deps.Client.Update(context.Background(), &pod)
		}

	}
	if err != nil {
		return result, err
	}
	if setNotExist {
		result.Action = ActionAdd
		err = k8s.SetStatefulSetLastAppliedConfigAnnotation(newSet)
		if err != nil {
			return result, err
		}
		//newSet.Spec.Template.ObjectMeta.Labels["daisy.com/sync-state"] = "NotSync"
		if err = m.deps.Client.Create(context.Background(), newSet); err != nil {
			return result, err
		}
		m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Created", "Created StatefulSet %q", replica.Name)
		log.Info("Create Sts for replica")
		if err = m.syncRepliaStatus(ctx, di, newSet, result); err != nil {
			log.Error(err, "Update status fail after create Sts for replica")
		}
		return result, k8s.RequeueErrorf("DaisyInstallation: [%s/%s], waiting for replica [%s] running", ns, dcName, replica.Name)
	}

	// Update StatefulSet
	var recreate bool
	if recreate, err = m.handleVolumeExpansion(di, newSet, oldSet, false); err != nil {
		log.Error(err, "handle volume expansion fail")
		return result, err
	}

	if recreate {
		//TODO: support recreation in future, now revert the changed PVCs
		//result.Action = ActionUpdate
		//return result, k8s.RequeueErrorf("DaisyInstallation: [%s/%s], waiting for pvc expansion", ns, dcName, replica.Name)
		log.V(2).Info("Ignore the VolumeClaimTemplates changes", "StatefulSet", newSet.Name)
	}

	// ignore Changes in PVC
	if !k8s.StatefulSetEqual(newSet, oldSet) {
		result.Action = ActionUpdate
		if err = UpdateStatefulSet(m.deps.Client, di, newSet, oldSet); err != nil {
			log.Error(err, "Update StatefulSet fail")
		}
	} else {
		newSet = oldSet
	}

	// Update Status of StatefulSet
	err = m.syncRepliaStatus(ctx, di, newSet, result)

	return result, err
}

func getPreReplicaStatus(di *v1.DaisyInstallation, name string) string {
	if name == "" {
		return ""
	}
	return di.Status.Clusters["cluster"].Shards[getClusterName(name)].Replicas[name].State
}

func getPreReplicatName(name string) string {
	nameSlice := strings.Split(name, "-")
	clusterName := getClusterName(name)
	replicaNum, _ := strconv.Atoi(nameSlice[3])
	if replicaNum == 0 {
		return ""
	}
	replicaNum--
	return fmt.Sprintf("%s-%d", clusterName, replicaNum)
}

func getClusterName(name string) string {
	nameSlice := strings.Split(name, "-")
	clusterName := nameSlice[0] + "-" + nameSlice[1] + "-" + nameSlice[2]
	return clusterName
}

func (m *DaisyMemberManager) handleVolumeExpansion(di *v1.DaisyInstallation, cur, old *apps.StatefulSet, validateStorageClass bool) (bool, error) {
	// ensure there are no incompatible storage size modification
	if err := ValidateClaimsStorageUpdate(
		m.deps.Client,
		old.Spec.VolumeClaimTemplates,
		cur.Spec.VolumeClaimTemplates,
		validateStorageClass); err != nil {
		return false, err
	}

	// resize all PVCs that can be resized
	err := resizePVCs(m.deps.Client, *cur, *old)
	if err != nil {
		return false, err
	}

	// schedule the StatefulSet for recreation if needed
	if needsRecreate(*cur, *old) {
		//TODO: re-create sts and restart pod in future, currently do not recreate sts
		// a. (FileSystem based PV) When PV has been resized, the statefulset needs to be recreated to reflect the PVC changes
		// b. (FileSystem based PV) To ensure the resized PV take effects, Pods of Sts need to be restarted
		return true, annotateForRecreation(m.deps.Client, di, *old, cur.Spec.VolumeClaimTemplates)
	}

	return false, nil
}

// needsRecreate returns true if the StatefulSet needs to be re-created to account for volume expansion.
func needsRecreate(expectedSset apps.StatefulSet, actualSset apps.StatefulSet) bool {
	for _, expectedClaim := range expectedSset.Spec.VolumeClaimTemplates {
		actualClaim := k8s.GetClaim(actualSset.Spec.VolumeClaimTemplates, expectedClaim.Name)
		if actualClaim == nil {
			continue
		}
		storageCmp := k8s.CompareStorageRequests(actualClaim.Spec.Resources, expectedClaim.Spec.Resources)
		if storageCmp.Increase {
			return true
		}
	}
	return false
}

// resizePVCs updates the spec of all existing PVCs whose storage requests can be expanded,
// according to their storage class and what's specified in the expected claim.
// It returns an error if the requested storage size is incompatible with the PVC.
func resizePVCs(
	k8sClient client.Client,
	expectedSset apps.StatefulSet,
	actualSset apps.StatefulSet,
) error {
	// match each existing PVC with an expected claim, and decide whether the PVC should be resized
	actualPVCs, err := k8s.RetrieveActualPVCs(k8sClient, actualSset)
	if err != nil {
		return err
	}
	for claimName, pvcs := range actualPVCs {
		expectedClaim := k8s.GetClaim(expectedSset.Spec.VolumeClaimTemplates, claimName)
		if expectedClaim == nil {
			continue
		}
		for _, pvc := range pvcs {
			storageCmp := k8s.CompareStorageRequests(pvc.Spec.Resources, expectedClaim.Spec.Resources)
			if !storageCmp.Increase {
				// not an increase, nothing to do
				continue
			}

			newSize := expectedClaim.Spec.Resources.Requests.Storage()
			log.Info("Resizing PVC storage requests. Depending on the volume provisioner, "+
				"Pods may need to be manually deleted for the filesystem to be resized.",
				"namespace", pvc.Namespace, "StatefulSet", expectedSset.Name, "pvc_name", pvc.Name,
				"old_value", pvc.Spec.Resources.Requests.Storage().String(), "new_value", newSize.String())

			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *newSize
			if err := k8sClient.Update(context.Background(), &pvc); err != nil {
				return err
			}
		}
	}
	return nil
}

// annotateForRecreation stores the StatefulSet spec with updated storage requirements
// in an annotation of the Elasticsearch resource, to be recreated at the next reconciliation.
func annotateForRecreation(
	k8sClient client.Client,
	di *v1.DaisyInstallation,
	actualSset apps.StatefulSet,
	expectedClaims []corev1.PersistentVolumeClaim,
) error {
	log.Info("Preparing StatefulSet re-creation to account for PVC resize",
		"Namespace", di.Namespace, "Installation", di.Name, "StatefulSet", actualSset.Name)

	//TODO: mark the sts as "recreate"
	//actualSset.Spec.VolumeClaimTemplates = expectedClaims
	//asJSON, err := json.Marshal(actualSset)
	//if err != nil {
	//	return err
	//}
	//if di.Annotations == nil {
	//	di.Annotations = make(map[string]string, 1)
	//}
	//di.Annotations[RecreateStatefulSetAnnotationPrefix+actualSset.Name] = string(asJSON)
	//
	//return k8sClient.Update(context.Background(), di)
	return nil
}

//// DeleteOrphanPVCs ensures PersistentVolumeClaims created for the given es resource are deleted
//// when no longer used, since this is not done automatically by the StatefulSet controller.
//// Related issue in the k8s repo: https://github.com/kubernetes/kubernetes/issues/55045
//// PVCs that are not supposed to exist given the actual and expected StatefulSets are removed.
//// This covers:
//// * leftover PVCs created for StatefulSets that do not exist anymore
//// * leftover PVCs created for StatefulSets replicas that don't exist anymore (eg. downscale from 5 to 3 nodes)
//func (m *DaisyMemberManager) DeleteOrphanPVCs(
//	ctx *memberContext,
//	actualStatefulSets k8s.StatefulSetList,
//	expectedStatefulSets k8s.StatefulSetList,
//) error {
//	// PVCs are using the same labels as their corresponding StatefulSet, so we can filter on ES cluster name.
//	var pvcs corev1.PersistentVolumeClaimList
//	ns := client.InNamespace(ctx.Namespace)
//	matchLabels := client.MatchingLabels(labelInstallation(ctx).Labels())
//	if err := m.deps.Client.List(context.Background(), &pvcs, ns, matchLabels); err != nil {
//		return err
//	}
//	for _, pvc := range pvcsToRemove(pvcs.Items, actualStatefulSets, expectedStatefulSets) {
//		log.Info("Deleting PVC", "Namespace", pvc.Namespace, "PVC", pvc.Name)
//		if err := m.deps.Client.Delete(context.Background(), &pvc); err != nil {
//			return err
//		}
//	}
//	return nil
//}
//
//// pvcsToRemove filters the given pvcs to ones that can be safely removed based on Pods
//// of actual and expected StatefulSets.
//func pvcsToRemove(
//	pvcs []corev1.PersistentVolumeClaim,
//	actualStatefulSets k8s.StatefulSetList,
//	expectedStatefulSets k8s.StatefulSetList,
//) []corev1.PersistentVolumeClaim {
//	// Build the list of PVCs from both actual & expected StatefulSets (may contain duplicate entries).
//	// The list may contain PVCs for Pods that do not exist (eg. not created yet), but does not
//	// consider Pods in the process of being deleted (but not deleted yet), since already covered
//	// by checking expectations earlier in the process.
//	// Then, just return existing PVCs that are not part of that list.
//
//	keepSet := sets.NewString(actualStatefulSets.PVCNames()...)
//	keepSet.Insert(expectedStatefulSets.PVCNames()...)
//	var toRemove []corev1.PersistentVolumeClaim
//	for _, pvc := range pvcs {
//		if keepSet.Has(pvc.Name) {
//			continue
//		}
//		toRemove = append(toRemove, pvc)
//	}
//	return toRemove
//}

func (m *DaisyMemberManager) syncRepliaStatus(ctx *memberContext, di *v1.DaisyInstallation, set *apps.StatefulSet, result *syncResult) error {
	if set == nil {
		// skip if not created yet
		return nil
	}

	shardStatus := di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard]
	if shardStatus.Replicas == nil {
		//unusual case
		shardStatus.Replicas = map[string]v1.ReplicaStatus{}
	}
	var replicaStatus v1.ReplicaStatus
	var ok bool
	if replicaStatus, ok = di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas[ctx.CurReplica]; !ok {
		replicaStatus = v1.ReplicaStatus{
			Name:               ctx.CurReplica,
			LastTransitionTime: metav1.Now(),
		}
	}
	replicaStatus.StatefulSet = &set.Status

	switch result.Action {
	case ActionAdd:
		di.Status.AddedReplicasCount++
		replicaStatus.LastTransitionTime = metav1.Now()
	case ActionUpdate:
		di.Status.UpdatedReplicasCount++
		replicaStatus.LastTransitionTime = metav1.Now()
	}

	var upgrading = false
	var ready = false
	var err error

	// for fake daisy manager, statefulSetIsUpgradingFn might be nil
	if m.statefulSetIsUpgradingFn != nil {
		upgrading, err = m.statefulSetIsUpgradingFn(m.deps.Client, set, di)
	}
	if err != nil {
		return err
	}

	if k8s.HasStatefulSetReachedGeneration(set) {
		ready = true
	}

	// Scaling takes precedence over upgrading.
	if set.Status.ReadyReplicas != 1 {
		replicaStatus.Phase = v1.ScalePhase
		replicaStatus.State = v1.NotSync
	} else if upgrading {
		replicaStatus.Phase = v1.UpgradePhase
	} else if ready && replicaStatus.State == v1.NotSync {
		replicaStatus.Phase = v1.ReadyPhase
	} else {
		replicaStatus.Phase = v1.NormalPhase
		di.Status.ReadyReplicas++
	}

	di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard].Replicas[ctx.CurReplica] = replicaStatus
	return nil
}

func (m *DaisyMemberManager) deleteServiceForInstallation(ctx *memberContext, di *v1.DaisyInstallation) error {
	log := m.deps.Log

	svcName := CreateInstallationServiceName(di)
	log.V(1).Info("Delete Service for Installation", "Service", svcName)
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
	//TODO: use Job to delete tables async
	if err = m.deleteTables(ctx, r); err == nil {
		m.deps.Recorder.Eventf(di, corev1.EventTypeNormal, "Deleting", "Delete Replicated* tables for Replica  %q", r.Name)
	}

	if err = m.deleteStatefulSetForReplica(ctx, di, r); err != nil {
		return err
	}
	di.Status.DeletedReplicasCount++
	shardStatus := di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard]
	delete(shardStatus.Replicas, r.Name)
	di.Status.Clusters[ctx.CurCluster].Shards[ctx.CurShard] = shardStatus

	//TODO: mark PVC for garbage collect in future to avoid Pod fail to schedule error
	if di.Spec.PVReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
		if err = m.deletePVC(ctx, r); err != nil {
			log.Error(err, "Delete PVC failed")
			return err
		}
	}

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
		if err = UpdateInstallationStatus(m.deps.Client, di, true); err != nil {
			log.Info("Update Status fail", "error", err)
			return err
		}
		log.Info("Delete Replica completed")
	} else {
		log.Error(err, "Fail to Delete Replica")
	}

	return err
}

// deletePVC deletes PersistentVolumeClaim
func (m *DaisyMemberManager) deletePVC(ctx *memberContext, r *v1.Replica) error {
	log := m.deps.Log.WithValues("Replica", r.Name)
	log.V(2).Info("deletePVC() - start")
	defer log.V(2).Info("deletePVC() - end")

	// PVCs are using the same labels as their corresponding StatefulSet, so we can filter on ES cluster name.
	var pvcs corev1.PersistentVolumeClaimList
	ns := client.InNamespace(ctx.Namespace)
	matchLabels := client.MatchingLabels(labelReplica(ctx, r).Labels())
	if err := m.deps.Client.List(context.Background(), &pvcs, ns, matchLabels); err != nil {
		return err
	}

	for _, pvc := range pvcs.Items {
		log.Info("Deleting PVC", "Namespace", pvc.Namespace, "PVC", pvc.Name)
		if err := m.deps.Client.Delete(context.Background(), &pvc); err != nil {
			return err
		}
	}

	return nil
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
	log := m.deps.Log.WithValues("Replica", r.Name)
	log.V(2).Info("Delete Tables for Replica", "replica", ctx.CurReplica)
	if !r.CanDeleteAllPVCs() {
		return nil
	}

	err := m.schemer.DeleteTablesForReplica(ctx.Namespace, r)
	if err == nil {
		log.Info("Delete Replicated* tables successfully")
	} else {
		log.Error(err, "Delete Replicated* tables fail")
	}
	return err
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

package daisymanager

import (
	"encoding/json"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	v1 "github.com/daisy/daisy-operator/api/v1"
)

func pvcsToObj(pvcs []corev1.PersistentVolumeClaim) []runtime.Object {
	var ptrs []runtime.Object
	for i := range pvcs {
		ptrs = append(ptrs, &pvcs[i])
	}
	return ptrs
}

func tpsToObj(tps []v1.DaisyTemplate) []runtime.Object {
	var objs []runtime.Object
	for i := range tps {
		objs = append(objs, &tps[i])
	}
	return objs
}

func newUseTemplate(tpName string, namespace string) v1.UseTemplate {
	return v1.UseTemplate{
		Name:      tpName,
		Namespace: namespace,
		UseType:   "None",
	}
}
func newTemplate(tpName string, namespace string, podTps []v1.DaisyPodTemplate, volTps []v1.VolumeClaimTemplate) v1.DaisyTemplate {
	return v1.DaisyTemplate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaisyCluster",
			APIVersion: "daisy.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tpName,
			Namespace: namespace,
			UID:       "test-template",
		},
		Spec: v1.DaisyInstallationSpec{
			Templates: v1.Templates{
				PodTemplates:         podTps,
				VolumeClaimTemplates: volTps,
			},
		},
	}
}

func newPodTemplate(name string, containers ...string) v1.DaisyPodTemplate {
	var list []corev1.Container

	for _, container := range containers {
		list = append(list, corev1.Container{
			Name:  container,
			Image: fmt.Sprintf("%s-image:latest", container),
		})
	}
	return v1.DaisyPodTemplate{
		Name: name,
		Spec: corev1.PodSpec{
			Containers: list,
		},
	}
}

func newPVC(name string, size string, sc string) corev1.PersistentVolumeClaim {
	volumeMode := corev1.PersistentVolumeFilesystem
	return withStorageReq(corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "",
			Name:      name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: pointer.StringPtr(sc),
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
			VolumeMode: &volumeMode,
		},
	}, size)
}

func newVolumeClaimTemplate(name string, size string, sc string) v1.VolumeClaimTemplate {
	return v1.VolumeClaimTemplate{
		Name: name,
		Spec: newPVC(name, size, sc).Spec,
	}
}

func newTestTemplates() []v1.DaisyTemplate {
	pods1 := []v1.DaisyPodTemplate{
		newPodTemplate("pod1-tp1", "daisy1-v1"),
		newPodTemplate("pod1-tp2", "daisy1-v2"),
	}
	pods2 := []v1.DaisyPodTemplate{
		newPodTemplate("pod2-tp1", "daisy2-v1"),
		newPodTemplate("pod2-tp2", "daisy2-v2"),
	}
	vol1 := []v1.VolumeClaimTemplate{
		newVolumeClaimTemplate("vol1-data-tp1", "5Gi", "standard"),
		newVolumeClaimTemplate("vol1-data-tp2", "10Gi", "expand"),
		newVolumeClaimTemplate("vol1-log-tp1", "100M", "standard"),
		newVolumeClaimTemplate("vol1-log-tp2", "200M", "expand"),
	}
	vol2 := []v1.VolumeClaimTemplate{
		newVolumeClaimTemplate("vol2-data-tp1", "5Gi", "standard"),
		newVolumeClaimTemplate("vol2-data-tp2", "10Gi", "expand"),
		newVolumeClaimTemplate("vol2-log-tp1", "100M", "standard"),
		newVolumeClaimTemplate("vol2-log-tp2", "200M", "expand"),
	}
	return []v1.DaisyTemplate{
		newTemplate("tp1", corev1.NamespaceDefault, pods1, vol1),
		newTemplate("tp2", corev1.NamespaceDefault, pods2, vol2),
	}
}

func newTestInstallation(shardNum int, replicaNum int) *v1.DaisyInstallation {
	return &v1.DaisyInstallation{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaisyCluster",
			APIVersion: "daisy.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-installation",
			Namespace: corev1.NamespaceDefault,
			UID:       "test",
		},
		Spec: v1.DaisyInstallationSpec{
			PVReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			Configuration: v1.Configuration{
				Clusters: map[string]v1.Cluster{
					"cluster": {
						Name: "cluster",
						Layout: v1.Layout{
							ShardsCount:   shardNum,
							ReplicasCount: replicaNum,
						},
					},
				},
			},
		},
	}
}

func addTemplates(di *v1.DaisyInstallation) *v1.DaisyInstallation {
	di.Spec.Templates = v1.Templates{
		PodTemplates: []v1.DaisyPodTemplate{
			newPodTemplate("pod-tp01", "daisy"),
			newPodTemplate("pod-tp02", "daisy-01"),
		},
		VolumeClaimTemplates: []v1.VolumeClaimTemplate{
			newVolumeClaimTemplate("log-tp01", "100M", "standard"),
			newVolumeClaimTemplate("data-tp01", "5Gi", "standard"),
			newVolumeClaimTemplate("extra", "100M", "standard"),
		},
	}
	return di
}
func addSettings(di *v1.DaisyInstallation) *v1.DaisyInstallation {
	di.Spec.Configuration.Zookeeper.MergeFrom(&v1.ZookeeperConfig{
		Nodes: []v1.ZookeeperNode{
			{
				Host: "zk01.svc.cluster.local",
				Port: 2180,
			},
		},
	}, v1.MergeTypeOverrideByNonEmptyValues)
	di.Spec.Configuration.Users.MergeFrom(v1.Settings{
		"test/password": v1.NewScalarSetting("qwerty"),
		"test/networks/ip": v1.NewVectorSetting([]string{
			"127.0.0.1/32", "192.168.74.1/24",
		}),
		"test/profile": v1.NewScalarSetting("test_profile"),
		"test/quota":   v1.NewScalarSetting("test_quota"),
	})

	di.Spec.Configuration.Profiles.MergeFrom(v1.Settings{
		"test_profile/max_memory_usage": v1.NewScalarSetting("1000000000"),
		"test_profile/readonly":         v1.NewScalarSetting("1"),
		"readonly/readonly":             v1.NewScalarSetting("1"),
	})

	di.Spec.Configuration.Quotas.MergeFrom(v1.Settings{
		"test_quota/interval/duration": v1.NewScalarSetting("3600"),
	})

	di.Spec.Configuration.Settings.MergeFrom(v1.Settings{
		"compression/case/method":    v1.NewScalarSetting("zstd"),
		"disable_internal_dns_cache": v1.NewScalarSetting("1"),
	})

	di.Spec.Configuration.Files.MergeFrom(v1.Settings{
		"COMMON/dict1.xml": v1.NewScalarSetting(`|
        <yandex>
            <!-- ref to file /etc/clickhouse-data/config.d/source1.csv -->
        </yandex>`),
		"COMMON/source1.csv": v1.NewScalarSetting(`|
        a1,b1,c1,d1
        a2,b2,c2,d2`),
		"HOST/users_prefixed_1.file": v1.NewScalarSetting(`|
		<yandex>
		<!-- users_prefixed_1.file -->
		</yandex>`),
	})

	return di
}

func modifyDefaults(di *v1.DaisyInstallation, podTp string, dataTp string, logTp string) *v1.DaisyInstallation {
	di.Spec.Defaults.Templates = v1.TemplateNames{
		PodTemplate:             podTp,
		DataVolumeClaimTemplate: dataTp,
		LogVolumeClaimTemplate:  logTp,
	}
	return di
}

func addDefaults(di *v1.DaisyInstallation) *v1.DaisyInstallation {
	di.Spec.Defaults.Templates = v1.TemplateNames{
		PodTemplate:             "pod-tp01",
		DataVolumeClaimTemplate: "data-tp01",
		LogVolumeClaimTemplate:  "log-tp01",
	}
	return di
}

func withUseTemplate(di *v1.DaisyInstallation, tps ...v1.UseTemplate) *v1.DaisyInstallation {
	di.Spec.UseTemplates = append(di.Spec.UseTemplates, tps...)
	return di
}

func addUseTemplates(di *v1.DaisyInstallation) *v1.DaisyInstallation {
	tps := []v1.UseTemplate{
		newUseTemplate("tp1", corev1.NamespaceDefault),
		newUseTemplate("tp2", corev1.NamespaceDefault),
	}
	di.Spec.UseTemplates = tps
	return di
}

func withDeleteSlots(di *v1.DaisyInstallation, deleteSlots ...string) *v1.DaisyInstallation {
	b, err := json.Marshal(deleteSlots)
	if err != nil {
		return di
	}
	if di.Annotations == nil {
		di.Annotations = make(map[string]string)
	}
	di.Annotations[DeleteSlotsAnn] = string(b)
	return di
}

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func withVolumeExpansion(sc storagev1.StorageClass) *storagev1.StorageClass {
	sc.AllowVolumeExpansion = pointer.BoolPtr(true)
	return &sc
}

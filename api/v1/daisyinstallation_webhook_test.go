package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"testing"
)

func TestDaisyInstallation_ValidateCreate(t *testing.T) {
	type fields struct {
		TypeMeta   v1.TypeMeta
		ObjectMeta v1.ObjectMeta
		Spec       DaisyInstallationSpec
		Status     DaisyInstallationStatus
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "create pvc-template-success",
			fields: fields{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaisyCluster",
					APIVersion: "daisy.com/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-installation",
					Namespace: corev1.NamespaceDefault,
					UID:       "test",
				},
				Spec: DaisyInstallationSpec{
					Templates: Templates{
						VolumeClaimTemplates: []VolumeClaimTemplate{
							newVolumeClaimTemplate("dataTest1", "1Gi", "standard"),
							newVolumeClaimTemplate("logTest1", "100Mi", "standard"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "create pvc-template-fail",
			fields: fields{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaisyCluster",
					APIVersion: "daisy.com/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-installation",
					Namespace: corev1.NamespaceDefault,
					UID:       "test",
				},
				Spec: DaisyInstallationSpec{
					Templates: Templates{
						VolumeClaimTemplates: []VolumeClaimTemplate{
							newVolumeClaimTemplate("dataTest1", "1Gi", "standard"),
							newVolumeClaimTemplate("logTest1", "64Mi", "standard"),
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DaisyInstallation{
				TypeMeta:   tt.fields.TypeMeta,
				ObjectMeta: tt.fields.ObjectMeta,
				Spec:       tt.fields.Spec,
				Status:     tt.fields.Status,
			}
			if err := r.ValidateCreate(); (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDaisyInstallation_ValidateUpdate(t *testing.T) {
	type fields struct {
		TypeMeta   v1.TypeMeta
		ObjectMeta v1.ObjectMeta
		Spec       DaisyInstallationSpec
		Status     DaisyInstallationStatus
	}
	type args struct {
		old runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "update pvc-template-success",
			fields: fields{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaisyCluster",
					APIVersion: "daisy.com/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-installation",
					Namespace: corev1.NamespaceDefault,
					UID:       "test",
				},
				Spec: DaisyInstallationSpec{
					Templates: Templates{
						VolumeClaimTemplates: []VolumeClaimTemplate{
							newVolumeClaimTemplate("dataTest1", "1Gi", "standard"),
							newVolumeClaimTemplate("logTest1", "100Mi", "standard"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "update pvc-template-fail",
			fields: fields{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DaisyCluster",
					APIVersion: "daisy.com/v1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-installation",
					Namespace: corev1.NamespaceDefault,
					UID:       "test",
				},
				Spec: DaisyInstallationSpec{
					Templates: Templates{
						VolumeClaimTemplates: []VolumeClaimTemplate{
							newVolumeClaimTemplate("dataTest1", "500Mi", "standard"),
							newVolumeClaimTemplate("logTest1", "100Mi", "standard"),
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &DaisyInstallation{
				TypeMeta:   tt.fields.TypeMeta,
				ObjectMeta: tt.fields.ObjectMeta,
				Spec:       tt.fields.Spec,
				Status:     tt.fields.Status,
			}
			if err := r.ValidateUpdate(tt.args.old); (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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

func withStorageReq(claim corev1.PersistentVolumeClaim, size string) corev1.PersistentVolumeClaim {
	c := claim.DeepCopy()
	c.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(size)
	return *c
}

func newVolumeClaimTemplate(name string, size string, sc string) VolumeClaimTemplate {
	return VolumeClaimTemplate{
		Name: name,
		Spec: newPVC(name, size, sc).Spec,
	}
}

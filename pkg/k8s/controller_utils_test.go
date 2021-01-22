package k8s

import (
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"testing"
)

func Test_allowsVolumeExpansion(t *testing.T) {

	tests := []struct {
		name string
		sc   v1.StorageClass
		want bool
	}{
		{
			name: "empty allowsVolumeExpansion",
			sc: v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc01",
				},
			},
			want: false,
		},
		{
			name: "support expansion",
			sc: v1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sc01",
				},
				AllowVolumeExpansion: pointer.BoolPtr(true),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowsVolumeExpansion(tt.sc); got != tt.want {
				t.Errorf("allowsVolumeExpansion() = %v, want %v", got, tt.want)
			}
		})
	}
}

package controllers

import (
	"context"
	v1 "github.com/daisy/daisy-operator/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("DaisyInstallation Controller", func() {

	const timeout = time.Second * 15
	const interval = time.Millisecond * 500

	BeforeEach(func() {

	})

	AfterEach(func() {

	})

	Context("DaisyInstallation Status Check", func() {

		It("Should update status.spec to last applied spec", func() {
			key := types.NamespacedName{
				Name:      "simple01",
				Namespace: "default",
			}
			retainPVP := corev1.PersistentVolumeReclaimRetain
			spec := v1.DaisyInstallationSpec{
				Configuration: v1.Configuration{
					Clusters: map[string]v1.Cluster{
						"cluster": {
							Layout: v1.Layout{
								ReplicasCount: 1,
								ShardsCount:   1,
							},
						},
					},
				},
				ImagePullPolicy: corev1.PullIfNotPresent,
				PVReclaimPolicy: retainPVP,
			}
			di := &v1.DaisyInstallation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: spec,
			}
			//b, _ := json.Marshal(spec)
			By("Creating the daisy installation successfully")
			Expect(k8sClient.Create(context.Background(), di)).Should(Succeed())

			By("Installation Service Should created successfully")
			diSvc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(),
					client.ObjectKey{Namespace: "default", Name: "daisy-simple01"}, diSvc)
			},
				timeout, interval).Should(Succeed())
			Expect(diSvc.Name).Should(Equal("daisy-simple01"))

			By("Replica Service should created successfully")
			replSvc := corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(context.Background(),
					client.ObjectKey{Namespace: "default", Name: "simple01-cluster-0-0"}, &replSvc)
			},
				timeout, interval).Should(Succeed())

			By("The stateful set should be created successfully")
			diSts1 := appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(
					context.Background(), client.ObjectKey{Name: "simple01-cluster-0-0", Namespace: "default"},
					&diSts1)
			},
				timeout, interval).Should(Succeed())

			By("Update the status with to last applied spec")
			updated := v1.DaisyInstallation{}
			Eventually(func() bool {
				k8sClient.Get(context.Background(), key, &updated)
				return updated.Status.PrevSpec.Configuration.Clusters["cluster"].Layout.Shards != nil
			}, timeout, interval).Should(Equal(true))

			By("Delete the daisy installation successfully")
			Expect(k8sClient.Delete(context.Background(), &updated)).Should(Succeed())
			tmp := &v1.DaisyInstallation{}
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), key, tmp)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

	})
})

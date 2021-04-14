package v1

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("DaisyInstallation Webhook", func() {

	const timeout = time.Second * 15
	const interval = time.Millisecond * 500

	BeforeEach(func() {

	})

	AfterEach(func() {

	})

	Context("Webhook func check", func() {

		It("Should init pv storage to last applied VolumeClaimTemplate", func() {
			key := types.NamespacedName{
				Name:      "simple01",
				Namespace: "default",
			}
			retainPVP := corev1.PersistentVolumeReclaimRetain
			spec := DaisyInstallationSpec{
				Configuration: Configuration{
					Clusters: map[string]Cluster{
						"cluster": {
							Layout: Layout{
								ReplicasCount: 1,
								ShardsCount:   1,
							},
						},
					},
				},
				Templates: Templates{
					VolumeClaimTemplates: []VolumeClaimTemplate{
						newVolumeClaimTemplate("dataTest1", "1Gi", "standard"),
						newVolumeClaimTemplate("logTest1", "100Mi", "standard"),
					},
				},
				ImagePullPolicy: corev1.PullIfNotPresent,
				PVReclaimPolicy: retainPVP,
			}
			di := &DaisyInstallation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: spec,
			}

			//b, _ := json.Marshal(spec)
			By("webhook check  pass and create the daisy installation successfully")
			Expect(k8sClient.Create(context.Background(), di)).Should(Succeed())

			By("update pv a unreasonable Storage and daisy install unsuccessfully")
			di.Spec.Templates.VolumeClaimTemplates[0] = newVolumeClaimTemplate("dataTest1", "500Mi", "standard")

			Eventually(func() bool {
				err := k8sClient.Update(context.Background(), di)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(Equal(false))

			By("Delete daisy installation successfully")
			Expect(k8sClient.Delete(context.Background(), di)).Should(Succeed())
			tmp := &DaisyInstallation{}
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), key, tmp)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

		})

	})
})

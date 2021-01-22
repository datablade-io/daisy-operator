package daisymanager

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/daisy/daisy-operator/pkg/k8s"
)

const (
	pvcImmutableErrMsg = "volume claim templates can only have their storage requests increased, if the storage class allows volume expansion. Any other change is forbidden"
)

// ValidateClaimsStorageUpdate compares updated vs. initial claim, and returns an error if:
// - a storage decrease is attempted
// - a storage increase is attempted but the storage class does not support volume expansion
// - a new claim was added in updated ones
func ValidateClaimsStorageUpdate(
	k8sClient client.Client,
	initial []corev1.PersistentVolumeClaim,
	updated []corev1.PersistentVolumeClaim,
	validateStorageClass bool,
) error {
	for _, updatedClaim := range updated {
		initialClaim := claimMatchingName(initial, updatedClaim.Name)
		if initialClaim == nil {
			// existing claim does not exist in updated
			return errors.New(pvcImmutableErrMsg)
		}

		cmp := k8s.CompareStorageRequests(initialClaim.Spec.Resources, updatedClaim.Spec.Resources)
		switch {
		case cmp.Increase:
			// storage increase requested: ensure the storage class allows volume expansion
			if err := k8s.EnsureClaimSupportsExpansion(k8sClient, updatedClaim, validateStorageClass); err != nil {
				return err
			}
		case cmp.Decrease:
			// storage decrease is not supported
			return fmt.Errorf("decreasing storage size is not supported: an attempt was made to decrease storage size for claim %s", updatedClaim.Name)
		}
	}
	return nil
}

func claimMatchingName(claims []corev1.PersistentVolumeClaim, name string) *corev1.PersistentVolumeClaim {
	for i, claim := range claims {
		if claim.Name == name {
			return &claims[i]
		}
	}
	return nil
}

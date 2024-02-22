package utils

import (
	"net/http"

	"github.com/SAP/sap-btp-service-operator/api/common"
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Condition Utils", func() {
	var resource *v1.ServiceBinding
	BeforeEach(func() {
		resource = getBinding()
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	})
	AfterEach(func() {
		err := k8sClient.Delete(ctx, resource)
		Expect(client.IgnoreNotFound(err)).ToNot(HaveOccurred())
	})
	Context("InitConditions", func() {
		It("should initialize conditions and update status", func() {
			err := InitConditions(ctx, k8sClient, resource)
			Expect(err).ToNot(HaveOccurred())
			Expect(meta.IsStatusConditionPresentAndEqual(resource.GetConditions(), common.ConditionReady, metav1.ConditionFalse)).To(BeTrue())
		})
	})

	Context("GetConditionReason", func() {
		When("given operation type CREATE and state SUCCEEDED", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.CREATE, smClientTypes.SUCCEEDED)).To(Equal(common.Created))
			})
		})

		When("given operation type UPDATE and state SUCCEEDED", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.UPDATE, smClientTypes.SUCCEEDED)).To(Equal(common.Updated))
			})
		})

		When("given operation type DELETE and state SUCCEEDED", func() {
			It("returns expected condition reason", func() {
				expected := common.Deleted
				Expect(GetConditionReason(smClientTypes.DELETE, smClientTypes.SUCCEEDED)).To(Equal(expected))
			})
		})

		When("given operation type CREATE and state INPROGRESS", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.CREATE, smClientTypes.INPROGRESS)).To(Equal(common.CreateInProgress))
			})
		})

		When("given operation type UPDATE and state INPROGRESS", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.UPDATE, smClientTypes.INPROGRESS)).To(Equal(common.UpdateInProgress))
			})
		})

		When("given operation type DELETE and state INPROGRESS", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.DELETE, smClientTypes.INPROGRESS)).To(Equal(common.DeleteInProgress))
			})
		})

		When("given operation type CREATE and state FAILED", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.CREATE, smClientTypes.FAILED)).To(Equal(common.CreateFailed))
			})
		})

		When("given operation type UPDATE and state FAILED", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.UPDATE, smClientTypes.FAILED)).To(Equal(common.UpdateFailed))
			})
		})

		When("given operation type DELETE and state FAILED", func() {
			It("returns expected condition reason", func() {
				Expect(GetConditionReason(smClientTypes.DELETE, smClientTypes.FAILED)).To(Equal(common.DeleteFailed))
			})
		})

		When("given an unknown operation type and state SUCCEEDED", func() {
			It("returns finished condition reason", func() {
				Expect(GetConditionReason("unknown", smClientTypes.SUCCEEDED)).To(Equal(common.Finished))
			})
		})

		When("given an unknown operation type and state INPROGRESS", func() {
			It("returns in progress condition reason", func() {
				Expect(GetConditionReason("unknown", smClientTypes.INPROGRESS)).To(Equal(common.InProgress))
			})
		})

		When("given an unknown operation type and state FAILED", func() {
			It("returns failed condition reason", func() {
				Expect(GetConditionReason("unknown", smClientTypes.FAILED)).To(Equal(common.Failed))
			})
		})

		When("given operation type CREATE and unknown state", func() {
			It("returns unknown condition reason", func() {
				Expect(GetConditionReason(smClientTypes.CREATE, "unknown")).To(Equal(common.Unknown))
			})
		})
	})

	Context("SetInProgressConditions", func() {
		It("should set in-progress conditions", func() {
			resource = getBinding()

			SetInProgressConditions(ctx, smClientTypes.CREATE, "Pending", resource)

			// Add assertions to check the state of the resource after calling SetInProgressConditions
			Expect(resource.GetConditions()).ToNot(BeEmpty())
			// Add more assertions based on your expected behavior
		})
	})

	Context("SetSuccessConditions", func() {
		It("should set success conditions", func() {
			operationType := smClientTypes.CREATE
			resource = getBinding()

			SetSuccessConditions(operationType, resource)

			// Add assertions to check the state of the resource after calling SetSuccessConditions
			Expect(resource.GetConditions()).ToNot(BeEmpty())
			Expect(resource.GetReady()).To(Equal(metav1.ConditionTrue))
			// Add more assertions based on your expected behavior
		})
	})

	Context("SetCredRotationInProgressConditions", func() {
		It("should set credentials rotation in-progress conditions", func() {
			reason := "RotationReason"
			message := "RotationMessage"
			resource = getBinding()

			SetCredRotationInProgressConditions(reason, message, resource)

			// Add assertions to check the state of the resource after calling SetCredRotationInProgressConditions
			Expect(resource.GetConditions()).ToNot(BeEmpty())
			// Add more assertions based on your expected behavior
		})
	})

	Context("SetFailureConditions", func() {
		It("should set failure conditions", func() {
			operationType := smClientTypes.CREATE
			errorMessage := "Operation failed"
			SetFailureConditions(operationType, errorMessage, resource)
			Expect(resource.GetConditions()).ToNot(BeEmpty())
			Expect(meta.IsStatusConditionPresentAndEqual(resource.GetConditions(), common.ConditionReady, metav1.ConditionFalse)).To(BeTrue())
		})
	})

	Context("MarkAsNonTransientError", func() {
		It("should mark as non-transient error and update status", func() {
			operationType := smClientTypes.CREATE
			errorMessage := "Non-transient error"

			result, err := MarkAsNonTransientError(ctx, k8sClient, operationType, errorMessage, resource)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("MarkAsTransientError", func() {
		It("should handle TooManyRequests error correctly", func() {
			resource.SetConditions([]metav1.Condition{{Message: "not TooManyRequests"}})
			serviceManagerError := &sm.ServiceManagerError{StatusCode: http.StatusTooManyRequests}
			result, err := MarkAsTransientError(ctx, k8sClient, smClientTypes.UPDATE, serviceManagerError, resource)
			Expect(err).ToNot(BeNil())
			Expect(resource.GetConditions()[0].Message).To(ContainSubstring("not TooManyRequests")) //TooManyRequests is not reflected to status
			Expect(result).To(BeEquivalentTo(ctrl.Result{}))
		})
	})

	Context("SetBlockedCondition", func() {
		It("Blocked Condition Set on ServiceBinding", func() {
			sb := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{
					Conditions: []metav1.Condition{},
				},
			}

			SetBlockedCondition(ctx, "Test message", sb)
			Expect(meta.FindStatusCondition(sb.Status.Conditions, common.ConditionSucceeded).Reason).To(Equal(common.Blocked))
		})
	})

	Context("IsInProgress", func() {
		It("should return true for in progress condition", func() {
			resource := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   common.ConditionSucceeded,
							Status: metav1.ConditionFalse,
						},
						{
							Type:   common.ConditionFailed,
							Status: metav1.ConditionFalse,
						},
					},
				},
			}

			Expect(IsInProgress(resource)).To(BeTrue())
		})

		It("should return false for failed condition", func() {
			resource := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   common.ConditionSucceeded,
							Status: metav1.ConditionFalse,
						},
						{
							Type:   common.ConditionFailed,
							Status: metav1.ConditionTrue,
						},
					},
				},
			}

			Expect(IsInProgress(resource)).To(BeFalse())
		})

		It("should return false for succeeded condition", func() {
			resource := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{
					Conditions: []metav1.Condition{
						{
							Type:   common.ConditionSucceeded,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   common.ConditionFailed,
							Status: metav1.ConditionFalse,
						},
					},
				},
			}

			Expect(IsInProgress(resource)).To(BeFalse())
		})

		It("should return false for empty conditions", func() {
			resource := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{
					Conditions: []metav1.Condition{},
				},
			}

			Expect(IsInProgress(resource)).To(BeFalse())
		})
	})

	Context("IsFailed", func() {
		It("Should return false when no conditions available", func() {
			sb := &v1.ServiceBinding{Status: v1.ServiceBindingStatus{Conditions: []metav1.Condition{}}}
			result := IsFailed(sb)
			Expect(result).Should(BeFalse())
		})

		It("Should return true when ConditionFailed is true", func() {
			sb := &v1.ServiceBinding{Status: v1.ServiceBindingStatus{Conditions: []metav1.Condition{{Type: common.ConditionFailed, Status: metav1.ConditionTrue}}}}
			result := IsFailed(sb)
			Expect(result).Should(BeTrue())
		})

		It("Should return false when ConditionFailed is false", func() {
			sb := &v1.ServiceBinding{Status: v1.ServiceBindingStatus{Conditions: []metav1.Condition{{Type: common.ConditionFailed, Status: metav1.ConditionFalse}}}}
			result := IsFailed(sb)
			Expect(result).Should(BeFalse())
		})

		It("Should return true when ConditionSucceeded is false and reason is Blocked", func() {
			sb := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{Conditions: []metav1.Condition{{Type: common.ConditionSucceeded, Status: metav1.ConditionFalse, Reason: common.Blocked}}},
			}
			result := IsFailed(sb)
			Expect(result).Should(BeTrue())
		})

		It("Should return false when ConditionSucceeded is true and reason is Blocked", func() {
			sb := &v1.ServiceBinding{
				Status: v1.ServiceBindingStatus{Conditions: []metav1.Condition{{Type: common.ConditionSucceeded, Status: metav1.ConditionTrue, Reason: common.Blocked}}},
			}
			result := IsFailed(sb)
			Expect(result).Should(BeFalse())
		})
	})
})

func getBinding() *v1.ServiceBinding {
	return &v1.ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "services.cloud.sap.com/v1",
			Kind:       "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-binding-1",
			Namespace: testNamespace,
		},
		Spec: v1.ServiceBindingSpec{
			ServiceInstanceName: "service-instance-1",
			ExternalName:        "my-service-binding-1",
			Parameters:          &runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
			ParametersFrom: []v1.ParametersFromSource{
				{
					SecretKeyRef: &v1.SecretKeyReference{
						Name: "param-secret",
						Key:  "secret-parameter",
					},
				},
			},
			CredRotationPolicy: &v1.CredentialsRotationPolicy{
				Enabled:           true,
				RotationFrequency: "1s",
				RotatedBindingTTL: "1s",
			},
		},

		Status: v1.ServiceBindingStatus{},
	}
}

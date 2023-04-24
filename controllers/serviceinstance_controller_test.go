package controllers

import (
	"context"
	"github.com/SAP/sap-btp-service-operator/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/utils/pointer"
	"net/http"
	"strings"

	"fmt"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/SAP/sap-btp-service-operator/client/sm/smfakes"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	smclientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

const (
	fakeInstanceID                    = "ic-fake-instance-id"
	fakeInstanceExternalName          = "ic-test-instance-external-name"
	fakeInstanceExternalNameNonShared = "ic-test-instance-external-name-non-shared"
	testNamespace                     = "ic-test-namespace"
	fakeOfferingName                  = "offering-a"
	fakePlanName                      = "plan-a"
)

var _ = Describe("ServiceInstance controller", func() {

	var serviceInstance *v1.ServiceInstance
	var fakeInstanceName string
	var ctx context.Context
	var defaultLookupKey types.NamespacedName

	updateSpec := func() v1.ServiceInstanceSpec {
		newExternalName := "my-new-external-name" + uuid.New().String()
		return v1.ServiceInstanceSpec{
			ExternalName:        newExternalName,
			ServicePlanName:     fakePlanName,
			ServiceOfferingName: fakeOfferingName,
		}
	}

	instanceSpec := v1.ServiceInstanceSpec{
		ExternalName:        fakeInstanceExternalName,
		ServicePlanName:     fakePlanName,
		ServiceOfferingName: fakeOfferingName,
		Parameters: &runtime.RawExtension{
			Raw: []byte(`{"key": "value"}`),
		},
		ParametersFrom: []v1.ParametersFromSource{
			{
				SecretKeyRef: &v1.SecretKeyReference{
					Name: "param-secret",
					Key:  "secret-parameter",
				},
			},
		},
	}

	sharedInstanceSpec := v1.ServiceInstanceSpec{
		ExternalName:        fakeInstanceExternalName + "shared",
		ServicePlanName:     fakePlanName + "shared",
		Shared:              pointer.BoolPtr(true),
		ServiceOfferingName: fakeOfferingName + "shared",
		Parameters: &runtime.RawExtension{
			Raw: []byte(`{"key": "value"}`),
		},
		ParametersFrom: []v1.ParametersFromSource{
			{
				SecretKeyRef: &v1.SecretKeyReference{
					Name: "param-secret",
					Key:  "secret-parameter",
				},
			},
		},
	}

	createInstance := func(ctx context.Context, instanceSpec v1.ServiceInstanceSpec) *v1.ServiceInstance {
		instance := &v1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "services.cloud.sap.com/v1",
				Kind:       "ServiceInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fakeInstanceName,
				Namespace: testNamespace,
			},
			Spec: instanceSpec,
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		createdInstance := &v1.ServiceInstance{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, defaultLookupKey, createdInstance)
			if err != nil {
				return false
			}
			readyCond := meta.FindStatusCondition(createdInstance.GetConditions(), api.ConditionReady)
			return readyCond != nil
		}, timeout, interval).Should(BeTrue())

		return createdInstance
	}

	deleteInstance := func(ctx context.Context, instanceToDelete *v1.ServiceInstance, wait bool) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceToDelete.Name, Namespace: instanceToDelete.Namespace}, &v1.ServiceInstance{})
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
			return
		}

		Expect(k8sClient.Delete(ctx, instanceToDelete)).Should(Succeed())

		if wait {
			Eventually(func() bool {
				a := &v1.ServiceInstance{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceToDelete.Name, Namespace: instanceToDelete.Namespace}, a)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}
	}

	updateInstance := func(ctx context.Context, serviceInstance *v1.ServiceInstance) *v1.ServiceInstance {
		isConditionRefersUpdateOp := func(instance *v1.ServiceInstance) bool {
			conditionReason := instance.Status.Conditions[0].Reason
			return strings.Contains(conditionReason, Updated) || strings.Contains(conditionReason, UpdateInProgress) || strings.Contains(conditionReason, UpdateFailed)

		}

		_ = k8sClient.Update(ctx, serviceInstance)
		updatedInstance := &v1.ServiceInstance{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, defaultLookupKey, updatedInstance)
			if err != nil {
				return false
			}
			return len(updatedInstance.Status.Conditions) > 0 && isConditionRefersUpdateOp(updatedInstance)
		}, timeout, interval).Should(BeTrue())

		return updatedInstance
	}

	BeforeEach(func() {
		ctx = context.Background()
		fakeInstanceName = "ic-test-" + uuid.New().String()
		defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: testNamespace}

		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
		fakeClient.DeprovisionReturns("", nil)
		fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)

		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "param-secret"}, secret)
		if apierrors.IsNotFound(err) {
			createParamsSecret(testNamespace)
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		if serviceInstance != nil {
			deleteInstance(ctx, serviceInstance, true)
		}
	})

	Describe("Create", func() {
		Context("Invalid parameters", func() {
			createInstanceWithFailure := func(spec v1.ServiceInstanceSpec) {
				instance := &v1.ServiceInstance{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "services.cloud.sap.com/v1",
						Kind:       "ServiceInstance",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      fakeInstanceName,
						Namespace: testNamespace,
					},
					Spec: spec,
				}
				Expect(k8sClient.Create(ctx, instance)).ShouldNot(Succeed())
			}
			Describe("service plan id not provided", func() {
				When("service offering name and service plan name are not provided", func() {
					It("provisioning should fail", func() {
						createInstanceWithFailure(v1.ServiceInstanceSpec{})
					})
				})
				When("service offering name is provided and service plan name is not provided", func() {
					It("provisioning should fail", func() {
						createInstanceWithFailure(v1.ServiceInstanceSpec{ServiceOfferingName: "fake-offering"})
					})
				})
				When("service offering name not provided and service plan name is provided", func() {
					It("provisioning should fail", func() {
						createInstanceWithFailure(v1.ServiceInstanceSpec{ServicePlanID: "fake-plan"})
					})
				})
			})

			Describe("service plan id is provided", func() {
				When("service offering name and service plan name are not provided", func() {
					It("provision should fail", func() {
						createInstanceWithFailure(v1.ServiceInstanceSpec{ServicePlanID: "fake-plan-id"})
					})
				})
				When("plan id does not match the provided offering name and plan name", func() {
					instanceSpec := v1.ServiceInstanceSpec{
						ServiceOfferingName: fakeOfferingName,
						ServicePlanName:     fakePlanName,
						ServicePlanID:       "wrong-id",
					}
					BeforeEach(func() {
						fakeClient.ProvisionReturns(nil, fmt.Errorf("provided plan id does not match the provided offeing name and plan name"))
					})

					It("provisioning should fail", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Expect(serviceInstance.Status.Conditions[0].Message).To(ContainSubstring("provided plan id does not match"))
					})
				})
			})
		})

		Context("Sync", func() {
			When("provision request to SM succeeds", func() {
				It("should provision instance of the provided offering and plan name successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
					Expect(serviceInstance.Spec.ExternalName).To(Equal(fakeInstanceExternalName))
					Expect(serviceInstance.Name).To(Equal(fakeInstanceName))
					Expect(serviceInstance.Status.HashedSpec).To(Not(BeNil()))
					Expect(string(serviceInstance.Spec.Parameters.Raw)).To(ContainSubstring("\"key\":\"value\""))
					Expect(serviceInstance.Status.HashedSpec).To(Equal(getSpecHash(serviceInstance)))
					smInstance, _, _, _, _, _ := fakeClient.ProvisionArgsForCall(0)
					params := smInstance.Parameters
					Expect(params).To(ContainSubstring("\"key\":\"value\""))
					Expect(params).To(ContainSubstring("\"secret-key\":\"secret-value\""))
				})
			})

			When("provision request to SM fails", func() {
				var errMessage string
				Context("with 400 status", func() {
					JustBeforeEach(func() {
						errMessage = "failed to provision instance"
						fakeClient.ProvisionReturns(nil, &sm.ServiceManagerError{
							StatusCode: http.StatusBadRequest,
							Message:    errMessage,
						})
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)

					})

					It("should have failure condition", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Expect(len(serviceInstance.Status.Conditions)).To(Equal(3))
						Expect(serviceInstance.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
						Expect(serviceInstance.Status.Conditions[0].Message).To(ContainSubstring(errMessage))
					})
				})

				Context("with 429 status eventually succeeds", func() {
					JustBeforeEach(func() {
						errMessage = "failed to provision instance"
						fakeClient.ProvisionReturnsOnCall(0, nil, &sm.ServiceManagerError{
							StatusCode: http.StatusTooManyRequests,
							Message:    errMessage,
						})
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should retry until success", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Eventually(func() bool {
							err := k8sClient.Get(context.Background(), types.NamespacedName{Name: serviceInstance.Name, Namespace: serviceInstance.Namespace}, serviceInstance)
							Expect(err).ToNot(HaveOccurred())
							return isReady(serviceInstance)
						}, timeout, interval).Should(BeTrue())

						Expect(len(serviceInstance.Status.Conditions)).To(Equal(2))
						Expect(serviceInstance.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
					})
				})
			})
		})

		Context("Async", func() {
			BeforeEach(func() {
				fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID, Location: "/v1/service_instances/fakeid/operations/1234"}, nil)
				fakeClient.StatusReturns(&smclientTypes.Operation{
					ID:    "1234",
					Type:  smClientTypes.CREATE,
					State: smClientTypes.INPROGRESS,
				}, nil)
			})

			When("polling ends with success", func() {
				It("should update in progress condition and provision the instance successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.SUCCEEDED,
					}, nil)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isReady(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})
			})

			When("polling ends with failure", func() {
				It("should update in progress condition and afterwards failure condition", func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.FAILED,
					}, nil)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isFailed(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})
			})

			When("updating during create", func() {
				It("should save the latest spec", func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					newName := "new-name" + uuid.New().String()
					serviceInstance.Spec.ExternalName = newName
					err := k8sClient.Update(ctx, serviceInstance)
					Expect(err).ToNot(HaveOccurred())

					// stop polling state
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.SUCCEEDED,
					}, nil)

					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isReady(serviceInstance) && serviceInstance.Spec.ExternalName == newName
					}, timeout, interval).Should(BeTrue())
				})
			})

			When("deleting during create", func() {
				It("should be deleted", func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					newName := "new-name" + uuid.New().String()
					serviceInstance.Spec.ExternalName = newName
					deleteInstance(ctx, serviceInstance, false)

					// stop polling state
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.SUCCEEDED,
					}, nil)

					// validate deletion
					Eventually(func() bool {
						err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return apierrors.IsNotFound(err)
					}, timeout, interval).Should(BeTrue())
				})
			})
		})

		When("external name is not provided", func() {
			It("succeeds and uses the k8s name as external name", func() {
				withoutExternal := v1.ServiceInstanceSpec{
					ServicePlanName:     "a-plan-name",
					ServiceOfferingName: "an-offering-name",
				}
				serviceInstance = createInstance(ctx, withoutExternal)
				Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
				Expect(serviceInstance.Spec.ExternalName).To(Equal(fakeInstanceName))
				Expect(serviceInstance.Name).To(Equal(fakeInstanceName))
			})
		})

	})

	Describe("Update", func() {

		JustBeforeEach(func() {
			serviceInstance = createInstance(ctx, instanceSpec)
			Expect(serviceInstance.Spec.ExternalName).To(Equal(fakeInstanceExternalName))
		})

		Context("When update call to SM succeed", func() {
			Context("Sync", func() {
				When("spec is changed", func() {
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "", nil)
					})
					It("condition should be Updated", func() {
						newSpec := updateSpec()
						serviceInstance.Spec = newSpec
						serviceInstance = updateInstance(ctx, serviceInstance)
						Expect(serviceInstance.Spec.ExternalName).To(Equal(newSpec.ExternalName))
						Expect(serviceInstance.Status.Conditions[0].Reason).To(Equal(Updated))
						Expect(serviceInstance.Spec.UserInfo).NotTo(BeNil())
					})
				})
			})

			Context("Async", func() {

				When("spec is changed", func() {
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "/v1/service_instances/id/operations/1234", nil)
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.INPROGRESS,
						}, nil)
					})

					It("condition should be updated from in progress to Updated", func() {
						newSpec := updateSpec()
						serviceInstance.Spec = newSpec
						updatedInstance := updateInstance(ctx, serviceInstance)
						Expect(updatedInstance.Status.Conditions[0].Reason).To(Equal(UpdateInProgress))
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.SUCCEEDED,
						}, nil)
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, updatedInstance)
							return isReady(serviceInstance)
						}, timeout, interval).Should(BeTrue())
						Expect(updatedInstance.Spec.ExternalName).To(Equal(newSpec.ExternalName))
					})

					When("updating during update", func() {
						It("should save the latest spec", func() {
							By("updating first time")
							serviceInstance.Spec = updateSpec()
							updatedInstance := updateInstance(ctx, serviceInstance)

							By("updating second time")
							lastSpec := updateSpec()
							updatedInstance.Spec = lastSpec
							err := k8sClient.Update(ctx, updatedInstance)
							Expect(err).ToNot(HaveOccurred())

							// stop polling state
							fakeClient.StatusReturns(&smclientTypes.Operation{
								ID:    "1234",
								Type:  smClientTypes.UPDATE,
								State: smClientTypes.SUCCEEDED,
							}, nil)
							Eventually(func() bool {
								_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
								return isReady(serviceInstance) && serviceInstance.Spec.ExternalName == lastSpec.ExternalName
							}, timeout, interval).Should(BeTrue())
						})
					})

					When("deleting during update", func() {
						It("should be deleted", func() {
							serviceInstance.Spec = updateSpec()
							updatedInstance := updateInstance(ctx, serviceInstance)
							deleteInstance(ctx, updatedInstance, false)
							// stop update polling
							fakeClient.StatusReturns(&smclientTypes.Operation{
								ID:    "1234",
								Type:  smClientTypes.UPDATE,
								State: smClientTypes.SUCCEEDED,
							}, nil)

							// validate deletion
							Eventually(func() bool {
								err := k8sClient.Get(ctx, defaultLookupKey, updatedInstance)
								return apierrors.IsNotFound(err)
							}, timeout, interval).Should(BeTrue())
						})
					})
				})
			})
		})

		Context("When update call to SM fails", func() {
			Context("Sync", func() {
				When("spec is changed", func() {
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "", fmt.Errorf("failed to update instance"))
					})
					It("condition should be Updated", func() {
						newSpec := updateSpec()
						serviceInstance.Spec = newSpec
						updatedInstance := updateInstance(ctx, serviceInstance)
						Expect(updatedInstance.Status.Conditions[0].Reason).To(Equal(UpdateFailed))
					})
				})
			})

			Context("Async", func() {
				When("spec is changed", func() {
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "/v1/service_instances/id/operations/1234", nil)
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.INPROGRESS,
						}, nil)
					})

					It("condition should be updated from in progress to Updated", func() {
						newSpec := updateSpec()
						serviceInstance.Spec = newSpec
						updatedInstance := updateInstance(ctx, serviceInstance)
						Expect(updatedInstance.Status.Conditions[0].Reason).To(Equal(UpdateInProgress))
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.FAILED,
						}, nil)
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, updatedInstance)
							return isReady(serviceInstance)
						}, timeout, interval).Should(BeTrue())
					})
				})

				When("Instance has operation url to operation that no longer exist in SM", func() {
					JustBeforeEach(func() {
						fakeClient.UpdateInstanceReturnsOnCall(0, nil, "/v1/service_instances/id/operations/1234", nil)
						fakeClient.UpdateInstanceReturnsOnCall(1, nil, "", nil)
						fakeClient.StatusReturns(nil, &sm.ServiceManagerError{StatusCode: http.StatusNotFound})
						smInstance := &smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.UPDATE}}
						fakeClient.GetInstanceByIDReturns(smInstance, nil)
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{*smInstance},
						}, nil)
					})
					It("should recover", func() {
						Eventually(func() bool {
							newSpec := updateSpec()
							serviceInstance.Spec = newSpec
							updateInstance(ctx, serviceInstance)
							err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							Expect(err).ToNot(HaveOccurred())
							return isReady(serviceInstance)
						}, timeout, interval).Should(BeTrue())
					})
				})
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			serviceInstance = createInstance(ctx, instanceSpec)
		})
		Context("Sync", func() {
			When("delete in SM succeeds", func() {
				BeforeEach(func() {
					fakeClient.DeprovisionReturns("", nil)
				})
				It("should delete the k8s instance", func() {
					deleteInstance(ctx, serviceInstance, true)
				})
			})

			When("delete without instance id", func() {
				JustBeforeEach(func() {
					fakeClient.DeprovisionReturns("", nil)

					fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
						ServiceInstances: []smclientTypes.ServiceInstance{
							{
								ID: serviceInstance.Status.InstanceID,
							},
						},
					}, nil)

					serviceInstance.Status.InstanceID = ""
					Expect(k8sClient.Status().Update(context.Background(), serviceInstance)).To(Succeed())
				})

				It("should delete the k8s instance", func() {
					deleteInstance(ctx, serviceInstance, true)
				})
			})

			When("delete in SM fails", func() {
				JustBeforeEach(func() {
					fakeClient.DeprovisionReturns("", fmt.Errorf("failed to delete instance"))
				})

				JustAfterEach(func() {
					fakeClient.DeprovisionReturns("", nil)
				})

				It("should not delete the k8s instance and should update the condition", func() {
					deleteInstance(ctx, serviceInstance, false)
					Eventually(func() bool {
						err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						if err != nil {
							return false
						}
						return isFailed(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})
			})
		})

		Context("Async", func() {
			JustBeforeEach(func() {
				fakeClient.DeprovisionReturns("/v1/service_instances/id/operations/1234", nil)
				fakeClient.StatusReturns(&smclientTypes.Operation{
					ID:    "1234",
					Type:  smClientTypes.DELETE,
					State: smClientTypes.INPROGRESS,
				}, nil)
				deleteInstance(ctx, serviceInstance, false)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
					if err != nil {
						return false
					}
					return isInProgress(serviceInstance)
				}, timeout, interval).Should(BeTrue())
			})

			When("polling ends with success", func() {
				JustBeforeEach(func() {
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.DELETE,
						State: smClientTypes.SUCCEEDED,
					}, nil)
				})
				It("should delete the k8s instance", func() {
					deleteInstance(ctx, serviceInstance, true)
				})
			})

			When("polling ends with failure", func() {
				JustBeforeEach(func() {
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.DELETE,
						State: smClientTypes.FAILED,
					}, nil)
				})

				AfterEach(func() {
					fakeClient.DeprovisionReturns("", nil)
				})

				It("should not delete the k8s instance and condition is updated with failure", func() {
					deleteInstance(ctx, serviceInstance, false)
					Eventually(func() bool {
						err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						if apierrors.IsNotFound(err) {
							return false
						}
						return isFailed(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})
			})
		})

		Context("Instance ID is empty", func() {
			BeforeEach(func() {
				serviceInstance.Status.InstanceID = ""
				Expect(k8sClient.Status().Update(context.Background(), serviceInstance)).Should(Succeed())
			})
			When("instance not exist in SM", func() {
				It("should be deleted successfully", func() {
					deleteInstance(ctx, serviceInstance, true)
					Expect(fakeClient.DeprovisionCallCount()).To(Equal(0))
				})
			})

			type TestCase struct {
				lastOpType  smClientTypes.OperationCategory
				lastOpState smClientTypes.OperationState
			}
			DescribeTable("instance exist in SM", func(testCase TestCase) {
				recoveredInstance := smclientTypes.ServiceInstance{
					ID:            fakeInstanceID,
					Name:          fakeInstanceName,
					LastOperation: &smClientTypes.Operation{State: testCase.lastOpState, Type: testCase.lastOpType},
				}
				fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
					ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
				fakeClient.DeprovisionReturns("", nil)

				deleteInstance(ctx, serviceInstance, true)
				Expect(fakeClient.DeprovisionCallCount()).To(Equal(1))
			},
				Entry("last operation is CREATE SUCCEEDED", TestCase{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.SUCCEEDED}),
				Entry("last operation is CREATE FAILED", TestCase{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.FAILED}),
				Entry("last operation is UPDATE SUCCEEDED", TestCase{lastOpType: smClientTypes.UPDATE, lastOpState: smClientTypes.SUCCEEDED}),
				Entry("last operation is UPDATE FAILED", TestCase{lastOpType: smClientTypes.UPDATE, lastOpState: smClientTypes.FAILED}),
				Entry("last operation is CREATE IN_PROGRESS", TestCase{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.INPROGRESS}),
				Entry("last operation is UPDATE IN_PROGRESS", TestCase{lastOpType: smClientTypes.UPDATE, lastOpState: smClientTypes.INPROGRESS}),
				Entry("last operation is DELETE IN_PROGRESS", TestCase{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.INPROGRESS}))
		})
	})

	Context("Recovery", func() {
		When("instance exists in SM", func() {
			recoveredInstance := smclientTypes.ServiceInstance{
				ID:            fakeInstanceID,
				Name:          fakeInstanceName,
				LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE},
			}
			BeforeEach(func() {
				fakeClient.ProvisionReturns(nil, fmt.Errorf("ERROR"))
			})
			AfterEach(func() {
				fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{ServiceInstances: []smclientTypes.ServiceInstance{}}, nil)
			})

			It("should call correctly to SM", func() {
				serviceInstance = createInstance(ctx, instanceSpec)
				smCallArgs := fakeClient.ListInstancesArgsForCall(0)
				Expect(smCallArgs.LabelQuery).To(HaveLen(1))
				Expect(smCallArgs.LabelQuery[0]).To(ContainSubstring("_k8sname"))

				Expect(smCallArgs.FieldQuery).To(HaveLen(3))
				Expect(smCallArgs.FieldQuery[0]).To(ContainSubstring("name"))
				Expect(smCallArgs.FieldQuery[1]).To(ContainSubstring("context/clusterid"))
				Expect(smCallArgs.FieldQuery[2]).To(ContainSubstring("context/namespace"))
			})

			Context("last operation", func() {
				When("last operation state is SUCCEEDED", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})
					It("should recover the existing instance", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
					})
				})

				When("last operation state is PENDING and ends with success", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = &smClientTypes.Operation{State: smClientTypes.PENDING, Type: smClientTypes.CREATE}
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
						fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.PENDING, Type: smClientTypes.CREATE}, nil)
					})
					It("should recover the existing instance and poll until instance is ready", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
						Expect(serviceInstance.Status.Conditions[0].Reason).To(Equal(CreateInProgress))
						fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}, nil)
						Eventually(func() bool {
							Expect(k8sClient.Get(ctx, defaultLookupKey, serviceInstance)).Should(Succeed())
							return isReady(serviceInstance)
						})
					})
				})

				When("last operation state is FAILED", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = &smClientTypes.Operation{State: smClientTypes.FAILED, Type: smClientTypes.CREATE}
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})
					It("should recover the existing instance and update condition failure", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
						Expect(len(serviceInstance.Status.Conditions)).To(Equal(3))
						Expect(serviceInstance.Status.Conditions[0].Reason).To(Equal(CreateFailed))
					})
				})

				When("no last operation", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = nil
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})
					It("should recover the existing instance", func() {
						serviceInstance = createInstance(ctx, instanceSpec)
						Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
					})
				})
			})
		})
	})

	Describe("Share instance", func() {
		Context("Share", func() {
			When("creating instance with shared=true", func() {
				It("should succeed to provision and sharing the instance", func() {
					instanceSharingReturnSuccess()
					serviceInstance = createInstance(ctx, sharedInstanceSpec)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isInstanceReady(serviceInstance)
					}, timeout, interval).Should(BeTrue())
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isInstanceShared(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})
			})

			Context("sharing an existing instance", func() {
				BeforeEach(func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isInstanceReady(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})

				When("updating existing instance to shared", func() {
					It("should succeed", func() {
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return isInstanceShared(serviceInstance)
						}, timeout, interval).Should(BeTrue())
					})
				})

				When("sharing succeeds", func() {
					It("hashed spec should be the same before and after", func() {
						before := serviceInstance.Status.HashedSpec
						fakeClient.UpdateInstanceReturns(nil, "", nil)
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						instanceUnSharingReturnSuccess()
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return isInstanceShared(serviceInstance)
						}, timeout, interval).Should(BeTrue())
						after := serviceInstance.Status.HashedSpec
						Expect(before).To(Equal(after))
					})
				})

				When("updating instance to shared and updating the name", func() {
					It("eventually should succeed updating both", func() {
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						serviceInstance.Spec.ExternalName = "new"
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return isInstanceShared(serviceInstance)
						}, timeout, interval).Should(BeTrue())
						Expect(serviceInstance.Spec.ExternalName).To(Equal("new"))
					})
				})
			})

			When("instance creation failed", func() {
				It("should not attempt to share the instance", func() {
					errMessage := "failed to provision instance"
					fakeClient.ProvisionReturns(nil, &sm.ServiceManagerError{
						StatusCode: http.StatusBadRequest,
						Message:    errMessage,
					})
					serviceInstance = createInstance(ctx, sharedInstanceSpec)
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: serviceInstance.Namespace}, serviceInstance)
						if err != nil {
							return false
						}
						conditions := serviceInstance.GetConditions()
						readyCond := meta.FindStatusCondition(conditions, api.ConditionReady)
						if readyCond == nil {
							return false
						}
						return readyCond.Status == metav1.ConditionFalse
					}, timeout, interval).Should(BeTrue())
					Expect(fakeClient.ShareInstanceCallCount()).To(BeZero())
				})
			})

			When("instance is valid and share failed", func() {
				BeforeEach(func() {
					serviceInstance = createInstance(ctx, instanceSpec)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isInstanceReady(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})

				When("shared failed with rate limit error", func() {
					It("status should be shared in progress", func() {
						instanceSharingReturnsRateLimitError()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							conditions := serviceInstance.GetConditions()
							cond := meta.FindStatusCondition(conditions, api.ConditionShared)
							return cond != nil && cond.Reason == InProgress
						}, timeout*3, interval).Should(BeTrue())

						instanceSharingReturnSuccess()
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return isInstanceShared(serviceInstance)
						}, timeout*3, interval).Should(BeTrue())
					})
				})

				When("shared failed with transient error which is not rate limit", func() {
					It("status should be shared failed and eventually succeed sharing", func() {
						instanceSharingReturnsTransientError()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							conditions := serviceInstance.GetConditions()
							cond := meta.FindStatusCondition(conditions, api.ConditionShared)
							return cond != nil && cond.Reason == ShareFailed
						}, timeout*3, interval).Should(BeTrue())

						instanceSharingReturnSuccess()
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return isInstanceShared(serviceInstance)
						}, timeout*3, interval).Should(BeTrue())
					})
				})

				When("shared failed with non transient error 400", func() {
					It("should have a final shared status", func() {
						instanceSharingReturnsNonTransientError400()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							conditions := serviceInstance.GetConditions()
							cond := meta.FindStatusCondition(conditions, api.ConditionShared)
							return cond != nil && cond.Reason == ShareNotSupported
						}, timeout, interval).Should(BeTrue())
					})
				})

				When("shared failed with non transient error 500", func() {
					It("should have a final shared status", func() {
						instanceSharingReturnsNonTransientError500()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							conditions := serviceInstance.GetConditions()
							cond := meta.FindStatusCondition(conditions, api.ConditionShared)
							return cond != nil && cond.Reason == ShareNotSupported
						}, timeout, interval).Should(BeTrue())
					})
				})
			})
		})

		Context("Un-Share", func() {
			Context("un-sharing an existing shared instance", func() {
				BeforeEach(func() {
					instanceSharingReturnSuccess()
					serviceInstance = createInstance(ctx, sharedInstanceSpec)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isInstanceReady(serviceInstance) && isInstanceShared(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})

				When("updating instance to un-shared and sm return success", func() {
					It("should succeed", func() {
						serviceInstance.Spec.Shared = pointer.BoolPtr(false)
						instanceUnSharingReturnSuccess()
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return !isInstanceShared(serviceInstance)
						}, timeout, interval).Should(BeTrue())
					})
				})

				When("deleting shared property from spec", func() {
					It("should succeed un-sharing and remove condition", func() {
						serviceInstance.Spec.Shared = nil
						instanceUnSharingReturnSuccess()
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return meta.FindStatusCondition(serviceInstance.GetConditions(), api.ConditionShared) == nil
						}, timeout, interval).Should(BeTrue())
					})
				})

				When("updating instance to un-shared and updating the name", func() {
					It("eventually should succeed updating both", func() {
						serviceInstance.Spec.Shared = pointer.BoolPtr(false)
						serviceInstance.Spec.ExternalName = "new"
						Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return !isInstanceShared(serviceInstance)
						}, timeout, interval).Should(BeTrue())
						Expect(serviceInstance.Spec.ExternalName).To(Equal("new"))
					})
				})
			})

			When("instance is valid and un-share failed", func() {
				BeforeEach(func() {
					instanceSharingReturnSuccess()
					serviceInstance = createInstance(ctx, sharedInstanceSpec)
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						return isInstanceReady(serviceInstance) && isInstanceShared(serviceInstance)
					}, timeout, interval).Should(BeTrue())
				})

				It("should have a reason un-shared failed", func() {
					instanceUnSharingReturnsNonTransientError()
					serviceInstance.Spec.Shared = pointer.BoolPtr(false)
					Expect(k8sClient.Update(ctx, serviceInstance)).To(Succeed())
					Eventually(func() bool {
						_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
						conditions := serviceInstance.GetConditions()
						cond := meta.FindStatusCondition(conditions, api.ConditionShared)
						return cond != nil && cond.Reason == UnShareFailed
					}, timeout, interval).Should(BeTrue())
				})
			})
		})
	})
})

func instanceSharingReturnSuccess() {
	fakeClient.ShareInstanceReturns(nil)
}

func instanceUnSharingReturnsNonTransientError() {
	fakeClient.UnShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode: http.StatusBadRequest,
		Message:    "nonTransient",
	})
}

func instanceSharingReturnsNonTransientError400() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode: http.StatusBadRequest,
		Message:    "nonTransient",
	})
}

func instanceSharingReturnsNonTransientError500() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode: http.StatusInternalServerError,
		Message:    "nonTransient",
	})
}

func instanceSharingReturnsTransientError() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode: http.StatusTooEarly,
		Message:    "transient",
	})
}

func instanceSharingReturnsRateLimitError() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode: http.StatusTooManyRequests,
		Message:    "transient",
	})
}

func instanceUnSharingReturnSuccess() {
	fakeClient.UnShareInstanceReturns(nil)
}

func createParamsSecret(namespace string) {
	credentialsMap := make(map[string][]byte)
	credentialsMap["secret-parameter"] = []byte("{\"secret-key\":\"secret-value\"}")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "param-secret",
			Namespace: namespace,
		},
		Data: credentialsMap,
	}
	err := k8sClient.Create(context.Background(), secret)
	Expect(err).ToNot(HaveOccurred())
}

func isInstanceShared(serviceInstance *v1.ServiceInstance) bool {
	conditions := serviceInstance.GetConditions()
	if conditions == nil {
		return false
	}

	sharedCond := meta.FindStatusCondition(conditions, api.ConditionShared)
	if sharedCond == nil {
		return false
	}

	return sharedCond.Status == metav1.ConditionTrue
}

func isInstanceReady(serviceInstance *v1.ServiceInstance) bool {
	conditions := serviceInstance.GetConditions()
	if conditions == nil {
		return false
	}

	sharedCond := meta.FindStatusCondition(conditions, api.ConditionReady)
	if sharedCond == nil {
		return false
	}

	return sharedCond.Status == metav1.ConditionTrue
}

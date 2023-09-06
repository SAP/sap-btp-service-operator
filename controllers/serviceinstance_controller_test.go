package controllers

import (
	"context"
	"fmt"
	"github.com/SAP/sap-btp-service-operator/api"
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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"net/http"
	"strings"
)

// +kubebuilder:docs-gen:collapse=Imports

const (
	fakeInstanceID           = "ic-fake-instance-id"
	fakeInstanceExternalName = "ic-test-instance-external-name"
	testNamespace            = "ic-test-namespace"
	fakeOfferingName         = "offering-a"
	fakePlanName             = "plan-a"
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

	createInstance := func(ctx context.Context, instanceSpec v1.ServiceInstanceSpec, waitForReady bool) *v1.ServiceInstance {
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

		if !waitForReady {
			return instance
		}
		return waitForInstanceToBeReady(ctx, types.NamespacedName{Name: fakeInstanceName, Namespace: testNamespace})
	}

	deleteInstance := func(ctx context.Context, instanceToDelete *v1.ServiceInstance, wait bool) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceToDelete.Name, Namespace: instanceToDelete.Namespace}, &v1.ServiceInstance{})
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
			return
		}

		Eventually(func() bool {
			return k8sClient.Delete(ctx, instanceToDelete) == nil
		}, timeout, interval).Should(BeTrue())

		if wait {
			Eventually(func() bool {
				a := &v1.ServiceInstance{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceToDelete.Name, Namespace: instanceToDelete.Namespace}, a)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		fakeInstanceName = "ic-test-" + uuid.New().String()
		defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: testNamespace}

		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
		fakeClient.DeprovisionReturns("", nil)
		fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)

		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "param-secret"}, &corev1.Secret{})
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
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceConditionAndMessage(ctx, defaultLookupKey, api.ConditionSucceeded, "provided plan id does not match")
					})
				})
			})
		})

		Context("Sync", func() {
			When("provision request to SM succeeds", func() {
				It("should provision instance of the provided offering and plan name successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec, true)
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
				Context("with 400 status", func() {
					errMessage := "failed to provision instance"
					BeforeEach(func() {
						fakeClient.ProvisionReturns(nil, &sm.ServiceManagerError{
							StatusCode:  http.StatusBadRequest,
							Description: errMessage,
						})
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should have failure condition", func() {
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceCreationFailure(ctx, defaultLookupKey, serviceInstance, errMessage)
					})
				})

				Context("with 429 status eventually succeeds", func() {
					BeforeEach(func() {
						errMessage := "failed to provision instance"
						fakeClient.ProvisionReturnsOnCall(0, nil, &sm.ServiceManagerError{
							StatusCode:  http.StatusTooManyRequests,
							Description: errMessage,
						})
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should retry until success", func() {
						serviceInstance = createInstance(ctx, instanceSpec, true)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, Created)
					})
				})

				Context("with sm status code 502 and broker status code 429", func() {
					errorMessage := "broker too many requests"
					BeforeEach(func() {
						fakeClient.ProvisionReturns(nil, getTransientBrokerError(errorMessage))
					})

					It("should be transient error and eventually succeed", func() {
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceCreationFailure(ctx, defaultLookupKey, serviceInstance, errorMessage)
						fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
						waitForInstanceToBeReady(ctx, defaultLookupKey)
					})
				})

				Context("with sm status code 502 and broker status code 400", func() {
					errMessage := "failed to provision instance"
					BeforeEach(func() {
						fakeClient.ProvisionReturns(nil, getNonTransientBrokerError(errMessage))
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should have failure condition - non transient error", func() {
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceCreationFailure(ctx, defaultLookupKey, serviceInstance, errMessage)
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
					serviceInstance = createInstance(ctx, instanceSpec, false)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.SUCCEEDED,
					}, nil)
					waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, Created)
				})
			})

			When("polling ends with failure", func() {
				It("should update to failure condition with the broker err description", func() {
					serviceInstance = createInstance(ctx, instanceSpec, false)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:     "1234",
						Type:   smClientTypes.CREATE,
						State:  smClientTypes.FAILED,
						Errors: []byte(`{"error": "brokerError","description":"broker-failure"}`),
					}, nil)
					waitForInstanceConditionAndMessage(ctx, defaultLookupKey, api.ConditionFailed, "broker-failure")
				})
			})

			When("updating during create", func() {
				It("should save the latest spec", func() {
					serviceInstance = createInstance(ctx, instanceSpec, false)
					newName := "new-name" + uuid.New().String()
					serviceInstance.Spec.ExternalName = newName

					Eventually(func() bool {
						if err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance); err != nil {
							return false
						}

						serviceInstance.Spec.ExternalName = newName
						return k8sClient.Update(ctx, serviceInstance) == nil
					}, timeout, interval).Should(BeTrue())

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

			When("deleting while create is in progress", func() {
				It("should be deleted successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec, false)

					By("waiting for instance to be CreateInProgress")
					waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, CreateInProgress)

					fakeClient.DeprovisionReturns("/v1/service_instances/id/operations/1234", nil)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.DELETE,
						State: smClientTypes.INPROGRESS,
					}, nil)

					deleteInstance(ctx, serviceInstance, false)
					waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, DeleteInProgress)

					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.DELETE,
						State: smClientTypes.SUCCEEDED,
					}, nil)

					validateInstanceGotDeleted(ctx, defaultLookupKey)
				})
			})
		})

		When("external name is not provided", func() {
			It("succeeds and uses the k8s name as external name", func() {
				withoutExternal := v1.ServiceInstanceSpec{
					ServicePlanName:     "a-plan-name",
					ServiceOfferingName: "an-offering-name",
				}
				serviceInstance = createInstance(ctx, withoutExternal, true)
				Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
				Expect(serviceInstance.Spec.ExternalName).To(Equal(fakeInstanceName))
				Expect(serviceInstance.Name).To(Equal(fakeInstanceName))
			})
		})
	})

	Describe("Update", func() {
		BeforeEach(func() {
			serviceInstance = createInstance(ctx, instanceSpec, true)
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
						Expect(serviceInstance.Spec.UserInfo).NotTo(BeNil())
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, Updated)
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
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, UpdateInProgress)
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.SUCCEEDED,
						}, nil)
						instance := waitForInstanceToBeReady(ctx, defaultLookupKey)
						Expect(instance.Status.InstanceID).ToNot(BeEmpty())
						Expect(instance.Spec.ExternalName).To(Equal(newSpec.ExternalName))
					})

					When("updating during update", func() {
						It("should save the latest spec", func() {
							serviceInstance.Spec = updateSpec()
							updatedInstance := updateInstance(ctx, serviceInstance)

							lastSpec := updateSpec()
							updatedInstance.Spec = lastSpec
							updateInstance(ctx, updatedInstance)

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

							fakeClient.StatusReturns(&smclientTypes.Operation{
								ID:    "1234",
								Type:  smClientTypes.UPDATE,
								State: smClientTypes.SUCCEEDED,
							}, nil)

							validateInstanceGotDeleted(ctx, defaultLookupKey)
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
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, UpdateFailed)
					})
				})
			})

			Context("spec is changed, sm returns 502 and broker returns 429", func() {
				errMessage := "broker too many requests"
				BeforeEach(func() {
					fakeClient.UpdateInstanceReturns(nil, "", getTransientBrokerError(errMessage))
				})

				It("recognize the error as transient and eventually succeed", func() {
					serviceInstance.Spec = updateSpec()
					updateInstance(ctx, serviceInstance)
					serviceInstance = waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, UpdateInProgress)
					fakeClient.UpdateInstanceReturns(nil, "", nil)
					updateInstance(ctx, serviceInstance)
					waitForInstanceToBeReady(ctx, defaultLookupKey)
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
						serviceInstance.Spec = updateSpec()
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, UpdateInProgress)
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.FAILED,
						}, nil)
						waitForInstanceToBeReady(ctx, defaultLookupKey)
					})
				})

				When("Instance has operation url to operation that no longer exist in SM", func() {
					BeforeEach(func() {
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
						serviceInstance.Spec = updateSpec()
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeReady(ctx, defaultLookupKey)
					})
				})
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			serviceInstance = createInstance(ctx, instanceSpec, true)
			fakeClient.DeprovisionReturns("", nil)
		})
		AfterEach(func() {
			fakeClient.DeprovisionReturns("", nil)
		})

		Context("Sync", func() {
			When("delete in SM succeeds", func() {
				It("should delete the k8s instance", func() {
					deleteInstance(ctx, serviceInstance, true)
				})
			})

			When("instance is marked for prevent deletion", func() {
				BeforeEach(func() {
					fakeClient.UpdateInstanceReturns(nil, "", nil)
				})
				It("should fail deleting the instance because of the webhook delete validation", func() {
					markInstanceAsPreventDeletion(serviceInstance)
					updateInstance(ctx, serviceInstance)
					err := k8sClient.Delete(ctx, serviceInstance)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("is marked with \"prevent deletion\""))

					/* After annotation is removed the instance should be deleted properly */
					serviceInstance.Annotations = nil
					updateInstance(ctx, serviceInstance)
					deleteInstance(ctx, serviceInstance, true)
				})
			})

			When("delete without instance id", func() {
				BeforeEach(func() {
					fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
						ServiceInstances: []smclientTypes.ServiceInstance{
							{
								ID: serviceInstance.Status.InstanceID,
							},
						},
					}, nil)

					serviceInstance.Status.InstanceID = ""
					Expect(k8sClient.Status().Update(ctx, serviceInstance)).To(Succeed())
				})

				It("should delete the k8s instance", func() {
					deleteInstance(ctx, serviceInstance, true)
				})
			})

			When("delete in SM fails", func() {
				It("should not delete the k8s instance and should update the condition", func() {
					err := "failed to delete instance"
					fakeClient.DeprovisionReturns("", fmt.Errorf(err))
					deleteInstance(ctx, serviceInstance, false)
					waitForInstanceToBeFailedWithMsg(ctx, defaultLookupKey, err)
				})
			})
		})

		Context("Async", func() {
			BeforeEach(func() {
				fakeClient.DeprovisionReturns("/v1/service_instances/id/operations/1234", nil)
				fakeClient.StatusReturns(&smclientTypes.Operation{
					ID:    "1234",
					Type:  smClientTypes.DELETE,
					State: smClientTypes.INPROGRESS,
				}, nil)
				deleteInstance(ctx, serviceInstance, false)
				waitForInstanceToBeInProgress(ctx, defaultLookupKey)
			})

			When("polling ends with success", func() {
				BeforeEach(func() {
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
				BeforeEach(func() {
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:     "1234",
						Type:   smClientTypes.DELETE,
						State:  smClientTypes.FAILED,
						Errors: []byte(`{"error": "brokerError","description":"broker-failure"}`),
					}, nil)
				})

				It("should not delete the k8s instance and condition is updated with failure", func() {
					deleteInstance(ctx, serviceInstance, false)
					waitForInstanceToBeFailedWithMsg(ctx, defaultLookupKey, "broker-failure")
				})
			})
		})

		Context("Instance ID is empty", func() {
			BeforeEach(func() {
				serviceInstance.Status.InstanceID = ""
				Expect(k8sClient.Status().Update(ctx, serviceInstance)).Should(Succeed())
			})
			AfterEach(func() {
				fakeClient.DeprovisionReturns("", nil)
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
				fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
			})

			It("should call correctly to SM and recover the instance", func() {
				serviceInstance = createInstance(ctx, instanceSpec, true)
				Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
				Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
				smCallArgs := fakeClient.ListInstancesArgsForCall(0)
				Expect(smCallArgs.LabelQuery).To(HaveLen(1))
				Expect(smCallArgs.LabelQuery[0]).To(ContainSubstring("_k8sname"))

				Expect(smCallArgs.FieldQuery).To(HaveLen(3))
				Expect(smCallArgs.FieldQuery[0]).To(ContainSubstring("name"))
				Expect(smCallArgs.FieldQuery[1]).To(ContainSubstring("context/clusterid"))
				Expect(smCallArgs.FieldQuery[2]).To(ContainSubstring("context/namespace"))
			})

			Context("last operation", func() {
				When("last operation state is PENDING and ends with success", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = &smClientTypes.Operation{State: smClientTypes.PENDING, Type: smClientTypes.CREATE}
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
						fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.PENDING, Type: smClientTypes.CREATE}, nil)
					})

					It("should recover the existing instance and poll until instance is ready", func() {
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceID(ctx, defaultLookupKey, fakeInstanceID)
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, CreateInProgress)
						fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}, nil)
						waitForInstanceToBeReady(ctx, defaultLookupKey)
					})
				})

				When("last operation state is FAILED", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = &smClientTypes.Operation{State: smClientTypes.FAILED, Type: smClientTypes.CREATE}
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})

					It("should recover the existing instance and update condition failure", func() {
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceID(ctx, defaultLookupKey, fakeInstanceID)
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionSucceeded, CreateFailed)
					})
				})

				When("no last operation", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = nil
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})
					It("should recover the existing instance", func() {
						serviceInstance = createInstance(ctx, instanceSpec, false)
						waitForInstanceID(ctx, defaultLookupKey, fakeInstanceID)
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
					createInstance(ctx, sharedInstanceSpec, true)
					serviceInstance = waitForInstanceToBeShared(ctx, defaultLookupKey)
					Expect(len(serviceInstance.Status.Conditions)).To(Equal(3))
				})
			})

			Context("sharing an existing instance", func() {
				BeforeEach(func() {
					serviceInstance = createInstance(ctx, instanceSpec, true)
				})

				When("updating existing instance to shared", func() {
					It("should succeed", func() {
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeShared(ctx, defaultLookupKey)
					})
				})

				When("sharing succeeds", func() {
					It("hashed spec should be the same before and after", func() {
						before := serviceInstance.Status.HashedSpec
						fakeClient.UpdateInstanceReturns(nil, "", nil)
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						instanceSharingReturnSuccess()
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeShared(ctx, defaultLookupKey)
						after := serviceInstance.Status.HashedSpec
						Expect(before).To(Equal(after))
					})
				})

				When("updating instance to shared and updating the name", func() {
					It("eventually should succeed updating both", func() {
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						serviceInstance.Spec.ExternalName = "new"
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeShared(ctx, defaultLookupKey)
						Expect(serviceInstance.Spec.ExternalName).To(Equal("new"))
					})
				})
			})

			When("instance creation failed", func() {
				It("should not attempt to share the instance", func() {
					fakeClient.ProvisionReturns(nil, &sm.ServiceManagerError{
						StatusCode:  http.StatusBadRequest,
						Description: "errMessage",
					})
					serviceInstance = createInstance(ctx, sharedInstanceSpec, false)
					waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionFailed, CreateFailed)
					Expect(fakeClient.ShareInstanceCallCount()).To(BeZero())
				})
			})

			When("instance is valid and share failed", func() {
				BeforeEach(func() {
					serviceInstance = createInstance(ctx, instanceSpec, true)
				})

				When("shared failed with rate limit error", func() {
					It("status should be shared in progress", func() {
						instanceSharingReturnsRateLimitError()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionShared, InProgress)
						instanceSharingReturnSuccess()
						waitForInstanceToBeShared(ctx, defaultLookupKey)
					})
				})

				When("shared failed with transient error which is not rate limit", func() {
					It("status should be shared failed and eventually succeed sharing", func() {
						instanceSharingReturnsTransientError()
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionShared, ShareFailed)

						instanceSharingReturnSuccess()
						waitForInstanceToBeShared(ctx, defaultLookupKey)
					})
				})

				When("shared failed with non transient error 400", func() {
					It("should have a final shared status", func() {
						instanceSharingReturnsNonTransientError400()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionShared, ShareNotSupported)
					})
				})

				When("shared failed with non transient error 500", func() {
					It("should have a final shared status", func() {
						instanceSharingReturnsNonTransientError500()
						serviceInstance.Spec.Shared = pointer.BoolPtr(true)
						updateInstance(ctx, serviceInstance)
						waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionShared, ShareNotSupported)
					})
				})
			})
		})

		Context("Un-Share", func() {
			Context("un-sharing an existing shared instance", func() {
				BeforeEach(func() {
					instanceSharingReturnSuccess()
					serviceInstance = createInstance(ctx, sharedInstanceSpec, true)
					waitForInstanceToBeShared(ctx, defaultLookupKey)
				})

				When("updating instance to un-shared and sm return success", func() {
					It("should succeed", func() {
						serviceInstance.Spec.Shared = pointer.BoolPtr(false)
						instanceUnSharingReturnSuccess()
						updateInstance(ctx, serviceInstance)
						serviceInstance = waitForInstanceToBeUnShared(ctx, defaultLookupKey)
						Expect(len(serviceInstance.Status.Conditions)).To(Equal(3))

					})
				})

				When("deleting shared property from spec", func() {
					It("should succeed un-sharing and remove condition", func() {
						serviceInstance.Spec.Shared = nil
						updateInstance(ctx, serviceInstance)
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
							return meta.FindStatusCondition(serviceInstance.GetConditions(), api.ConditionShared) == nil
						}, timeout, interval).Should(BeTrue())
						Expect(len(serviceInstance.Status.Conditions)).To(Equal(2))
					})
				})

				When("updating instance to un-shared and updating the name", func() {
					It("eventually should succeed updating both", func() {
						serviceInstance.Spec.Shared = pointer.Bool(false)
						serviceInstance.Spec.ExternalName = "new"
						updateInstance(ctx, serviceInstance)
						serviceInstance = waitForInstanceToBeUnShared(ctx, defaultLookupKey)
						Expect(serviceInstance.Spec.ExternalName).To(Equal("new"))
					})
				})
			})

			When("instance is valid and un-share failed", func() {
				BeforeEach(func() {
					instanceSharingReturnSuccess()
					serviceInstance = createInstance(ctx, sharedInstanceSpec, true)
					waitForInstanceToBeShared(ctx, defaultLookupKey)
					instanceUnSharingReturnsNonTransientError()
				})

				It("should have a reason un-shared failed", func() {
					serviceInstance.Spec.Shared = pointer.Bool(false)
					updateInstance(ctx, serviceInstance)
					waitForInstanceConditionAndReason(ctx, defaultLookupKey, api.ConditionShared, UnShareFailed)
				})
			})
		})
	})

	FContext("Unit Tests", func() {
		Context("isFinalState", func() {
			When("Ready.ObservedGeneration == 0", func() {
				It("should be true", func() {
					var instance = &v1.ServiceInstance{Status: v1.ServiceInstanceStatus{
						Conditions: []metav1.Condition{
							{
								Type:               api.ConditionReady,
								Status:             metav1.ConditionTrue,
								ObservedGeneration: 0,
							},
							{
								Type:               api.ConditionSucceeded,
								Status:             metav1.ConditionTrue,
								ObservedGeneration: 1,
							}},
					}}
					instance.SetGeneration(1)
					Expect(isFinalState(instance)).To(BeTrue())
				})

				When("Succeeded is false", func() {
					It("should return false", func() {
						var instance = &v1.ServiceInstance{Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 0,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionFalse,
									ObservedGeneration: 1,
								}},
						}}
						instance.SetGeneration(1)
						Expect(isFinalState(instance)).To(BeFalse())
					})
				})
			})

			When("generation is > 1", func() {
				When("observed generation == generation", func() {
					It("should return true ", func() {
						var instance = &v1.ServiceInstance{Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 0,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 8,
								}},
						}}
						instance.SetGeneration(8)
						Expect(isFinalState(instance)).To(BeTrue())
					})
				})

				When("observed generation != generation", func() {
					It("should return false", func() {
						var instance = &v1.ServiceInstance{Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 0,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 7,
								}},
						}}
						instance.SetGeneration(8)
						Expect(isFinalState(instance)).To(BeFalse())
					})
				})
			})
		})
	})
})

func validateInstanceConditionAndReason(ctx context.Context, key types.NamespacedName, condType string, reasons []string) bool {
	si := &v1.ServiceInstance{}
	var cond *metav1.Condition

	if err := k8sClient.Get(ctx, key, si); err != nil {
		return false
	}
	if cond = meta.FindStatusCondition(si.GetConditions(), condType); cond == nil {
		return false
	}

	for _, reason := range reasons {
		if reason == cond.Reason {
			return true
		}
	}
	return false
}

func waitForInstanceToBeReady(ctx context.Context, key types.NamespacedName) *v1.ServiceInstance {
	return waitForInstanceConditionAndReason(ctx, key, api.ConditionReady, Provisioned)
}

func waitForInstanceConditionAndMessage(ctx context.Context, key types.NamespacedName, conditionType, msg string) {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(si.GetConditions(), conditionType)
		return cond != nil && strings.Contains(cond.Message, msg)
	}, timeout, interval).Should(BeTrue())
}

func waitForInstanceToBeShared(ctx context.Context, key types.NamespacedName) *v1.ServiceInstance {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		return isInstanceShared(si)
	}, timeout*4, interval).Should(BeTrue())
	return si
}

func waitForInstanceToBeUnShared(ctx context.Context, key types.NamespacedName) *v1.ServiceInstance {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		return !isInstanceShared(si)
	}, timeout, interval).Should(BeTrue())
	return si
}

func waitForInstanceID(ctx context.Context, key types.NamespacedName, id string) {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		return si.Status.InstanceID == id
	}, timeout, interval).Should(BeTrue())
}

func waitForInstanceToBeInProgress(ctx context.Context, key types.NamespacedName) {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, si)
		return err == nil && isInProgress(si)
	}, timeout, interval).Should(BeTrue())
}

func waitForInstanceToBeFailedWithMsg(ctx context.Context, key types.NamespacedName, msg string) {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(si.GetConditions(), api.ConditionFailed)
		return cond != nil && isFailed(si) && strings.Contains(cond.Message, msg)
	}, timeout, interval).Should(BeTrue())
}

func validateInstanceGotDeleted(ctx context.Context, key types.NamespacedName) {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, si)
		return apierrors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

func waitForInstanceConditionAndReason(ctx context.Context, key types.NamespacedName, conditionType, reason string) *v1.ServiceInstance {
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(si.GetConditions(), conditionType)
		return cond != nil && cond.Reason == reason
	}, timeout, interval).Should(BeTrue())
	return si
}

func getNonTransientBrokerError(errMessage string) error {
	return &sm.ServiceManagerError{
		StatusCode:  http.StatusBadRequest,
		Description: "smErrMessage",
		BrokerError: &api.HTTPStatusCodeError{
			StatusCode:   400,
			ErrorMessage: &errMessage,
		}}
}

func getTransientBrokerError(errorMessage string) error {
	return &sm.ServiceManagerError{
		StatusCode:  http.StatusBadGateway,
		Description: "smErrMessage",
		BrokerError: &api.HTTPStatusCodeError{
			StatusCode:   http.StatusTooManyRequests,
			ErrorMessage: &errorMessage,
		},
	}
}

func waitForInstanceCreationFailure(ctx context.Context, defaultLookupKey types.NamespacedName, serviceInstance *v1.ServiceInstance, errMessage string) {
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(serviceInstance.Status.Conditions, api.ConditionSucceeded)
		return cond != nil && cond.Status == metav1.ConditionFalse && strings.Contains(cond.Message, errMessage)
	}, timeout, interval).Should(BeTrue())
}

func instanceSharingReturnSuccess() {
	fakeClient.ShareInstanceReturns(nil)
}

func instanceUnSharingReturnsNonTransientError() {
	fakeClient.UnShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode:  http.StatusBadRequest,
		Description: "nonTransient",
	})
}

func instanceSharingReturnsNonTransientError400() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode:  http.StatusBadRequest,
		Description: "nonTransient",
	})
}

func instanceSharingReturnsNonTransientError500() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode:  http.StatusInternalServerError,
		Description: "nonTransient",
	})
}

func instanceSharingReturnsTransientError() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode:  http.StatusServiceUnavailable,
		Description: "transient",
	})
}

func instanceSharingReturnsRateLimitError() {
	fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
		StatusCode:  http.StatusTooManyRequests,
		Description: "transient",
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

	Expect(k8sClient.Create(context.Background(), secret)).ToNot(HaveOccurred())
}

func isInstanceShared(serviceInstance *v1.ServiceInstance) bool {
	conditions := serviceInstance.GetConditions()
	if conditions == nil {
		return false
	}

	sharedCond := meta.FindStatusCondition(conditions, api.ConditionShared)
	return sharedCond != nil && sharedCond.Status == metav1.ConditionTrue
}

func markInstanceAsPreventDeletion(serviceInstance *v1.ServiceInstance) {
	serviceInstance.Annotations = map[string]string{
		api.PreventDeletion: "true",
	}
}

func updateInstance(ctx context.Context, serviceInstance *v1.ServiceInstance) *v1.ServiceInstance {
	err := k8sClient.Update(ctx, serviceInstance)
	if err == nil {
		return serviceInstance
	}

	key := types.NamespacedName{Name: serviceInstance.Name, Namespace: serviceInstance.Namespace}
	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, si); err != nil {
			return false
		}
		si.Spec = serviceInstance.Spec
		si.Labels = serviceInstance.Labels
		si.Annotations = serviceInstance.Annotations
		return k8sClient.Update(ctx, si) == nil
	}, timeout, interval).Should(BeTrue())

	return si
}

func updateInstanceStatus(ctx context.Context, instance *v1.ServiceInstance) *v1.ServiceInstance {
	err := k8sClient.Status().Update(ctx, instance)
	if err == nil {
		return instance
	}

	si := &v1.ServiceInstance{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, si); err != nil {
			return false
		}
		si.Status = instance.Status
		return k8sClient.Status().Update(ctx, si) == nil
	}, timeout, interval).Should(BeTrue())
	return si
}

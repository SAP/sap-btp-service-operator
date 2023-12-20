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
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
	"time"
)

// +kubebuilder:docs-gen:collapse=Imports

const (
	fakeInstanceID           = "ic-fake-instance-id"
	fakeSubaccountID         = "fake-subaccount-id"
	fakeInstanceExternalName = "ic-test-instance-external-name"
	testNamespace            = "ic-test-namespace"
	fakeOfferingName         = "offering-a"
	fakePlanName             = "plan-a"
)

var _ = Describe("ServiceInstance controller", func() {
	var (
		ctx context.Context

		serviceInstance  *v1.ServiceInstance
		fakeInstanceName string
		defaultLookupKey types.NamespacedName
	)

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
		Shared:              pointer.Bool(true),
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

	createInstance := func(ctx context.Context, instanceSpec v1.ServiceInstanceSpec, annotations map[string]string, waitForReady bool) *v1.ServiceInstance {
		instance := &v1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "services.cloud.sap.com/v1",
				Kind:       "ServiceInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        fakeInstanceName,
				Namespace:   testNamespace,
				Annotations: annotations,
			},
			Spec: instanceSpec,
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		if !waitForReady {
			return instance
		}
		waitForResourceToBeReady(ctx, instance)
		return instance
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
		log := ctrl.Log.WithName("instanceTest")
		ctx = context.WithValue(ctx, LogKey{}, log)
		fakeInstanceName = "ic-test-" + uuid.New().String()
		defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: testNamespace}

		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID, SubaccountID: fakeSubaccountID}, nil)
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
						serviceInstance = createInstance(ctx, instanceSpec, nil, false)
						waitForInstanceConditionAndMessage(ctx, defaultLookupKey, api.ConditionSucceeded, "provided plan id does not match")
					})
				})
			})
		})

		Context("Sync", func() {
			When("provision request to SM succeeds", func() {
				It("should provision instance of the provided offering and plan name successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, true)
					Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
					Expect(serviceInstance.Status.SubaccountID).To(Equal(fakeSubaccountID))
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
				errMessage := "failed to provision instance"

				Context("provision fails once and then succeeds", func() {
					BeforeEach(func() {
						fakeClient.ProvisionReturnsOnCall(0, nil, &sm.ServiceManagerError{
							StatusCode:  http.StatusBadRequest,
							Description: errMessage,
						})
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should have failure condition", func() {
						serviceInstance = createInstance(ctx, instanceSpec, nil, false)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, CreateFailed, errMessage)
					})
				})

				Context("ignoreNonTransientErrorAnnotation exists", func() {
					When("provision fails once and then succeeds", func() {
						It("should remove the annotation", func() {
							fakeClient.ProvisionReturnsOnCall(0, nil, &sm.ServiceManagerError{
								StatusCode:  http.StatusBadRequest,
								Description: errMessage,
							})
							fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
							serviceInstance = createInstance(ctx, instanceSpec, map[string]string{api.IgnoreNonTransientErrorAnnotation: "true"}, false)
							waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionTrue, Created, "")
							waitForResourceAnnotationRemove(ctx, serviceInstance, api.IgnoreNonTransientErrorAnnotation, api.IgnoreNonTransientErrorTimestampAnnotation)
						})
					})

					When("provision fails until timeout", func() {
						It("should have failure conditions and remove the annotation", func() {
							fakeClient.ProvisionReturns(nil, &sm.ServiceManagerError{
								StatusCode:  http.StatusBadRequest,
								Description: errMessage,
							})
							serviceInstance = createInstance(ctx, instanceSpec, map[string]string{api.IgnoreNonTransientErrorAnnotation: "true"}, false)
							waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, CreateFailed, errMessage)
							waitForResourceAnnotationRemove(ctx, serviceInstance, api.IgnoreNonTransientErrorAnnotation, api.IgnoreNonTransientErrorTimestampAnnotation)
							waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, CreateFailed, errMessage)
							sinceCreate := time.Since(serviceInstance.GetCreationTimestamp().Time)
							Expect(sinceCreate > ignoreNonTransientTimeout)
						})
					})
				})

				Context("with 429 status eventually succeeds", func() {
					BeforeEach(func() {
						fakeClient.ProvisionReturnsOnCall(0, nil, &sm.ServiceManagerError{
							StatusCode:  http.StatusTooManyRequests,
							Description: errMessage,
						})
						fakeClient.ProvisionReturnsOnCall(1, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should retry until success", func() {
						serviceInstance = createInstance(ctx, instanceSpec, nil, true)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionTrue, Created, "")
					})
				})

				Context("with sm status code 502 and broker status code 429", func() {
					tooManyRequestsError := "broker too many requests"
					BeforeEach(func() {
						fakeClient.ProvisionReturns(nil, getTransientBrokerError(tooManyRequestsError))
					})

					It("should be transient error and eventually succeed", func() {
						serviceInstance = createInstance(ctx, instanceSpec, nil, false)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, CreateInProgress, tooManyRequestsError)
						fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
						waitForResourceToBeReady(ctx, serviceInstance)
					})
				})

				Context("with sm status code 502 and broker status code 400", func() {
					BeforeEach(func() {
						fakeClient.ProvisionReturns(nil, getNonTransientBrokerError(errMessage))
						fakeClient.ProvisionReturnsOnCall(2, &sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
					})

					It("should have failure condition - non transient error", func() {
						serviceInstance = createInstance(ctx, instanceSpec, nil, false)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, CreateFailed, errMessage)
					})

					When("ignoreNonTransientErrorAnnotation exists", func() {
						It("should have failure conditions and remove the annotation after timeout", func() {
							serviceInstance = createInstance(ctx, instanceSpec, map[string]string{api.IgnoreNonTransientErrorAnnotation: "true"}, false)
							waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionTrue, Created, "")
							waitForResourceAnnotationRemove(ctx, serviceInstance, api.IgnoreNonTransientErrorAnnotation, api.IgnoreNonTransientErrorTimestampAnnotation)
							Expect(fakeClient.ProvisionCallCount()).To(BeNumerically(">", 1))
						})
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
				BeforeEach(func() {
					fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{Labels: map[string][]string{"subaccount_id": {fakeSubaccountID}}}, nil)
				})
				It("should update in progress condition and provision the instance successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, false)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.SUCCEEDED,
					}, nil)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionTrue, Created, "")
					Expect(serviceInstance.Status.SubaccountID).To(Equal(fakeSubaccountID))
				})
			})

			When("polling ends with failure", func() {
				It("should update to failure condition with the broker err description", func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, false)
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
				It("should update the instance after created successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, false)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, CreateInProgress, "")
					newName := "new-name" + uuid.New().String()

					Eventually(func() bool {
						if err := k8sClient.Get(ctx, defaultLookupKey, serviceInstance); err != nil {
							return false
						}

						serviceInstance.Spec.ExternalName = newName
						return k8sClient.Update(ctx, serviceInstance) == nil
					}, timeout, interval).Should(BeTrue())

					Expect(fakeClient.UpdateInstanceCallCount()).To(Equal(0))

					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.CREATE,
						State: smClientTypes.SUCCEEDED,
					}, nil)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionTrue, Updated, "")
					Expect(fakeClient.UpdateInstanceCallCount()).To(BeNumerically(">", 0))
					Expect(fakeClient.ProvisionCallCount()).To(BeNumerically(">", 0))
				})
			})

			When("deleting while create is in progress", func() {
				It("should be deleted successfully", func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, false)

					By("waiting for instance to be CreateInProgress")
					waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, CreateInProgress, "")

					fakeClient.DeprovisionReturns("/v1/service_instances/id/operations/1234", nil)
					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.DELETE,
						State: smClientTypes.INPROGRESS,
					}, nil)

					deleteInstance(ctx, serviceInstance, false)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, DeleteInProgress, "")

					fakeClient.StatusReturns(&smclientTypes.Operation{
						ID:    "1234",
						Type:  smClientTypes.DELETE,
						State: smClientTypes.SUCCEEDED,
					}, nil)

					waitForResourceToBeDeleted(ctx, getResourceNamespacedName(serviceInstance), serviceInstance)
				})
			})
		})

		When("external name is not provided", func() {
			It("succeeds and uses the k8s name as external name", func() {
				withoutExternal := v1.ServiceInstanceSpec{
					ServicePlanName:     "a-plan-name",
					ServiceOfferingName: "an-offering-name",
				}
				serviceInstance = createInstance(ctx, withoutExternal, nil, true)
				Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
				Expect(serviceInstance.Spec.ExternalName).To(Equal(fakeInstanceName))
				Expect(serviceInstance.Name).To(Equal(fakeInstanceName))
			})
		})
	})

	Describe("Update", func() {
		BeforeEach(func() {
			serviceInstance = createInstance(ctx, instanceSpec, nil, true)
			Expect(serviceInstance.Spec.ExternalName).To(Equal(fakeInstanceExternalName))
		})

		Context("When update call to SM succeed", func() {
			Context("Sync", func() {
				When("spec is changed", func() {
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "", nil)
					})
					It("condition should be Updated", func() {
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						serviceInstance = updateInstance(ctx, serviceInstance)
						Expect(serviceInstance.Spec.ExternalName).To(Equal(newExternalName))
						Expect(serviceInstance.Spec.UserInfo).NotTo(BeNil())
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionTrue, Updated, "")
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
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, UpdateInProgress, "")
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.SUCCEEDED,
						}, nil)
						waitForResourceToBeReady(ctx, serviceInstance)
						Expect(serviceInstance.Status.InstanceID).ToNot(BeEmpty())
						Expect(serviceInstance.Spec.ExternalName).To(Equal(newExternalName))
					})

					When("updating during update", func() {
						It("should save the latest spec", func() {
							newExternalName := "my-new-external-name" + uuid.New().String()
							serviceInstance.Spec.ExternalName = newExternalName
							updatedInstance := updateInstance(ctx, serviceInstance)

							updatedInstance.Spec.ExternalName = newExternalName + "-new"
							updateInstance(ctx, updatedInstance)

							fakeClient.StatusReturns(&smclientTypes.Operation{
								ID:    "1234",
								Type:  smClientTypes.UPDATE,
								State: smClientTypes.SUCCEEDED,
							}, nil)

							Eventually(func() bool {
								_ = k8sClient.Get(ctx, defaultLookupKey, serviceInstance)
								return isResourceReady(serviceInstance) && serviceInstance.Spec.ExternalName == newExternalName+"-new"
							}, timeout, interval).Should(BeTrue())
						})
					})

					When("deleting during update", func() {
						It("should be deleted", func() {
							newExternalName := "my-new-external-name" + uuid.New().String()
							serviceInstance.Spec.ExternalName = newExternalName
							updatedInstance := updateInstance(ctx, serviceInstance)
							deleteInstance(ctx, updatedInstance, false)

							fakeClient.StatusReturns(&smclientTypes.Operation{
								ID:    "1234",
								Type:  smClientTypes.UPDATE,
								State: smClientTypes.SUCCEEDED,
							}, nil)

							waitForResourceToBeDeleted(ctx, getResourceNamespacedName(serviceInstance), serviceInstance)
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
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, UpdateFailed, "")
					})
				})
			})

			Context("spec is changed and sm returns 502", func() {
				When("the error is transient", func() {
					errMessage := "broker too many requests"
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "", getTransientBrokerError(errMessage))
					})

					It("recognize the error as transient and eventually succeed", func() {
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, UpdateInProgress, "")
						fakeClient.UpdateInstanceReturns(nil, "", nil)
						updateInstance(ctx, serviceInstance)
						waitForResourceToBeReady(ctx, serviceInstance)
					})
				})

				When("the error is non transient but ignoreNonTransientErrorAnnotation exists", func() {
					errMessage := "broker update error"
					BeforeEach(func() {
						fakeClient.UpdateInstanceReturns(nil, "", getNonTransientBrokerError(errMessage))
					})
					It("recognizes the error as transient and eventually succeed", func() {
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, UpdateFailed, "")
						serviceInstance.Annotations = map[string]string{api.IgnoreNonTransientErrorAnnotation: "true"}
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, UpdateInProgress, "")
						fakeClient.UpdateInstanceReturns(nil, "", nil)

						waitForResourceToBeReady(ctx, serviceInstance)
						waitForResourceAnnotationRemove(ctx, serviceInstance, api.IgnoreNonTransientErrorAnnotation, api.IgnoreNonTransientErrorTimestampAnnotation)

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
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, UpdateInProgress, "")
						fakeClient.StatusReturns(&smclientTypes.Operation{
							ID:    "1234",
							Type:  smClientTypes.UPDATE,
							State: smClientTypes.FAILED,
						}, nil)
						waitForResourceToBeReady(ctx, serviceInstance)
					})
				})

				//TODO: fix the test
				XWhen("Instance has operation url to operation that no longer exist in SM", func() {
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
						newExternalName := "my-new-external-name" + uuid.New().String()
						serviceInstance.Spec.ExternalName = newExternalName
						ui := updateInstance(ctx, serviceInstance)
						waitForResourceToBeReady(ctx, ui)
					})
				})
			})
		})

		When("subaccount id changed", func() {
			It("should fail", func() {
				deleteInstance(ctx, serviceInstance, true)
				serviceInstance = createInstance(ctx, instanceSpec, nil, true)
				serviceInstance.Spec.BTPAccessCredentialsSecret = "12345"
				err := k8sClient.Update(ctx, serviceInstance)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("changing the btpAccessCredentialsSecret for an existing instance is not allowed"))
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			serviceInstance = createInstance(ctx, instanceSpec, nil, true)
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
					serviceInstance.Annotations = map[string]string{
						api.PreventDeletion: "true",
					}
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
					errMsg := "failed to delete instance"
					fakeClient.DeprovisionReturns("", fmt.Errorf(errMsg))
					deleteInstance(ctx, serviceInstance, false)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, DeleteFailed, errMsg)
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
				waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, DeleteInProgress, "")
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
					waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, DeleteFailed, "broker-failure")
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

	Describe("full reconcile", func() {
		When("instance hashedSpec is not initialized", func() {
			BeforeEach(func() {
				serviceInstance = createInstance(ctx, instanceSpec, nil, true)
			})
			It("should not send update request and update the hashed spec", func() {
				hashed := serviceInstance.Status.HashedSpec
				serviceInstance.Status.HashedSpec = ""
				Expect(k8sClient.Status().Update(ctx, serviceInstance)).To(Succeed())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: serviceInstance.Namespace}, serviceInstance)
					if err != nil {
						return false
					}
					cond := meta.FindStatusCondition(serviceInstance.GetConditions(), api.ConditionSucceeded)
					return serviceInstance.Status.HashedSpec == hashed && cond != nil && cond.Reason == Created
				}, timeout, interval).Should(BeTrue())
			})
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
				serviceInstance = createInstance(ctx, instanceSpec, nil, true)
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
						serviceInstance = createInstance(ctx, instanceSpec, nil, false)
						key := getResourceNamespacedName(serviceInstance)
						Eventually(func() bool {
							_ = k8sClient.Get(ctx, key, serviceInstance)
							return serviceInstance.Status.InstanceID == fakeInstanceID
						}, timeout, interval).Should(BeTrue(), eventuallyMsgForResource("service instance id not recovered", key, serviceInstance))
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
						Expect(fakeClient.ListInstancesCallCount()).To(BeNumerically(">", 0))
						fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}, nil)
						waitForResourceToBeReady(ctx, serviceInstance)
					})
				})

				When("last operation state is FAILED", func() {
					BeforeEach(func() {
						recoveredInstance.LastOperation = &smClientTypes.Operation{State: smClientTypes.FAILED, Type: smClientTypes.CREATE}
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})

					It("should recover the existing instance and update condition failure", func() {
						serviceInstance = createInstance(ctx, instanceSpec, nil, false)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionSucceeded, metav1.ConditionFalse, CreateFailed, "")
						Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
					})
				})

				When("no last operation", func() {
					JustBeforeEach(func() {
						recoveredInstance.LastOperation = nil
						fakeClient.ListInstancesReturns(&smclientTypes.ServiceInstances{
							ServiceInstances: []smclientTypes.ServiceInstance{recoveredInstance}}, nil)
					})
					When("instance is ready in SM", func() {
						BeforeEach(func() {
							recoveredInstance.Ready = true
						})
						It("should recover the instance with status Ready=true", func() {
							serviceInstance = createInstance(ctx, instanceSpec, nil, false)
							waitForResourceToBeReady(ctx, serviceInstance)
							Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
							Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						})
					})
					When("instance is not ready in SM", func() {
						BeforeEach(func() {
							recoveredInstance.Ready = false
						})
						It("should recover the instance with status Ready=false", func() {
							serviceInstance = createInstance(ctx, instanceSpec, nil, false)
							waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, CreateFailed, "")
							Expect(fakeClient.ProvisionCallCount()).To(Equal(0))
							Expect(serviceInstance.Status.InstanceID).To(Equal(fakeInstanceID))
						})
					})

				})
			})
		})
	})

	Describe("Share instance", func() {
		Context("Share", func() {
			When("creating instance with shared=true", func() {
				It("should succeed to provision and sharing the instance", func() {
					fakeClient.ShareInstanceReturns(nil)
					serviceInstance = createInstance(ctx, sharedInstanceSpec, nil, true)
					waitForInstanceToBeShared(ctx, serviceInstance)
				})
			})

			Context("sharing an existing instance", func() {
				BeforeEach(func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, true)
				})

				When("updating existing instance to shared", func() {
					It("should succeed", func() {
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeShared(ctx, serviceInstance)
					})
				})

				When("sharing succeeds", func() {
					It("hashed spec should be the same before and after", func() {
						originalHashedSpec := serviceInstance.Status.HashedSpec
						fakeClient.UpdateInstanceReturns(nil, "", nil)
						serviceInstance.Spec.Shared = pointer.Bool(true)
						fakeClient.ShareInstanceReturns(nil)
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeShared(ctx, serviceInstance)
						newHashedSpec := serviceInstance.Status.HashedSpec
						Expect(originalHashedSpec).To(Equal(newHashedSpec))
					})
				})

				When("updating instance to shared and updating the name", func() {
					It("eventually should succeed updating both", func() {
						serviceInstance.Spec.Shared = pointer.Bool(true)
						serviceInstance.Spec.ExternalName = "new"
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeShared(ctx, serviceInstance)
						Expect(fakeClient.UpdateInstanceCallCount()).To(Equal(1))
					})
				})
			})

			When("instance creation failed", func() {
				It("should not attempt to share the instance", func() {
					fakeClient.ProvisionReturns(nil, &sm.ServiceManagerError{
						StatusCode:  http.StatusBadRequest,
						Description: "errMessage",
					})
					serviceInstance = createInstance(ctx, sharedInstanceSpec, nil, false)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionFailed, metav1.ConditionTrue, CreateFailed, "")
					Expect(fakeClient.ShareInstanceCallCount()).To(BeZero())
				})
			})

			When("instance is valid and share failed", func() {
				BeforeEach(func() {
					serviceInstance = createInstance(ctx, instanceSpec, nil, true)
				})

				When("shared failed with rate limit error", func() {
					It("status should be shared in progress", func() {
						fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
							StatusCode:  http.StatusTooManyRequests,
							Description: "transient",
						})
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionFalse, InProgress, "")
						fakeClient.ShareInstanceReturns(nil)
						waitForInstanceToBeShared(ctx, serviceInstance)
					})
				})

				When("shared failed with transient error which is not rate limit", func() {
					It("status should be shared failed and eventually succeed sharing", func() {
						fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
							StatusCode:  http.StatusServiceUnavailable,
							Description: "transient",
						})
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionFalse, ShareFailed, "")

						fakeClient.ShareInstanceReturns(nil)
						waitForInstanceToBeShared(ctx, serviceInstance)
					})
				})

				When("shared failed with non transient error 400", func() {
					It("should have a final shared status", func() {
						fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
							StatusCode:  http.StatusBadRequest,
							Description: "nonTransient",
						})
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionFalse, ShareNotSupported, "")
					})
				})

				When("shared failed with non transient error 500", func() {
					It("should have a final shared status", func() {
						fakeClient.ShareInstanceReturns(&sm.ServiceManagerError{
							StatusCode:  http.StatusInternalServerError,
							Description: "nonTransient",
						})
						serviceInstance.Spec.Shared = pointer.Bool(true)
						updateInstance(ctx, serviceInstance)
						waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionFalse, ShareNotSupported, "")
					})
				})
			})
		})

		Context("Un-Share", func() {
			Context("un-sharing an existing shared instance", func() {
				BeforeEach(func() {
					fakeClient.ShareInstanceReturns(nil)
					serviceInstance = createInstance(ctx, sharedInstanceSpec, nil, true)
					waitForInstanceToBeShared(ctx, serviceInstance)
				})

				When("updating instance to un-shared and sm return success", func() {
					It("should succeed", func() {
						serviceInstance.Spec.Shared = pointer.Bool(false)
						fakeClient.UnShareInstanceReturns(nil)
						updateInstance(ctx, serviceInstance)
						waitForInstanceToBeUnShared(ctx, serviceInstance)
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
						waitForInstanceToBeUnShared(ctx, serviceInstance)
						Expect(fakeClient.UpdateInstanceCallCount()).To(Equal(1))
					})
				})
			})

			When("instance is valid and un-share failed", func() {
				BeforeEach(func() {
					fakeClient.ShareInstanceReturns(nil)
					serviceInstance = createInstance(ctx, sharedInstanceSpec, nil, true)
					waitForInstanceToBeShared(ctx, serviceInstance)
					fakeClient.UnShareInstanceReturns(&sm.ServiceManagerError{
						StatusCode:  http.StatusBadRequest,
						Description: "nonTransient",
					})
				})

				It("should have a reason un-shared failed", func() {
					serviceInstance.Spec.Shared = pointer.Bool(false)
					updateInstance(ctx, serviceInstance)
					waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionTrue, UnShareFailed, "")
				})
			})
		})
	})

	Context("Unit Tests", func() {
		Context("isFinalState", func() {
			When("Succeeded condition is not for current generation", func() {
				It("should be false", func() {
					var instance = &v1.ServiceInstance{
						Status: v1.ServiceInstanceStatus{
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
								},
							},
							HashedSpec: "929e78f4449f8036ce39da3cc3e7eaea",
						},
						Spec: v1.ServiceInstanceSpec{
							ExternalName: "name",
						}}
					instance.SetGeneration(2)
					Expect(isFinalState(ctx, instance)).To(BeFalse())
				})

				When("Succeeded is false", func() {
					It("should return false", func() {
						var instance = &v1.ServiceInstance{
							Status: v1.ServiceInstanceStatus{
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
									},
								},
								HashedSpec: "929e78f4449f8036ce39da3cc3e7eaea",
							},
							Spec: v1.ServiceInstanceSpec{
								ExternalName: "name",
							},
						}
						instance.SetGeneration(1)
						Expect(isFinalState(ctx, instance)).To(BeFalse())
					})
				})
			})

			When("async operation in progress", func() {
				It("should return false", func() {
					var instance = &v1.ServiceInstance{
						Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
							},
							HashedSpec:         "929e78f4449f8036ce39da3cc3e7eaea",
							OperationURL:       "/operations/somepollingurl",
							ObservedGeneration: 2,
						},
						Spec: v1.ServiceInstanceSpec{
							ExternalName: "name",
						}}
					instance.SetGeneration(2)
					Expect(isFinalState(ctx, instance)).To(BeFalse())
				})
			})

			When("in progress", func() {
				It("should return false", func() {
					var instance = &v1.ServiceInstance{
						Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionFalse,
									ObservedGeneration: 2,
								},
							},
							HashedSpec: "929e78f4449f8036ce39da3cc3e7eaea",
						},
						Spec: v1.ServiceInstanceSpec{
							ExternalName: "name",
						}}
					instance.SetGeneration(2)
					Expect(isFinalState(ctx, instance)).To(BeFalse())
				})
			})

			When("spec changed", func() {
				It("should return false", func() {
					var instance = &v1.ServiceInstance{
						Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 2,
								},
							},
							HashedSpec: "bla",
						},
						Spec: v1.ServiceInstanceSpec{
							ExternalName: "name",
						}}
					instance.SetGeneration(2)
					Expect(isFinalState(ctx, instance)).To(BeFalse())
				})
			})

			When("sharing update is required", func() {
				It("should return false", func() {
					var instance = &v1.ServiceInstance{
						Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 2,
								},
								{
									Type:   api.ConditionShared,
									Status: metav1.ConditionFalse,
								},
							},
							HashedSpec: "929e78f4449f8036ce39da3cc3e7eaea",
							Ready:      metav1.ConditionTrue,
						},
						Spec: v1.ServiceInstanceSpec{
							ExternalName: "name",
							Shared:       pointer.Bool(true),
						}}
					instance.SetGeneration(2)
					Expect(isFinalState(ctx, instance)).To(BeFalse())
				})
			})

			When("in final state", func() {
				It("should return true", func() {
					var instance = &v1.ServiceInstance{
						Status: v1.ServiceInstanceStatus{
							Conditions: []metav1.Condition{
								{
									Type:               api.ConditionReady,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               api.ConditionSucceeded,
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 2,
								},
								{
									Type:   api.ConditionShared,
									Status: metav1.ConditionTrue,
								},
							},
							HashedSpec:         "929e78f4449f8036ce39da3cc3e7eaea",
							ObservedGeneration: 2,
						},
						Spec: v1.ServiceInstanceSpec{
							ExternalName: "name",
							Shared:       pointer.Bool(true),
						}}
					instance.SetGeneration(2)
					Expect(isFinalState(ctx, instance)).To(BeTrue())
				})
			})
		})
	})
})

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

func waitForInstanceToBeShared(ctx context.Context, serviceInstance *v1.ServiceInstance) {
	waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionTrue, "", "")
	Expect(len(serviceInstance.Status.Conditions)).To(Equal(3))
}

func waitForInstanceToBeUnShared(ctx context.Context, serviceInstance *v1.ServiceInstance) {
	waitForResourceCondition(ctx, serviceInstance, api.ConditionShared, metav1.ConditionFalse, "", "")
	Expect(len(serviceInstance.Status.Conditions)).To(Equal(3))
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
		err := k8sClient.Update(ctx, si)
		return err == nil
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

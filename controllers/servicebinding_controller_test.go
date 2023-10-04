package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"k8s.io/utils/pointer"
	"net/http"
	"strings"

	"github.com/SAP/sap-btp-service-operator/api"
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/SAP/sap-btp-service-operator/client/sm/smfakes"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"fmt"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("ServiceBinding controller", func() {
	var (
		ctx context.Context

		createdInstance *v1.ServiceInstance
		createdBinding  *v1.ServiceBinding

		defaultLookupKey types.NamespacedName

		testUUID             string
		bindingName          string
		instanceName         string
		instanceExternalName string
	)

	createBindingWithoutAssertionsAndWait := func(ctx context.Context, name, namespace, instanceName, instanceNamespace, externalName string, wait bool) (*v1.ServiceBinding, error) {
		binding := generateBasicBindingTemplate(name, namespace, instanceName, instanceNamespace, externalName)

		if err := k8sClient.Create(ctx, binding); err != nil {
			return nil, err
		}

		bindingLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
		createdBinding = &v1.ServiceBinding{}

		Eventually(func() bool {
			if err := k8sClient.Get(ctx, bindingLookupKey, createdBinding); err != nil {
				return false
			}

			if wait {
				return isResourceReady(createdBinding) || isFailed(createdBinding)
			} else {
				return len(createdBinding.Status.Conditions) > 0 && createdBinding.Status.Conditions[0].Message != "Pending"
			}
		}, timeout, interval).Should(BeTrue())
		return createdBinding, nil
	}

	createBindingWithoutAssertions := func(ctx context.Context, name, namespace, instanceName, instanceNamespace, externalName string) (*v1.ServiceBinding, error) {
		return createBindingWithoutAssertionsAndWait(ctx, name, namespace, instanceName, instanceNamespace, externalName, true)
	}

	createBindingWithError := func(ctx context.Context, name, namespace, instanceName, externalName, failureMessage string) {
		binding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, "", externalName)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			waitForResourceCondition(ctx, binding, api.ConditionFailed, metav1.ConditionTrue, "", failureMessage)
		}
	}

	createBindingWithBlockedError := func(ctx context.Context, name, namespace, instanceName, externalName, failureMessage string) *v1.ServiceBinding {
		binding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, "", externalName)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			waitForResourceCondition(ctx, binding, api.ConditionSucceeded, metav1.ConditionFalse, "", failureMessage)
		}
		return binding
	}

	createBinding := func(ctx context.Context, name, namespace, instanceName, instanceNamespace, externalName string) *v1.ServiceBinding {
		createdBinding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, instanceNamespace, externalName)
		Expect(err).ToNot(HaveOccurred())
		Expect(createdBinding.Status.InstanceID).ToNot(BeEmpty())
		Expect(createdBinding.Status.BindingID).To(Equal(fakeBindingID))
		Expect(createdBinding.Spec.SecretName).To(Not(BeEmpty()))
		Expect(int(createdBinding.Status.ObservedGeneration)).To(Equal(1))
		Expect(string(createdBinding.Spec.Parameters.Raw)).To(ContainSubstring("\"key\":\"value\""))
		smBinding, _, _ := fakeClient.BindArgsForCall(0)
		params := smBinding.Parameters
		Expect(params).To(ContainSubstring("\"key\":\"value\""))
		Expect(params).To(ContainSubstring("\"secret-key\":\"secret-value\""))
		return createdBinding
	}

	createInstance := func(ctx context.Context, name, namespace, externalName string) *v1.ServiceInstance {
		instance := &v1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "services.cloud.sap.com/v1",
				Kind:       "ServiceInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1.ServiceInstanceSpec{
				ExternalName:        externalName,
				ServicePlanName:     "a-plan-name",
				ServiceOfferingName: "an-offering-name",
				CustomTags:          []string{"custom-tag"},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())
		waitForResourceToBeReady(ctx, instance)
		Expect(instance.Status.InstanceID).ToNot(BeEmpty())
		return instance
	}

	validateInstanceInfo := func(bindingSecret *corev1.Secret, instanceName string) {
		validateSecretData(bindingSecret, "plan", `a-plan-name`)
		validateSecretData(bindingSecret, "label", `an-offering-name`)
		validateSecretData(bindingSecret, "type", `an-offering-name`)
		validateSecretData(bindingSecret, "tags", "[\"test\",\"custom-tag\"]")
		validateSecretData(bindingSecret, "instance_name", instanceName)
		Expect(bindingSecret.Data).To(HaveKey("instance_guid"))
	}

	validateSecretMetadata := func(bindingSecret *corev1.Secret, credentialProperties []SecretMetadataProperty) {
		metadata := make(map[string][]SecretMetadataProperty)
		Expect(json.Unmarshal(bindingSecret.Data[".metadata"], &metadata)).To(Succeed())
		if credentialProperties != nil {
			Expect(metadata["credentialProperties"]).To(ContainElements(credentialProperties))
		}
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "instance_name", Format: string(TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "instance_guid", Format: string(TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "plan", Format: string(TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "label", Format: string(TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "type", Format: string(TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "tags", Format: string(JSON)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "subaccount_id", Format: string(TEXT)}))
	}

	BeforeEach(func() {
		ctx = context.Background()
		testUUID = uuid.New().String()
		instanceName = "test-instance-" + testUUID
		bindingName = "test-binding-" + testUUID
		instanceExternalName = instanceName + "-external"

		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: "12345678", SubaccountID: "1234", Tags: []byte("[\"test\"]")}, nil)
		fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage(`{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`)}, "", nil)

		smInstance := &smClientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.UPDATE}}
		fakeClient.GetInstanceByIDReturns(smInstance, nil)

		defaultLookupKey = types.NamespacedName{Namespace: bindingTestNamespace, Name: bindingName}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: bindingTestNamespace, Name: "param-secret"}, &corev1.Secret{})
		if apierrors.IsNotFound(err) {
			createParamsSecret(bindingTestNamespace)
		} else {
			Expect(err).ToNot(HaveOccurred())
		}

		createdInstance = createInstance(ctx, instanceName, bindingTestNamespace, instanceExternalName)
	})

	AfterEach(func() {
		if createdBinding != nil {
			fakeClient.UnbindReturns("", nil)
			deleteAndWait(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: createdBinding.Namespace}, &v1.ServiceBinding{})
		}

		if createdInstance != nil {
			deleteAndWait(ctx, types.NamespacedName{Name: instanceName, Namespace: bindingTestNamespace}, &v1.ServiceInstance{})
		}

		createdBinding = nil
		createdInstance = nil
	})

	Context("Create", func() {
		Context("invalid parameters", func() {
			When("service instance name is not provided", func() {
				It("should fail", func() {
					createBindingWithError(ctx, bindingName, bindingTestNamespace, "", "",
						"spec.serviceInstanceName in body should be at least 1 chars long")
				})
			})

			When("referenced service instance does not exist", func() {
				It("should fail", func() {
					createBindingWithBlockedError(ctx, bindingName, bindingTestNamespace, "no-such-instance", "",
						"couldn't find the service instance")
				})
			})

			When("secret exists", func() {
				var (
					secret     *corev1.Secret
					secretName string
				)

				BeforeEach(func() {
					secretName = "mysecret-" + testUUID
					secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: bindingTestNamespace}}
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				})

				AfterEach(func() {
					deleteAndWait(ctx, types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace}, &corev1.Secret{})
				})

				When("name is already taken", func() {
					It("should fail the request and allow the user to replace secret name", func() {
						binding := newBindingObject(bindingName, bindingTestNamespace)
						binding.Spec.ServiceInstanceName = instanceName
						binding.Spec.SecretName = secretName
						Expect(k8sClient.Create(ctx, binding)).To(Succeed())
						waitForResourceCondition(ctx, binding, api.ConditionSucceeded, metav1.ConditionFalse, Blocked, fmt.Sprintf(secretNameTakenErrorFormat, secretName))
					})
				})

				When("secret is owned by another binding", func() {
					var owningBindingName = "owning-binding-name"
					BeforeEach(func() {
						secret.SetOwnerReferences([]metav1.OwnerReference{{
							APIVersion:         "services.cloud.sap.com/v1",
							Kind:               "ServiceBinding",
							Name:               owningBindingName,
							UID:                "111",
							BlockOwnerDeletion: pointer.Bool(true),
							Controller:         pointer.Bool(true),
						}})
						Expect(k8sClient.Update(ctx, secret)).To(Succeed())
					})
					It("should fail the request with relevant message and allow the user to replace secret name", func() {
						binding := newBindingObject(bindingName, bindingTestNamespace)
						binding.Spec.ServiceInstanceName = instanceName
						binding.Spec.SecretName = secretName
						Expect(k8sClient.Create(ctx, binding)).To(Succeed())

						waitForResourceCondition(ctx, binding, api.ConditionSucceeded, metav1.ConditionFalse, Blocked, fmt.Sprintf(secretAlreadyOwnedErrorFormat, secretName, owningBindingName))

						bindingLookupKey := getResourceNamespacedName(binding)
						binding.Spec.SecretName = secretName + "-new"
						updateBinding(ctx, bindingLookupKey, binding)
						waitForResourceToBeReady(ctx, binding)

						By("Verify binding secret created")
						bindingSecret := getSecret(ctx, binding.Spec.SecretName, binding.Namespace, true)
						Expect(bindingSecret).ToNot(BeNil())
					})
				})
			})
		})

		Context("sync", func() {
			It("Should create binding and store the binding credentials in a secret", func() {
				createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
				Expect(createdBinding.Spec.ExternalName).To(Equal("binding-external-name"))
				Expect(createdBinding.Spec.UserInfo).NotTo(BeNil())

				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateSecretData(bindingSecret, "secret_key", "secret_value")
				validateSecretData(bindingSecret, "escaped", `{"escaped_key":"escaped_val"}`)
				validateInstanceInfo(bindingSecret, instanceExternalName)
				credentialProperties := []SecretMetadataProperty{
					{
						Name:   "secret_key",
						Format: string(TEXT),
					},
					{
						Name:   "escaped",
						Format: string(TEXT),
					},
				}
				validateSecretMetadata(bindingSecret, credentialProperties)
			})

			It("should put the raw broker response into the secret if spec.secretKey is provided", func() {
				binding := newBindingObject("binding-with-secretkey", bindingTestNamespace)
				binding.Spec.ServiceInstanceName = instanceName
				binding.Spec.SecretName = "mysecret"
				secretKey := "mycredentials"
				binding.Spec.SecretKey = &secretKey
				Expect(k8sClient.Create(ctx, binding)).To(Succeed())

				waitForResourceToBeReady(ctx, binding)

				bindingSecret := getSecret(ctx, binding.Spec.SecretName, bindingTestNamespace, true)
				validateSecretData(bindingSecret, secretKey, `{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`)
				validateInstanceInfo(bindingSecret, instanceExternalName)
				credentialProperties := []SecretMetadataProperty{
					{
						Name:      "mycredentials",
						Format:    string(JSON),
						Container: true,
					},
				}
				validateSecretMetadata(bindingSecret, credentialProperties)
			})

			It("should put binding data in single key if spec.secretRootKey is provided", func() {
				binding := newBindingObject("binding-with-secretrootkey", bindingTestNamespace)
				binding.Spec.ServiceInstanceName = instanceName
				secretKey := "mycredentials"
				binding.Spec.SecretKey = &secretKey
				secretRootKey := "root"
				binding.Spec.SecretRootKey = &secretRootKey
				Expect(k8sClient.Create(ctx, binding)).To(Succeed())

				waitForResourceToBeReady(ctx, binding)

				bindingSecret := getSecret(ctx, binding.Spec.SecretName, bindingTestNamespace, true)
				Expect(len(bindingSecret.Data)).To(Equal(1))
				Expect(bindingSecret.Data).To(HaveKey("root"))
				res := make(map[string]string)
				Expect(json.Unmarshal(bindingSecret.Data["root"], &res)).To(Succeed())
				Expect(res[secretKey]).To(Equal(`{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`))
				Expect(res["plan"]).To(Equal("a-plan-name"))
				Expect(res["label"]).To(Equal("an-offering-name"))
				Expect(res["tags"]).To(Equal("[\"test\",\"custom-tag\"]"))
				Expect(res).To(HaveKey("instance_guid"))
			})

			When("secret deleted by user", func() {
				It("should recreate the secret", func() {
					createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
					secretLookupKey := types.NamespacedName{Name: createdBinding.Spec.SecretName, Namespace: createdBinding.Namespace}
					bindingSecret := getSecret(ctx, secretLookupKey.Name, secretLookupKey.Namespace, true)
					originalSecretUID := bindingSecret.UID
					fakeClient.ListBindingsReturns(&smClientTypes.ServiceBindings{
						ServiceBindings: []smClientTypes.ServiceBinding{
							{
								ID:          createdBinding.Status.BindingID,
								Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
								LastOperation: &smClientTypes.Operation{
									Type:        smClientTypes.CREATE,
									State:       smClientTypes.SUCCEEDED,
									Description: "fake-description",
								},
							},
						},
					}, nil)
					Expect(k8sClient.Delete(ctx, bindingSecret)).To(Succeed())

					newSecret := &corev1.Secret{}
					Eventually(func() bool {
						err := k8sClient.Get(ctx, secretLookupKey, newSecret)
						return err == nil && newSecret.UID != originalSecretUID
					}, timeout, interval).Should(BeTrue())
				})
			})

			When("bind call to SM returns error", func() {
				var errorMessage string

				When("general error occurred", func() {
					errorMessage = "no binding for you"
					BeforeEach(func() {
						fakeClient.BindReturns(nil, "", errors.New(errorMessage))
					})

					It("should fail with the error returned from SM", func() {
						createBindingWithError(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name",
							errorMessage)
					})
				})

				When("SM returned transient error(429)", func() {
					BeforeEach(func() {
						errorMessage = "too many requests"
						fakeClient.BindReturnsOnCall(0, nil, "", &sm.ServiceManagerError{
							StatusCode:  http.StatusTooManyRequests,
							Description: errorMessage,
						})
						fakeClient.BindReturnsOnCall(1, &smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
					})

					It("should eventually succeed", func() {
						b, err := createBindingWithoutAssertionsAndWait(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", true)
						Expect(err).ToNot(HaveOccurred())
						Expect(isResourceReady(b)).To(BeTrue())
					})
				})

				When("SM returned non transient error(400)", func() {
					BeforeEach(func() {
						errorMessage = "very bad request"
						fakeClient.BindReturnsOnCall(0, nil, "", &sm.ServiceManagerError{
							StatusCode:  http.StatusBadRequest,
							Description: errorMessage,
						})
					})

					It("should fail", func() {
						createBindingWithError(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name", errorMessage)
					})
				})

				When("SM returned error 502 and broker returned 429", func() {
					BeforeEach(func() {
						errorMessage = "too many requests from broker"
						fakeClient.BindReturns(nil, "", getTransientBrokerError(errorMessage))
					})

					It("should detect the error as transient and eventually succeed", func() {
						createdBinding, _ := createBindingWithoutAssertionsAndWait(ctx,
							bindingName,
							bindingTestNamespace,
							instanceName,
							"",
							"binding-external-name",
							false,
						)

						cond := meta.FindStatusCondition(createdBinding.GetConditions(), api.ConditionSucceeded)
						Expect(cond).To(Not(BeNil()))
						Expect(cond.Message).To(ContainSubstring(errorMessage))
						Expect(cond.Status).To(Equal(metav1.ConditionFalse))

						fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID,
							Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
						waitForResourceToBeReady(ctx, createdBinding)
					})
				})

				When("SM returned 502 and broker returned 400", func() {
					BeforeEach(func() {
						errorMessage = "very bad request"
						fakeClient.BindReturnsOnCall(0, nil, "", getNonTransientBrokerError(errorMessage))
					})

					It("should detect the error as non-transient and fail", func() {
						createBindingWithError(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name", errorMessage)
					})
				})

			})

			When("SM returned invalid credentials json", func() {
				BeforeEach(func() {
					fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("\"invalidjson\": \"secret_value\"")}, "", nil)
				})

				It("creation will fail with appropriate message", func() {
					createdBinding, _ = createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
					waitForResourceCondition(ctx, createdBinding, api.ConditionFailed, metav1.ConditionTrue, "CreateFailed", "failed to create secret")
				})
			})
		})

		Context("async", func() {
			BeforeEach(func() {
				fakeClient.BindReturns(nil, fmt.Sprintf("/v1/service_bindings/%s/operations/an-operation-id", fakeBindingID), nil)
			})

			When("bind polling returns success", func() {
				It("Should create binding and store the binding credentials in a secret", func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED}, nil)
					fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, nil)
					createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
				})
			})

			When("bind polling returns FAILED state", func() {
				It("should fail with the error returned from SM", func() {
					errorMessage := "no binding for you"
					fakeClient.StatusReturns(&smClientTypes.Operation{
						Type:        smClientTypes.CREATE,
						State:       smClientTypes.FAILED,
						Description: errorMessage,
					}, nil)
					createBindingWithError(ctx, bindingName, bindingTestNamespace, instanceName, "existing-name", errorMessage)
				})
			})

			// TODO redefine test
			XWhen("bind polling returns error", func() {
				BeforeEach(func() {
					fakeClient.StatusReturns(nil, fmt.Errorf("no polling for you"))
					fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				})
				It("should eventually succeed", func() {
					binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
					Expect(err).ToNot(HaveOccurred())
					waitForResourceCondition(ctx, binding, api.ConditionFailed, metav1.ConditionTrue, "", "no polling for you")
					fakeClient.ListBindingsReturns(&smClientTypes.ServiceBindings{
						ServiceBindings: []smClientTypes.ServiceBinding{
							{
								ID:          binding.Status.BindingID,
								Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
								LastOperation: &smClientTypes.Operation{
									Type:        smClientTypes.CREATE,
									State:       smClientTypes.SUCCEEDED,
									Description: "fake-description",
								},
							},
						},
					}, nil)
					waitForResourceToBeReady(ctx, binding)
					Expect(binding.Status.BindingID).To(Equal(fakeBindingID))
				})
			})
		})

		Context("useMetaName annotation is provided", func() {
			It("should put in the secret.instance_name the instance meta.name", func() {
				createdInstance.Annotations = map[string]string{
					api.UseInstanceMetadataNameInSecret: "true",
				}
				updateInstance(ctx, createdInstance)
				createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateInstanceInfo(bindingSecret, instanceName)
				validateSecretMetadata(bindingSecret, nil)
			})
		})

		When("external name is not provided", func() {
			It("succeeds and uses the k8s name as external name", func() {
				createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
				Expect(createdBinding.Spec.ExternalName).To(Equal(createdBinding.Name))
			})
		})

		When("secret name is provided", func() {
			It("should create a secret with the provided name", func() {
				binding := newBindingObject(bindingName, bindingTestNamespace)
				binding.Spec.ServiceInstanceName = instanceName
				binding.Spec.SecretName = "my-special-secret"
				Expect(k8sClient.Create(ctx, binding)).To(Succeed())
			})
		})

		When("referenced service instance is failed", func() {
			It("should retry and succeed once the instance is ready", func() {
				setFailureConditions(smClientTypes.CREATE, "Failed to create instance (test)", createdInstance)
				updateInstanceStatus(ctx, createdInstance)
				binding := createBindingWithBlockedError(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name", "is not usable")
				setSuccessConditions(smClientTypes.CREATE, createdInstance)
				updateInstanceStatus(ctx, createdInstance)
				waitForResourceToBeReady(ctx, binding)
			})
		})

		When("referenced service instance is not ready", func() {
			It("should retry and succeed once the instance is ready", func() {
				fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.INPROGRESS}, nil)
				setInProgressConditions(smClientTypes.CREATE, "", createdInstance)
				createdInstance.Status.OperationURL = "/1234"
				createdInstance.Status.OperationType = smClientTypes.CREATE
				updateInstanceStatus(ctx, createdInstance)

				createdBinding, err := createBindingWithoutAssertionsAndWait(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", false)
				Expect(err).ToNot(HaveOccurred())
				Expect(isInProgress(createdBinding)).To(BeTrue())

				setSuccessConditions(smClientTypes.CREATE, createdInstance)
				createdInstance.Status.OperationType = ""
				createdInstance.Status.OperationURL = ""
				updateInstanceStatus(ctx, createdInstance)
				waitForResourceToBeReady(ctx, createdBinding)
			})
		})
	})

	Context("Update", func() {
		BeforeEach(func() {
			createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
			Expect(isResourceReady(createdBinding)).To(BeTrue())
		})

		When("external name is changed", func() {
			It("should fail", func() {
				createdBinding.Spec.ExternalName = "new-external-name"
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

		When("service instance name is changed", func() {
			It("should fail", func() {
				createdBinding.Spec.ServiceInstanceName = "new-instance-name"
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

		When("parameters are changed", func() {
			It("should fail", func() {
				createdBinding.Spec.Parameters = &runtime.RawExtension{
					Raw: []byte(`{"new-key": "new-value"}`),
				}
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

		When("secretKey is changed", func() {
			It("should fail", func() {
				secretKey := "not-nil"
				createdBinding.Spec.SecretKey = &secretKey
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})
	})

	Context("Delete", func() {
		deleteAndValidate := func(binding *v1.ServiceBinding) {
			deleteAndWait(ctx, getResourceNamespacedName(createdBinding), &v1.ServiceBinding{})
			err := k8sClient.Get(ctx, types.NamespacedName{Name: binding.Spec.SecretName, Namespace: binding.Namespace}, &corev1.Secret{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}

		BeforeEach(func() {
			createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
			Expect(isResourceReady(createdBinding)).To(BeTrue())
		})

		Context("Sync", func() {
			When("delete in SM succeeds", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturns("", nil)
				})
				It("should delete the k8s binding and secret", func() {
					deleteAndValidate(createdBinding)
				})
			})

			When("delete when binding id is empty", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturns("", nil)
					fakeClient.ListBindingsReturns(&smClientTypes.ServiceBindings{
						ServiceBindings: []smClientTypes.ServiceBinding{
							{
								ID: createdBinding.Status.BindingID,
							},
						},
					}, nil)

					Eventually(func() bool {
						if err := k8sClient.Get(ctx, getResourceNamespacedName(createdBinding), createdBinding); err != nil {
							return false
						}
						createdBinding.Status.BindingID = ""
						return k8sClient.Status().Update(ctx, createdBinding) == nil
					}, timeout, interval).Should(BeTrue())
				})

				It("recovers the binding and delete the k8s binding and secret", func() {
					deleteAndValidate(createdBinding)
				})
			})

			When("delete in SM fails with general error", func() {
				errorMessage := "some-error"
				BeforeEach(func() {
					fakeClient.UnbindReturns("", fmt.Errorf(errorMessage))
				})
				AfterEach(func() {
					fakeClient.UnbindReturns("", nil)
					deleteAndValidate(createdBinding)
				})

				It("should not remove finalizer and keep the secret", func() {
					Expect(k8sClient.Delete(ctx, createdBinding)).To(Succeed())
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: createdBinding.Namespace}, createdBinding)
						if err != nil {
							return false
						}
						failedCond := meta.FindStatusCondition(createdBinding.GetConditions(), api.ConditionFailed)
						return failedCond != nil && strings.Contains(failedCond.Message, errorMessage)
					}, timeout, interval).Should(BeTrue())
					Expect(len(createdBinding.Finalizers)).To(Equal(1))
					getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				})
			})

			When("delete in SM fails with transient error", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturnsOnCall(0, "", &sm.ServiceManagerError{StatusCode: http.StatusTooManyRequests})
					fakeClient.UnbindReturnsOnCall(1, "", nil)
				})

				It("should eventually succeed", func() {
					deleteAndValidate(createdBinding)
				})
			})
		})

		Context("Async", func() {
			BeforeEach(func() {
				fakeClient.UnbindReturns(sm.BuildOperationURL("an-operation-id", fakeBindingID, smClientTypes.ServiceBindingsURL), nil)
			})

			When("polling ends with success", func() {
				BeforeEach(func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED}, nil)
				})

				It("should delete the k8s binding and secret", func() {
					Expect(k8sClient.Delete(ctx, createdBinding)).To(Succeed())
					deleteAndValidate(createdBinding)
				})
			})

			When("polling ends with FAILED state", func() {
				errorMessage := "delete-binding-async-error"
				BeforeEach(func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{
						Type:        smClientTypes.DELETE,
						State:       smClientTypes.FAILED,
						Description: errorMessage,
					}, nil)
				})

				It("should not delete the k8s binding and secret", func() {
					Expect(k8sClient.Delete(ctx, createdBinding)).To(Succeed())
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: createdBinding.Namespace}, createdBinding)
						if err != nil {
							return false
						}
						failedCond := meta.FindStatusCondition(createdBinding.GetConditions(), api.ConditionFailed)
						return failedCond != nil && strings.Contains(failedCond.Message, errorMessage)
					}, timeout, interval).Should(BeTrue())
					fakeClient.UnbindReturns("", nil)
					deleteAndValidate(createdBinding)
				})
			})

			When("polling returns error", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturns(sm.BuildOperationURL("an-operation-id", fakeBindingID, smClientTypes.ServiceBindingsURL), nil)
					fakeClient.StatusReturns(nil, fmt.Errorf("no polling for you"))
					//fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				})

				It("should recover and eventually succeed", func() {
					Expect(k8sClient.Delete(ctx, createdBinding)).To(Succeed())
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: createdBinding.Namespace}, createdBinding)
						if err != nil {
							return false
						}
						failedCond := meta.FindStatusCondition(createdBinding.GetConditions(), api.ConditionFailed)
						return failedCond != nil && strings.Contains(failedCond.Message, "no polling for you")
					}, timeout, interval).Should(BeTrue())
					fakeClient.UnbindReturns("", nil)
					deleteAndValidate(createdBinding)
				})
			})
		})
	})

	Context("Recovery", func() {
		type recoveryTestCase struct {
			lastOpType                       smClientTypes.OperationCategory
			lastOpState                      smClientTypes.OperationState
			expectedConditionSucceededStatus metav1.ConditionStatus
		}
		executeTestCase := func(testCase recoveryTestCase) {
			fakeBinding := func(state smClientTypes.OperationState) *smClientTypes.ServiceBinding {
				return &smClientTypes.ServiceBinding{
					ID:          fakeBindingID,
					Name:        "fake-binding-external-name",
					Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
					LastOperation: &smClientTypes.Operation{
						Type:        testCase.lastOpType,
						State:       state,
						Description: "fake-description",
					},
				}
			}

			When("binding exists in SM", func() {
				BeforeEach(func() {
					fakeClient.ListBindingsReturns(
						&smClientTypes.ServiceBindings{
							ServiceBindings: []smClientTypes.ServiceBinding{*fakeBinding(testCase.lastOpState)},
						}, nil)
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.INPROGRESS}, nil)
				})

				AfterEach(func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED}, nil)
					fakeClient.GetBindingByIDReturns(fakeBinding(smClientTypes.SUCCEEDED), nil)
				})

				When(fmt.Sprintf("last operation is %s %s", testCase.lastOpType, testCase.lastOpState), func() {
					It("should resync status", func() {
						var err error
						createdBinding, err = createBindingWithoutAssertionsAndWait(ctx, bindingName, bindingTestNamespace, instanceName, "", "fake-binding-external-name", false)
						Expect(err).ToNot(HaveOccurred())
						smCallArgs := fakeClient.ListBindingsArgsForCall(0)
						Expect(smCallArgs.LabelQuery).To(HaveLen(1))
						Expect(smCallArgs.LabelQuery[0]).To(ContainSubstring("_k8sname"))

						Expect(smCallArgs.FieldQuery).To(HaveLen(3))
						Expect(smCallArgs.FieldQuery[0]).To(ContainSubstring("name"))
						Expect(smCallArgs.FieldQuery[1]).To(ContainSubstring("context/clusterid"))
						Expect(smCallArgs.FieldQuery[2]).To(ContainSubstring("context/namespace"))

						waitForResourceCondition(ctx, createdBinding, api.ConditionSucceeded, testCase.expectedConditionSucceededStatus, getConditionReason(testCase.lastOpType, testCase.lastOpState), "")

						switch testCase.lastOpState {
						case smClientTypes.FAILED:
							Expect(isFailed(createdBinding))
						case smClientTypes.INPROGRESS:
							Expect(isInProgress(createdBinding))
						case smClientTypes.SUCCEEDED:
							Expect(isResourceReady(createdBinding))
						}
					})
				})
			})
		}

		for _, testCase := range []recoveryTestCase{
			{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.SUCCEEDED, expectedConditionSucceededStatus: metav1.ConditionTrue},
			{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.INPROGRESS, expectedConditionSucceededStatus: metav1.ConditionFalse},
			{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.FAILED, expectedConditionSucceededStatus: metav1.ConditionFalse},
			{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.SUCCEEDED, expectedConditionSucceededStatus: metav1.ConditionTrue},
			{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.INPROGRESS, expectedConditionSucceededStatus: metav1.ConditionFalse},
			{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.FAILED, expectedConditionSucceededStatus: metav1.ConditionFalse},
		} {
			executeTestCase(testCase)
		}

		When("binding exists in SM without last operation", func() {
			BeforeEach(func() {
				smBinding := &smClientTypes.ServiceBinding{
					ID:          fakeBindingID,
					Name:        "fake-binding-external-name",
					Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
				}
				fakeClient.ListBindingsReturns(
					&smClientTypes.ServiceBindings{
						ServiceBindings: []smClientTypes.ServiceBinding{*smBinding},
					}, nil)
			})

			It("should resync successfully", func() {
				var err error
				createdBinding, err = createBindingWithoutAssertionsAndWait(ctx, bindingName, bindingTestNamespace, instanceName, "", "fake-binding-external-name", false)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("Credential Rotation", func() {
		BeforeEach(func() {
			fakeClient.RenameBindingReturns(nil, nil)
			createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
			fakeClient.ListBindingsStub = func(params *sm.Parameters) (*smClientTypes.ServiceBindings, error) {
				if params == nil || params.FieldQuery == nil || len(params.FieldQuery) == 0 {
					return nil, nil
				}

				if strings.Contains(params.FieldQuery[0], "binding-external-name-") {
					return &smClientTypes.ServiceBindings{
						ServiceBindings: []smClientTypes.ServiceBinding{
							{
								ID:          fakeBindingID,
								Ready:       true,
								Credentials: json.RawMessage("{\"secret_key2\": \"secret_value2\"}"),
								LastOperation: &smClientTypes.Operation{
									Type:        smClientTypes.CREATE,
									State:       smClientTypes.SUCCEEDED,
									Description: "fake-description",
								},
							},
						},
					}, nil
				}
				return nil, nil
			}
		})

		It("should rotate the credentials and create old binding", func() {
			Expect(k8sClient.Get(ctx, defaultLookupKey, createdBinding)).To(Succeed())
			createdBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
				Enabled:           true,
				RotatedBindingTTL: "1h",
				RotationFrequency: "1ns",
			}

			var secret *corev1.Secret
			Eventually(func() bool {
				secret = getSecret(ctx, createdBinding.Spec.SecretName, bindingTestNamespace, true)
				secret.Data = map[string][]byte{}
				return k8sClient.Update(ctx, secret) == nil
			}, timeout, interval).Should(BeTrue())

			updateBinding(ctx, defaultLookupKey, createdBinding)

			myBinding := &v1.ServiceBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, defaultLookupKey, myBinding)
				return err == nil && myBinding.Status.LastCredentialsRotationTime != nil && len(myBinding.Status.Conditions) == 2
			}, timeout, interval).Should(BeTrue())

			secret = getSecret(ctx, myBinding.Spec.SecretName, bindingTestNamespace, true)
			val := secret.Data["secret_key"]
			Expect(string(val)).To(Equal("secret_value"))

			bindingList := &v1.ServiceBindingList{}
			Eventually(func() bool {
				Expect(k8sClient.List(ctx, bindingList, client.MatchingLabels{api.StaleBindingIDLabel: myBinding.Status.BindingID}, client.InNamespace(bindingTestNamespace))).To(Succeed())
				return len(bindingList.Items) > 0
			}, timeout, interval).Should(BeTrue())

			oldBinding := bindingList.Items[0]
			Expect(oldBinding.Spec.CredRotationPolicy.Enabled).To(BeFalse())

			secret = getSecret(ctx, oldBinding.Spec.SecretName, bindingTestNamespace, true)
			val = secret.Data["secret_key2"]
			Expect(string(val)).To(Equal("secret_value2"))
		})

		It("should rotate the credentials with force rotate annotation", func() {
			Expect(k8sClient.Get(ctx, defaultLookupKey, createdBinding)).To(Succeed())
			createdBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
				Enabled:           true,
				RotationFrequency: "1h",
				RotatedBindingTTL: "1h",
			}
			createdBinding.Annotations = map[string]string{
				api.ForceRotateAnnotation: "true",
			}

			updateBinding(ctx, defaultLookupKey, createdBinding)
			myBinding := &v1.ServiceBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, defaultLookupKey, myBinding)
				return err == nil && myBinding.Status.LastCredentialsRotationTime != nil
			}, timeout, interval).Should(BeTrue())

			_, ok := myBinding.Annotations[api.ForceRotateAnnotation]
			Expect(ok).To(BeFalse())
		})

		When("original binding ready=true", func() {
			It("should delete old binding when stale", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := generateBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel:         createdBinding.Status.BindingID,
					api.StaleBindingRotationOfLabel: createdBinding.Name,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           false,
					RotatedBindingTTL: "0ns",
					RotationFrequency: "0ns",
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				waitForResourceToBeDeleted(ctx, getResourceNamespacedName(staleBinding), staleBinding)
			})
		})

		When("original binding ready=false (rotation failed)", func() {
			var failedBinding *v1.ServiceBinding
			BeforeEach(func() {
				failedBinding = newBindingObject("failedbinding", bindingTestNamespace)
				failedBinding.Spec.ServiceInstanceName = "notexistinstance"
				Expect(k8sClient.Create(ctx, failedBinding)).To(Succeed())
				waitForResourceCondition(ctx, failedBinding, api.ConditionSucceeded, metav1.ConditionFalse, Blocked, "")
			})

			It("should not delete old binding when stale", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := generateBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel:         "1234",
					api.StaleBindingRotationOfLabel: failedBinding.Name,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           false,
					RotatedBindingTTL: "0ns",
					RotationFrequency: "0ns",
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				waitForResourceCondition(ctx, staleBinding, api.ConditionPendingTermination, metav1.ConditionTrue, api.ConditionPendingTermination, "")
			})
		})

		When("stale binding is missing rotationOf label", func() {
			It("should delete the binding", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := generateBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel: createdBinding.Status.BindingID,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           false,
					RotatedBindingTTL: "0ns",
					RotationFrequency: "0ns",
				}

				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				waitForResourceToBeDeleted(ctx, getResourceNamespacedName(staleBinding), staleBinding)
			})
		})
	})

	Context("Cross Namespace", func() {
		var crossBinding *v1.ServiceBinding
		var paramsSecret *corev1.Secret
		BeforeEach(func() {
			paramsSecret = &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "param-secret"}, paramsSecret)
			if apierrors.IsNotFound(err) {
				createParamsSecret(testNamespace)
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})

		AfterEach(func() {
			deleteAndWait(ctx, types.NamespacedName{Namespace: testNamespace, Name: "param-secret"}, &corev1.Secret{})
		})

		When("binding is created in a different namespace than the instance", func() {
			AfterEach(func() {
				if crossBinding != nil {
					Expect(k8sClient.Delete(ctx, crossBinding))
				}
			})
			It("should succeed", func() {
				crossBinding = createBinding(ctx, bindingName, testNamespace, instanceName, bindingTestNamespace, "cross-binding-external-name")

				By("Verify binding secret created")
				getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
			})
		})

		Context("cred rotation", func() {
			BeforeEach(func() {
				fakeClient.RenameBindingReturns(nil, nil)
				crossBinding = createBinding(ctx, bindingName, testNamespace, instanceName, bindingTestNamespace, "cross-binding-external-name")
				fakeClient.ListBindingsStub = func(params *sm.Parameters) (*smClientTypes.ServiceBindings, error) {
					if params == nil || params.FieldQuery == nil || len(params.FieldQuery) == 0 {
						return nil, nil
					}

					if strings.Contains(params.FieldQuery[0], "cross-binding-external-name-") {
						return &smClientTypes.ServiceBindings{
							ServiceBindings: []smClientTypes.ServiceBinding{
								{
									ID:          fakeBindingID,
									Ready:       true,
									Credentials: json.RawMessage("{\"secret_key2\": \"secret_value2\"}"),
									LastOperation: &smClientTypes.Operation{
										Type:        smClientTypes.CREATE,
										State:       smClientTypes.SUCCEEDED,
										Description: "fake-description",
									},
								},
							},
						}, nil
					}
					return nil, nil
				}
			})
			AfterEach(func() {
				if crossBinding != nil {
					Expect(k8sClient.Delete(ctx, crossBinding))
				}
			})

			It("should rotate the credentials and create old binding", func() {
				key := types.NamespacedName{Name: bindingName, Namespace: testNamespace}
				Expect(k8sClient.Get(ctx, key, crossBinding)).To(Succeed())
				crossBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           true,
					RotatedBindingTTL: "1h",
					RotationFrequency: "1ns",
				}

				var secret *corev1.Secret
				Eventually(func() bool {
					secret = getSecret(ctx, crossBinding.Spec.SecretName, testNamespace, true)
					secret.Data = map[string][]byte{}
					return k8sClient.Update(ctx, secret) == nil
				}, timeout, interval).Should(BeTrue())

				updateBinding(ctx, key, crossBinding)

				myBinding := &v1.ServiceBinding{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, key, myBinding)
					return err == nil && myBinding.Status.LastCredentialsRotationTime != nil && len(myBinding.Status.Conditions) == 2
				}, timeout, interval).Should(BeTrue())

				secret = getSecret(ctx, myBinding.Spec.SecretName, testNamespace, true)
				val := secret.Data["secret_key"]
				Expect(string(val)).To(Equal("secret_value"))

				bindingList := &v1.ServiceBindingList{}
				Eventually(func() bool {
					Expect(k8sClient.List(ctx, bindingList, client.MatchingLabels{api.StaleBindingIDLabel: myBinding.Status.BindingID}, client.InNamespace(testNamespace))).To(Succeed())
					return len(bindingList.Items) > 0
				}, timeout, interval).Should(BeTrue())
				oldBinding := bindingList.Items[0]
				Expect(oldBinding.Spec.CredRotationPolicy.Enabled).To(BeFalse())

				secret = getSecret(ctx, oldBinding.Spec.SecretName, testNamespace, true)
				val = secret.Data["secret_key2"]
				Expect(string(val)).To(Equal("secret_value2"))
			})
		})
	})
})

func generateBasicBindingTemplate(name, namespace, instanceName, instanceNamespace, externalName string) *v1.ServiceBinding {
	binding := newBindingObject(name, namespace)
	binding.Spec.ServiceInstanceName = instanceName
	if len(instanceNamespace) > 0 {
		binding.Spec.ServiceInstanceNamespace = instanceNamespace
	}
	binding.Spec.ExternalName = externalName
	binding.Spec.Parameters = &runtime.RawExtension{
		Raw: []byte(`{"key": "value"}`),
	}
	binding.Spec.ParametersFrom = []v1.ParametersFromSource{
		{
			SecretKeyRef: &v1.SecretKeyReference{
				Name: "param-secret",
				Key:  "secret-parameter",
			},
		},
	}
	return binding
}

func generateBasicStaleBinding(createdBinding *v1.ServiceBinding) *v1.ServiceBinding {
	staleBinding := &v1.ServiceBinding{}
	staleBinding.Spec = createdBinding.Spec
	staleBinding.Spec.SecretName = createdBinding.Spec.SecretName + "-stale"
	staleBinding.Name = createdBinding.Name + "-stale"
	staleBinding.Namespace = createdBinding.Namespace
	return staleBinding
}

func validateSecretData(secret *corev1.Secret, expectedKey string, expectedValue string) {
	Expect(secret.Data).ToNot(BeNil())
	Expect(secret.Data).To(HaveKey(expectedKey))
	Expect(string(secret.Data[expectedKey])).To(Equal(expectedValue))
}

func getSecret(ctx context.Context, name, namespace string, failOnError bool) *corev1.Secret {
	secret := &corev1.Secret{}

	if failOnError {
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
			return err == nil
		}, timeout, interval).Should(BeTrue())
	} else {
		_ = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	}

	return secret
}

func updateBinding(ctx context.Context, key types.NamespacedName, serviceBinding *v1.ServiceBinding) {
	if err := k8sClient.Update(ctx, serviceBinding); err == nil {
		return
	}
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, sb); err != nil {
			return false
		}
		sb.Annotations = serviceBinding.Annotations
		sb.Labels = serviceBinding.Labels
		sb.Spec = serviceBinding.Spec
		return k8sClient.Update(ctx, serviceBinding) == nil
	}, timeout, interval).Should(BeTrue())
}

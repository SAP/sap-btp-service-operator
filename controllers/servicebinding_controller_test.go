package controllers

import (
	"context"
	"encoding/json"
	"errors"
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

const (
	fakeBindingID        = "fake-binding-id"
	bindingTestNamespace = "test-namespace"
)

var _ = Describe("ServiceBinding controller", func() {
	var createdInstance *v1.ServiceInstance
	var createdBinding *v1.ServiceBinding
	var instanceName string
	var bindingName string
	var guid string
	var defaultLookupKey types.NamespacedName

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
				return isReady(createdBinding) || isFailed(createdBinding)
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
		_, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, "", externalName)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			waitForBindingConditionMessageAndStatus(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, api.ConditionFailed, failureMessage, metav1.ConditionTrue)
		}
	}

	createBindingWithBlockedError := func(ctx context.Context, name, namespace, instanceName, externalName, failureMessage string) *v1.ServiceBinding {
		_, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, "", externalName)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			waitForBindingConditionMessageAndStatus(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, api.ConditionSucceeded, failureMessage, metav1.ConditionFalse)
		}
		return nil
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
		instanceLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
		createdInstance := waitForInstanceToBeReady(ctx, instanceLookupKey)
		Expect(createdInstance.Status.InstanceID).ToNot(BeEmpty())
		return createdInstance
	}

	JustBeforeEach(func() {
		createdInstance = createInstance(context.Background(), instanceName, bindingTestNamespace, instanceName+"-external")
	})

	BeforeEach(func() {
		guid = uuid.New().String()
		instanceName = "test-instance-" + guid
		bindingName = "test-binding-" + guid
		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: "12345678", Tags: []byte("[\"test\"]")}, nil)
		fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage(`{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`)}, "", nil)

		smInstance := &smClientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.UPDATE}}
		fakeClient.GetInstanceByIDReturns(smInstance, nil)
		secret := &corev1.Secret{}
		defaultLookupKey = types.NamespacedName{Namespace: bindingTestNamespace, Name: bindingName}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: bindingTestNamespace, Name: "param-secret"}, secret)
		if apierrors.IsNotFound(err) {
			createParamsSecret(bindingTestNamespace)
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		if createdBinding != nil {
			fakeClient.UnbindReturns("", nil)
			_ = k8sClient.Delete(context.Background(), createdBinding)
			waitForBindingAndSecretToBeDeleted(context.Background(), defaultLookupKey)
			createdBinding = nil
		}

		Expect(k8sClient.Delete(context.Background(), createdInstance)).To(Succeed())
		validateInstanceGotDeleted(context.Background(), types.NamespacedName{Name: instanceName, Namespace: bindingTestNamespace})
		k8sClient.Get(context.Background(), types.NamespacedName{Name: instanceName, Namespace: bindingTestNamespace}, createdInstance)
		createdInstance = nil
	})

	Context("Create", func() {
		Context("Invalid parameters", func() {
			When("service instance name is not provided", func() {
				It("should fail", func() {
					createBindingWithError(context.Background(), bindingName, bindingTestNamespace, "", "",
						"spec.serviceInstanceName in body should be at least 1 chars long")
				})
			})

			When("referenced service instance does not exist", func() {
				It("should fail", func() {
					createBindingWithBlockedError(context.Background(), bindingName, bindingTestNamespace, "no-such-instance", "",
						"couldn't find the service instance")
				})
			})

			When("referenced service instance exist in another namespace", func() {
				var otherNamespace = "other-" + bindingTestNamespace
				BeforeEach(func() {
					nsSpec := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: otherNamespace}}
					err := k8sClient.Create(context.Background(), nsSpec)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should fail", func() {
					createBindingWithBlockedError(context.Background(), bindingName, otherNamespace, instanceName, "",
						"couldn't find the service instance")
				})
			})

			When("secret name is already taken", func() {
				ctx := context.Background()
				var secret *corev1.Secret
				var secretName string
				BeforeEach(func() {
					secretName = "mysecret-" + guid
					secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: bindingTestNamespace}}
					Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
					By("Verify secret created")
					waitForSecretToBeCreated(ctx, types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace})
				})
				AfterEach(func() {
					Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
				})
				It("should fail the request and allow the user to replace secret name", func() {
					binding := newBindingObject(bindingName, bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					binding.Spec.SecretName = secretName

					Expect(k8sClient.Create(ctx, binding)).To(Succeed())
					bindingLookupKey := types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}
					Eventually(func() bool {
						if err := k8sClient.Get(ctx, bindingLookupKey, binding); err != nil {
							return false
						}
						cond := meta.FindStatusCondition(binding.GetConditions(), api.ConditionSucceeded)
						return cond != nil && cond.Reason == Blocked && strings.Contains(cond.Message, "is already taken. Choose another name and try again")
					}, timeout, interval).Should(BeTrue())

					binding.Spec.SecretName = secretName + "-new"
					Expect(k8sClient.Update(ctx, binding)).Should(Succeed())
					waitForBindingToBeReady(ctx, bindingLookupKey)

					By("Verify binding secret created")
					bindingSecret := getSecret(ctx, binding.Spec.SecretName, binding.Namespace, true)
					Expect(bindingSecret).ToNot(BeNil())
				})
			})

			When("secret belong to a different binding", func() {
				ctx := context.Background()
				var tmpBinding *v1.ServiceBinding
				var secretName string
				JustBeforeEach(func() {
					tmpBindingName := bindingName + "-tmp"
					secretName = "mysecret-" + guid
					tmpBinding = newBindingObject(tmpBindingName, bindingTestNamespace)
					tmpBinding.Spec.ServiceInstanceName = instanceName
					tmpBinding.Spec.SecretName = secretName

					_ = k8sClient.Create(ctx, tmpBinding)
					waitForBindingToBeReady(ctx, types.NamespacedName{Name: tmpBindingName, Namespace: bindingTestNamespace})
				})
				JustAfterEach(func() {
					Expect(k8sClient.Delete(ctx, tmpBinding)).Should(Succeed())
				})
				It("should fail the request with relevant message and allow the user to replace secret name", func() {
					binding := newBindingObject(bindingName, bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					binding.Spec.SecretName = secretName

					Expect(k8sClient.Create(ctx, binding)).To(Succeed())
					bindingLookupKey := types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}
					waitForBindingConditionReasonAndMessage(ctx, bindingLookupKey, api.ConditionSucceeded, Blocked, "belongs to another binding")
					k8sClient.Get(ctx, bindingLookupKey, binding)

					binding.Spec.SecretName = secretName + "-new"
					Expect(k8sClient.Update(ctx, binding)).Should(Succeed())
					waitForBindingToBeReady(ctx, bindingLookupKey)

					By("Verify binding secret created")
					bindingSecret := getSecret(ctx, binding.Spec.SecretName, binding.Namespace, true)
					Expect(bindingSecret).ToNot(BeNil())
				})
			})
		})

		Context("Valid parameters", func() {
			Context("Sync", func() {

				validateInstanceInfo := func(bindingSecret *corev1.Secret) {
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
					Expect(metadata["credentialProperties"]).To(ContainElements(credentialProperties))
					Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "instance_name", Format: string(TEXT)}))
					Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "instance_guid", Format: string(TEXT)}))
					Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "plan", Format: string(TEXT)}))
					Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "label", Format: string(TEXT)}))
					Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "type", Format: string(TEXT)}))
					Expect(metadata["metaDataProperties"]).To(ContainElement(SecretMetadataProperty{Name: "tags", Format: string(JSON)}))
				}

				It("Should create binding and store the binding credentials in a secret", func() {
					ctx := context.Background()
					createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
					Expect(createdBinding.Spec.ExternalName).To(Equal("binding-external-name"))
					Expect(createdBinding.Spec.UserInfo).NotTo(BeNil())

					By("Verify binding secret created")
					bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
					validateSecretData(bindingSecret, "secret_key", "secret_value")
					validateSecretData(bindingSecret, "escaped", `{"escaped_key":"escaped_val"}`)
					validateSecretData(bindingSecret, "instance_external_name", createdInstance.Spec.ExternalName)
					validateSecretData(bindingSecret, "instance_name", createdInstance.Name)
					validateInstanceInfo(bindingSecret)
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
					ctx := context.Background()
					binding := newBindingObject("binding-with-secretkey", bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					binding.Spec.SecretName = "mysecret"
					secretKey := "mycredentials"
					binding.Spec.SecretKey = &secretKey

					_ = k8sClient.Create(ctx, binding)
					bindingLookupKey := types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}
					waitForBindingToBeReady(ctx, bindingLookupKey)

					bindingSecret := getSecret(ctx, binding.Spec.SecretName, bindingTestNamespace, true)
					validateSecretData(bindingSecret, secretKey, `{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`)
					validateInstanceInfo(bindingSecret)
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
					ctx := context.Background()
					binding := newBindingObject("binding-with-secretrootkey", bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					secretKey := "mycredentials"
					binding.Spec.SecretKey = &secretKey
					secretRootKey := "root"
					binding.Spec.SecretRootKey = &secretRootKey

					_ = k8sClient.Create(ctx, binding)
					bindingLookupKey := types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}
					waitForBindingToBeReady(ctx, bindingLookupKey)

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
					fakeSmResponse := func(bindingID string) {
						fakeClient.ListBindingsReturns(&smClientTypes.ServiceBindings{
							ServiceBindings: []smClientTypes.ServiceBinding{
								{
									ID:          bindingID,
									Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
									LastOperation: &smClientTypes.Operation{
										Type:        smClientTypes.CREATE,
										State:       smClientTypes.SUCCEEDED,
										Description: "fake-description",
									},
								},
							},
						}, nil)
					}

					It("should recreate the secret", func() {
						ctx := context.Background()
						createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
						secretLookupKey := types.NamespacedName{Name: createdBinding.Spec.SecretName, Namespace: createdBinding.Namespace}
						bindingSecret := getSecret(ctx, secretLookupKey.Name, secretLookupKey.Namespace, true)
						fakeSmResponse(createdBinding.Status.BindingID)
						err := k8sClient.Delete(ctx, bindingSecret)
						Expect(err).ToNot(HaveOccurred())

						Eventually(func() bool {
							sec := getSecret(ctx, secretLookupKey.Name, secretLookupKey.Namespace, false)
							return len(sec.Name) > 0
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
							createBindingWithError(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name",
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
							b, err := createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", true)
							Expect(err).ToNot(HaveOccurred())
							Expect(isReady(b)).To(BeTrue())
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
							createBindingWithError(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", errorMessage)
						})
					})

					When("SM returned error 502 and broker returned 429", func() {
						BeforeEach(func() {
							errorMessage = "too many requests from broker"
							fakeClient.BindReturns(nil, "", getTransientBrokerError(errorMessage))
						})

						It("should detect the error as transient and eventually succeed", func() {
							createdBinding, _ := createBindingWithoutAssertionsAndWait(context.Background(),
								bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", false)
							expectBindingToBeInFailedStateWithMsg(createdBinding, errorMessage)

							fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID,
								Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
							waitForBindingToBeReady(context.Background(), defaultLookupKey)
						})
					})

					When("SM returned 502 and broker returned 400", func() {
						BeforeEach(func() {
							errorMessage = "very bad request"
							fakeClient.BindReturnsOnCall(0, nil, "", getNonTransientBrokerError(errorMessage))
						})

						It("should detect the error as non-transient and fail", func() {
							createBindingWithError(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", errorMessage)
						})
					})

				})

				When("SM returned invalid credentials json", func() {
					BeforeEach(func() {
						fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("\"invalidjson\": \"secret_value\"")}, "", nil)

					})

					It("creation will fail with appropriate message", func() {
						ctx := context.Background()
						createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
						waitForBindingConditionReasonAndMessage(ctx, defaultLookupKey, api.ConditionFailed, "CreateFailed", "failed to create secret")
					})
				})
			})

			Context("Async", func() {
				JustBeforeEach(func() {
					fakeClient.BindReturns(
						nil,
						fmt.Sprintf("/v1/service_bindings/%s/operations/an-operation-id", fakeBindingID),
						nil)
				})

				When("bind polling returns success", func() {
					JustBeforeEach(func() {
						fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED}, nil)
						fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, nil)
					})

					It("Should create binding and store the binding credentials in a secret", func() {
						ctx := context.Background()
						createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "")
					})
				})

				When("bind polling returns FAILED state", func() {
					errorMessage := "no binding for you"

					JustBeforeEach(func() {
						fakeClient.StatusReturns(&smClientTypes.Operation{
							Type:        smClientTypes.CREATE,
							State:       smClientTypes.FAILED,
							Description: errorMessage,
						}, nil)
					})

					It("should fail with the error returned from SM", func() {
						createBindingWithError(context.Background(), bindingName, bindingTestNamespace, instanceName, "existing-name",
							errorMessage)
					})
				})

				When("bind polling returns error", func() {
					JustBeforeEach(func() {
						fakeClient.BindReturns(nil, "/v1/service_bindings/id/operations/1234", nil)
						fakeClient.StatusReturnsOnCall(0, nil, fmt.Errorf("no polling for you"))
						fakeClient.StatusReturnsOnCall(1, &smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}, nil)
						fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
					})
					It("should eventually succeed", func() {
						_, err := createBindingWithoutAssertions(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "")
						Expect(err).ToNot(HaveOccurred())
						waitForBindingToBeReady(context.Background(), defaultLookupKey)
					})
				})
			})

			When("external name is not provided", func() {
				It("succeeds and uses the k8s name as external name", func() {
					createdBinding = createBinding(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "")
					Expect(createdBinding.Spec.ExternalName).To(Equal(createdBinding.Name))
				})
			})

			When("secret name is provided", func() {
				It("should create a secret with the provided name", func() {
					binding := newBindingObject(bindingName, bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					binding.Spec.SecretName = "my-special-secret"
					Expect(k8sClient.Create(context.Background(), binding)).Should(Succeed())
					waitForSecretToBeCreated(context.Background(), types.NamespacedName{Name: "my-special-secret", Namespace: bindingTestNamespace})
				})
			})

			When("referenced service instance is failed", func() {
				JustBeforeEach(func() {
					setFailureConditions(smClientTypes.CREATE, "Failed to create instance (test)", createdInstance)
					err := k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should retry and succeed once the instance is ready", func() {
					// verify create fail with appropriate message
					createBindingWithBlockedError(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name",
						"is not usable")

					// verify creation is retired and succeeds after instance is ready
					setSuccessConditions(smClientTypes.CREATE, createdInstance)
					err := k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())
					waitForBindingToBeReady(context.Background(), defaultLookupKey)
				})
			})

			When("referenced service instance is not ready", func() {
				JustBeforeEach(func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.INPROGRESS}, nil)
					setInProgressConditions(smClientTypes.CREATE, "", createdInstance)
					createdInstance.Status.OperationURL = "/1234"
					createdInstance.Status.OperationType = smClientTypes.CREATE
					err := k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should retry and succeed once the instance is ready", func() {
					var err error

					createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", false)
					Expect(err).ToNot(HaveOccurred())
					Expect(isInProgress(createdBinding)).To(BeTrue())

					// verify creation is retired and succeeds after instance is ready
					setSuccessConditions(smClientTypes.CREATE, createdInstance)
					createdInstance.Status.OperationType = ""
					createdInstance.Status.OperationURL = ""
					err = k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())

					waitForBindingToBeReady(context.Background(), defaultLookupKey)
				})
			})
		})
	})

	Context("Update", func() {
		JustBeforeEach(func() {
			createdBinding = createBinding(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
			Expect(isReady(createdBinding)).To(BeTrue())
		})
		When("external name is changed", func() {
			It("should fail", func() {
				createdBinding.Spec.ExternalName = "new-external-name"
				err := k8sClient.Update(context.Background(), createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

		When("service instance name is changed", func() {
			It("should fail", func() {
				createdBinding.Spec.ServiceInstanceName = "new-instance-name"
				err := k8sClient.Update(context.Background(), createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

		When("parameters are changed", func() {
			It("should fail", func() {
				createdBinding.Spec.Parameters = &runtime.RawExtension{
					Raw: []byte(`{"new-key": "new-value"}`),
				}
				err := k8sClient.Update(context.Background(), createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

		When("secretKey is changed", func() {
			It("should fail", func() {
				secretKey := "not-nil"
				createdBinding.Spec.SecretKey = &secretKey
				err := k8sClient.Update(context.Background(), createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("updating service bindings is not supported"))
			})
		})

	})

	Context("Delete", func() {
		validateBindingDeletion := func(binding *v1.ServiceBinding) {
			secretName := binding.Spec.SecretName
			Expect(secretName).ToNot(BeEmpty())
			Expect(k8sClient.Delete(context.Background(), binding)).To(Succeed())
			waitForBindingAndSecretToBeDeleted(context.Background(), defaultLookupKey)
		}

		validateBindingNotDeleted := func(binding *v1.ServiceBinding, errorMessage string) {
			secretName := createdBinding.Spec.SecretName
			Expect(secretName).ToNot(BeEmpty())
			Expect(k8sClient.Delete(context.Background(), createdBinding)).To(Succeed())
			key := defaultLookupKey
			err := k8sClient.Get(context.Background(), key, createdBinding)
			Expect(err).ToNot(HaveOccurred())

			waitForBindingConditionAndReason(context.Background(), key, api.ConditionSucceeded, getConditionReason(smClientTypes.DELETE, smClientTypes.FAILED))
			waitForBindingConditionAndReason(context.Background(), key, api.ConditionReady, "Provisioned")
			waitForBindingConditionReasonAndMessage(context.Background(), key, api.ConditionFailed, getConditionReason(smClientTypes.DELETE, smClientTypes.FAILED), errorMessage)

			err = k8sClient.Get(context.Background(), key, &corev1.Secret{})
			Expect(err).ToNot(HaveOccurred())
		}

		JustBeforeEach(func() {
			createdBinding = createBinding(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "binding-external-name")
			Expect(isReady(createdBinding)).To(BeTrue())
		})

		Context("Sync", func() {
			When("delete in SM succeeds", func() {
				JustBeforeEach(func() {
					fakeClient.UnbindReturns("", nil)
				})
				It("should delete the k8s binding and secret", func() {
					validateBindingDeletion(createdBinding)
				})
			})

			When("delete without binding id", func() {
				JustBeforeEach(func() {
					fakeClient.UnbindReturns("", nil)
					fakeClient.ListBindingsReturns(&smClientTypes.ServiceBindings{
						ServiceBindings: []smClientTypes.ServiceBinding{
							{
								ID: createdBinding.Status.BindingID,
							},
						},
					}, nil)

					createdBinding.Status.BindingID = ""
					Expect(k8sClient.Status().Update(context.Background(), createdBinding)).To(Succeed())
				})

				It("should delete the k8s binding and secret", func() {
					validateBindingDeletion(createdBinding)
				})
			})

			When("delete in SM fails with general error", func() {
				errorMessage := "some-error"
				BeforeEach(func() {
					fakeClient.UnbindReturns("", fmt.Errorf(errorMessage))
				})

				It("should not remove finalizer and keep the secret", func() {
					validateBindingNotDeleted(createdBinding, errorMessage)
				})
			})

			When("delete in SM fails with transient error", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturnsOnCall(0, "", &sm.ServiceManagerError{StatusCode: http.StatusTooManyRequests})
					fakeClient.UnbindReturnsOnCall(1, "", nil)
				})

				It("should eventually succeed", func() {
					validateBindingDeletion(createdBinding)
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
					validateBindingDeletion(createdBinding)
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
					validateBindingNotDeleted(createdBinding, errorMessage)
				})
			})

			When("polling returns error", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturnsOnCall(0, sm.BuildOperationURL("an-operation-id", fakeBindingID, smClientTypes.ServiceBindingsURL), nil)
					fakeClient.StatusReturns(nil, fmt.Errorf("no polling for you"))
					fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
					fakeClient.UnbindReturnsOnCall(1, "", nil)
				})

				It("should recover and eventually succeed", func() {
					validateBindingDeletion(createdBinding)
				})
			})
		})
	})

	Context("Recovery", func() {
		type recoveryTestCase struct {
			lastOpType  smClientTypes.OperationCategory
			lastOpState smClientTypes.OperationState
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
						createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "fake-binding-external-name", false)
						Expect(err).ToNot(HaveOccurred())
						smCallArgs := fakeClient.ListBindingsArgsForCall(0)
						Expect(smCallArgs.LabelQuery).To(HaveLen(1))
						Expect(smCallArgs.LabelQuery[0]).To(ContainSubstring("_k8sname"))

						Expect(smCallArgs.FieldQuery).To(HaveLen(3))
						Expect(smCallArgs.FieldQuery[0]).To(ContainSubstring("name"))
						Expect(smCallArgs.FieldQuery[1]).To(ContainSubstring("context/clusterid"))
						Expect(smCallArgs.FieldQuery[2]).To(ContainSubstring("context/namespace"))

						waitForBindingConditionAndReason(context.Background(), defaultLookupKey, api.ConditionSucceeded, getConditionReason(testCase.lastOpType, testCase.lastOpState))

						switch testCase.lastOpState {
						case smClientTypes.FAILED:
							Expect(isFailed(createdBinding))
						case smClientTypes.INPROGRESS:
							Expect(isInProgress(createdBinding))
						case smClientTypes.SUCCEEDED:
							Expect(isReady(createdBinding))
						}
					})
				})
			})
		}

		for _, testCase := range []recoveryTestCase{
			{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.SUCCEEDED},
			{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.INPROGRESS},
			{lastOpType: smClientTypes.CREATE, lastOpState: smClientTypes.FAILED},
			{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.SUCCEEDED},
			{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.INPROGRESS},
			{lastOpType: smClientTypes.DELETE, lastOpState: smClientTypes.FAILED},
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
				createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "", "fake-binding-external-name", false)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("Credential Rotation", func() {
		var ctx context.Context

		JustBeforeEach(func() {
			fakeClient.RenameBindingReturns(nil, nil)
			ctx = context.Background()
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
			Expect(k8sClient.Get(context.Background(), defaultLookupKey, createdBinding)).To(Succeed())
			createdBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
				Enabled:           true,
				RotatedBindingTTL: "1h",
				RotationFrequency: "1ns",
			}
			secret := getSecret(ctx, createdBinding.Spec.SecretName, bindingTestNamespace, true)
			secret.Data = map[string][]byte{}
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())
			Expect(k8sClient.Update(ctx, createdBinding)).To(Succeed())

			// binding rotated
			myBinding := &v1.ServiceBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, defaultLookupKey, myBinding)
				return err == nil && myBinding.Status.LastCredentialsRotationTime != nil && len(myBinding.Status.Conditions) == 2
			}, timeout, interval).Should(BeTrue())

			// secret updated back
			secret = getSecret(ctx, myBinding.Spec.SecretName, bindingTestNamespace, true)
			val := secret.Data["secret_key"]
			Expect(string(val)).To(Equal("secret_value"))

			// old binding created
			bindingList := &v1.ServiceBindingList{}
			Eventually(func() bool {
				Expect(k8sClient.List(ctx, bindingList, client.MatchingLabels{api.StaleBindingIDLabel: myBinding.Status.BindingID}, client.InNamespace(bindingTestNamespace))).To(Succeed())
				return len(bindingList.Items) > 0
			}, timeout, interval).Should(BeTrue())
			oldBinding := bindingList.Items[0]
			Expect(oldBinding.Spec.CredRotationPolicy.Enabled).To(BeFalse())

			// old secret created
			secret = getSecret(ctx, oldBinding.Spec.SecretName, bindingTestNamespace, true)
			val = secret.Data["secret_key2"]
			Expect(string(val)).To(Equal("secret_value2"))
		})

		It("should rotate the credentials with force rotate annotation", func() {
			Expect(k8sClient.Get(context.Background(), defaultLookupKey, createdBinding)).To(Succeed())
			createdBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
				Enabled:           true,
				RotationFrequency: "1h",
				RotatedBindingTTL: "1h",
			}
			createdBinding.Annotations = map[string]string{
				api.ForceRotateAnnotation: "true",
			}
			Expect(k8sClient.Update(ctx, createdBinding)).To(Succeed())
			// binding rotated
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
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := getBasicStaleBinding(createdBinding)
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
				waitForBindingToBeDeleted(ctx, types.NamespacedName{Name: staleBinding.Name, Namespace: bindingTestNamespace})
			})
		})

		When("original binding ready=false (rotation failed)", func() {
			var failedBinding *v1.ServiceBinding
			BeforeEach(func() {
				failedBinding = newBindingObject("failedbinding", bindingTestNamespace)
				failedBinding.Spec.ServiceInstanceName = "notexistinstance"
				Expect(k8sClient.Create(ctx, failedBinding)).To(Succeed())
				waitForBindingConditionAndReason(ctx, types.NamespacedName{Name: failedBinding.Name, Namespace: bindingTestNamespace}, api.ConditionSucceeded, Blocked)
			})
			It("should not delete old binding when stale", func() {
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := getBasicStaleBinding(createdBinding)
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
				waitForBindingConditionAndReason(ctx, types.NamespacedName{Name: staleBinding.Name, Namespace: bindingTestNamespace}, api.ConditionPendingTermination, api.ConditionPendingTermination)
			})
		})

		When("stale binding is missing rotationOf label", func() {
			It("should delete the binding", func() {
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := getBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel: createdBinding.Status.BindingID,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           false,
					RotatedBindingTTL: "0ns",
					RotationFrequency: "0ns",
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				waitForBindingToBeDeleted(ctx, types.NamespacedName{Name: staleBinding.Name, Namespace: bindingTestNamespace})
			})
		})
	})

	Context("Cross Namespace", func() {
		var ctx context.Context
		var crossBinding *v1.ServiceBinding
		var paramsSecret *corev1.Secret
		BeforeEach(func() {
			ctx = context.Background()
			paramsSecret = &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "param-secret"}, paramsSecret)
			if apierrors.IsNotFound(err) {
				createParamsSecret(testNamespace)
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		})
		AfterEach(func() {
			_ = k8sClient.Delete(ctx, paramsSecret)
		})

		When("binding is created in a different namespace than the instance", func() {
			AfterEach(func() {
				if crossBinding != nil {
					Expect(k8sClient.Delete(context.Background(), crossBinding))
				}
			})
			It("should succeed", func() {
				crossBinding = createBinding(ctx, bindingName, testNamespace, instanceName, bindingTestNamespace, "cross-binding-external-name")

				By("Verify binding secret created")
				_ = getSecret(context.Background(), createdBinding.Spec.SecretName, createdBinding.Namespace, true)
			})
		})

		Context("cred rotation", func() {
			JustBeforeEach(func() {
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
			JustAfterEach(func() {
				if crossBinding != nil {
					Expect(k8sClient.Delete(context.Background(), crossBinding))
				}
			})
			It("should rotate the credentials and create old binding", func() {
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: testNamespace}, crossBinding)).To(Succeed())
				crossBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           true,
					RotatedBindingTTL: "1h",
					RotationFrequency: "1ns",
				}
				secret := getSecret(ctx, crossBinding.Spec.SecretName, testNamespace, true)
				secret.Data = map[string][]byte{}
				Expect(k8sClient.Update(ctx, secret)).To(Succeed())
				Expect(k8sClient.Update(ctx, crossBinding)).To(Succeed())

				// binding rotated
				myBinding := &v1.ServiceBinding{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: bindingName, Namespace: testNamespace}, myBinding)
					return err == nil && myBinding.Status.LastCredentialsRotationTime != nil && len(myBinding.Status.Conditions) == 2
				}, timeout, interval).Should(BeTrue())

				// secret updated back
				secret = getSecret(ctx, myBinding.Spec.SecretName, testNamespace, true)
				val := secret.Data["secret_key"]
				Expect(string(val)).To(Equal("secret_value"))

				// old binding created
				bindingList := &v1.ServiceBindingList{}
				Eventually(func() bool {
					Expect(k8sClient.List(ctx, bindingList, client.MatchingLabels{api.StaleBindingIDLabel: myBinding.Status.BindingID}, client.InNamespace(testNamespace))).To(Succeed())
					return len(bindingList.Items) > 0
				}, timeout, interval).Should(BeTrue())
				oldBinding := bindingList.Items[0]
				Expect(oldBinding.Spec.CredRotationPolicy.Enabled).To(BeFalse())

				// old secret created
				secret = getSecret(ctx, oldBinding.Spec.SecretName, testNamespace, true)
				val = secret.Data["secret_key2"]
				Expect(string(val)).To(Equal("secret_value2"))
			})
		})
	})
})

func getBasicStaleBinding(createdBinding *v1.ServiceBinding) *v1.ServiceBinding {
	staleBinding := &v1.ServiceBinding{}
	staleBinding.Spec = createdBinding.Spec
	staleBinding.Spec.SecretName = createdBinding.Spec.SecretName + "-stale"
	staleBinding.Name = createdBinding.Name + "-stale"
	staleBinding.Namespace = createdBinding.Namespace
	return staleBinding
}

func expectBindingToBeInFailedStateWithMsg(binding *v1.ServiceBinding, message string) {
	cond := meta.FindStatusCondition(binding.GetConditions(), api.ConditionSucceeded)
	Expect(cond).To(Not(BeNil()))
	Expect(cond.Message).To(ContainSubstring(message))
	Expect(cond.Status).To(Equal(metav1.ConditionFalse))
}

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

func waitForBindingToBeReady(ctx context.Context, key types.NamespacedName) {
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, sb)
		if err != nil {
			return false
		}
		return isReady(sb)
	}, timeout, interval).Should(BeTrue())
}

func waitForBindingAndSecretToBeDeleted(ctx context.Context, key types.NamespacedName) {
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, sb)
		if apierrors.IsNotFound(err) {
			err := k8sClient.Get(ctx, key, &corev1.Secret{})
			return apierrors.IsNotFound(err)
		}
		return false
	}, timeout, interval).Should(BeTrue())
}

func waitForBindingToBeDeleted(ctx context.Context, key types.NamespacedName) {
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, sb)
		if apierrors.IsNotFound(err) {
			return apierrors.IsNotFound(err)
		}
		return false
	}, timeout, interval).Should(BeTrue())
}

func waitForBindingConditionAndReason(ctx context.Context, key types.NamespacedName, conditionType, reason string) {
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, sb); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(sb.GetConditions(), conditionType)
		return cond != nil && cond.Reason == reason
	}, timeout, interval).Should(BeTrue())
}

func waitForBindingConditionMessageAndStatus(ctx context.Context, key types.NamespacedName, conditionType, msg string, status metav1.ConditionStatus) {
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, sb); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(sb.GetConditions(), conditionType)
		return cond != nil && cond.Status == status && strings.Contains(cond.Message, msg)
	}, timeout, interval).Should(BeTrue())
}

func waitForBindingConditionReasonAndMessage(ctx context.Context, key types.NamespacedName, conditionType, reason, msg string) {
	sb := &v1.ServiceBinding{}
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, sb); err != nil {
			return false
		}
		cond := meta.FindStatusCondition(sb.GetConditions(), conditionType)
		return cond != nil && cond.Reason == reason && strings.Contains(cond.Message, msg)
	}, 2*timeout, interval).Should(BeTrue())
}

func waitForSecretToBeCreated(ctx context.Context, key types.NamespacedName) {
	Eventually(func() bool {
		return k8sClient.Get(ctx, key, &corev1.Secret{}) == nil
	}, timeout, interval).Should(BeTrue())
}

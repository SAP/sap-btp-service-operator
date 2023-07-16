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

	// Define utility constants for object names and testing timeouts/durations and intervals.

	var createdInstance *v1.ServiceInstance
	var createdBinding *v1.ServiceBinding
	var instanceName string
	var bindingName string
	var guid string

	createBindingWithoutAssertionsAndWait := func(ctx context.Context, name string, namespace string, instanceName string, externalName string, wait bool) (*v1.ServiceBinding, error) {
		binding := newBindingObject(name, namespace)
		binding.Spec.ServiceInstanceName = instanceName
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

		if err := k8sClient.Create(ctx, binding); err != nil {
			return nil, err
		}

		bindingLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
		createdBinding = &v1.ServiceBinding{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, bindingLookupKey, createdBinding)
			if err != nil {
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

	createBindingWithoutAssertions := func(ctx context.Context, name string, namespace string, instanceName string, externalName string) (*v1.ServiceBinding, error) {
		return createBindingWithoutAssertionsAndWait(ctx, name, namespace, instanceName, externalName, true)
	}

	createBindingWithError := func(ctx context.Context, name, namespace, instanceName, externalName, failureMessage string) {
		createdBinding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, externalName)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			Expect(len(createdBinding.Status.Conditions)).To(Equal(3))
			Expect(createdBinding.Status.Conditions[2].Status).To(Equal(metav1.ConditionTrue))
			Expect(createdBinding.Status.Conditions[2].Message).To(ContainSubstring(failureMessage))
		}
	}

	createBindingWithBlockedError := func(ctx context.Context, name, namespace, instanceName, externalName, failureMessage string) *v1.ServiceBinding {
		createdBinding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, externalName)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			Expect(len(createdBinding.Status.Conditions)).To(Equal(2))
			Expect(createdBinding.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(createdBinding.Status.Conditions[0].Message).To(ContainSubstring(failureMessage))
			Expect(createdBinding.Status.Conditions[0].Reason).To(Equal(Blocked))
		}

		return nil
	}

	createBinding := func(ctx context.Context, name, namespace, instanceName, externalName string) *v1.ServiceBinding {
		createdBinding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, externalName)
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
		createdInstance := &v1.ServiceInstance{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, instanceLookupKey, createdInstance)
			if err != nil {
				return false
			}
			return isReady(createdInstance)
		}, timeout, interval).Should(BeTrue())
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
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: bindingTestNamespace, Name: "param-secret"}, secret)
		if apierrors.IsNotFound(err) {
			createParamsSecret(bindingTestNamespace)
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		if createdBinding != nil {
			secretName := createdBinding.Spec.SecretName
			fakeClient.UnbindReturns("", nil)
			_ = k8sClient.Delete(context.Background(), createdBinding)
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
			if len(secretName) > 0 {
				Eventually(func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace}, &corev1.Secret{})
					return apierrors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}

			createdBinding = nil
		}

		Expect(k8sClient.Delete(context.Background(), createdInstance)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: instanceName, Namespace: bindingTestNamespace}, createdInstance)
			if err != nil {
				if apierrors.IsNotFound(err) {
					return true
				}
			}

			return false
		}, timeout, interval).Should(BeTrue())

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
				JustBeforeEach(func() {
					secretName = "mysecret-" + guid
					secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: bindingTestNamespace}}
					Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
					By("Verify secret created")
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace}, &corev1.Secret{})
						return err == nil
					}, timeout, interval).Should(BeTrue())
				})
				JustAfterEach(func() {
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
					Eventually(func() bool {
						err := k8sClient.Get(ctx, bindingLookupKey, binding)
						if err != nil {
							return false
						}
						return isReady(binding)
					}, timeout, interval).Should(BeTrue())

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
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: tmpBindingName, Namespace: bindingTestNamespace}, &v1.ServiceBinding{})
						return err == nil
					}, timeout, interval).Should(BeTrue())
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
					Eventually(func() bool {
						if err := k8sClient.Get(ctx, bindingLookupKey, binding); err != nil {
							return false
						}
						cond := meta.FindStatusCondition(binding.GetConditions(), api.ConditionSucceeded)
						return cond != nil && cond.Reason == Blocked && strings.Contains(cond.Message, "belongs to another binding")
					}, timeout*2, interval).Should(BeTrue())

					binding.Spec.SecretName = secretName + "-new"
					Expect(k8sClient.Update(ctx, binding)).Should(Succeed())
					Eventually(func() bool {
						err := k8sClient.Get(ctx, bindingLookupKey, binding)
						if err != nil {
							return false
						}
						return isReady(binding)
					}, timeout, interval).Should(BeTrue())

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
					createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name")
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
					Eventually(func() bool {
						err := k8sClient.Get(ctx, bindingLookupKey, binding)
						if err != nil {
							return false
						}
						return isReady(binding)
					}, timeout, interval).Should(BeTrue())

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
					Eventually(func() bool {
						err := k8sClient.Get(ctx, bindingLookupKey, binding)
						if err != nil {
							return false
						}
						return isReady(binding)
					}, timeout, interval).Should(BeTrue())

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
						createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name")
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
								StatusCode: http.StatusTooManyRequests,
								Message:    errorMessage,
							})
							fakeClient.BindReturnsOnCall(1, &smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
						})

						It("should eventually succeed", func() {
							b, err := createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", true)
							Expect(err).ToNot(HaveOccurred())
							Expect(isReady(b)).To(BeTrue())
						})
					})

					When("SM returned non transient error(400)", func() {
						BeforeEach(func() {
							errorMessage = "very bad request"
							fakeClient.BindReturnsOnCall(0, nil, "", &sm.ServiceManagerError{
								StatusCode: http.StatusBadRequest,
								Message:    errorMessage,
							})
						})

						It("should fail", func() {
							createBindingWithError(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", errorMessage)
						})
					})

					When("SM returned error 502 and broker returned 429", func() {
						BeforeEach(func() {
							errorMessage = "too many requests"
							fakeClient.BindReturnsOnCall(0, nil, "", getTransientBrokerError())
							fakeClient.BindReturnsOnCall(1, &smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
						})

						It("should detect the error as transient and eventually succeed", func() {
							b, err := createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", true)
							Expect(err).ToNot(HaveOccurred())
							Expect(isReady(b)).To(BeTrue())
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
						var err error
						createdBinding, err = createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "")
						Expect(err).To(BeNil())
						Eventually(func() bool {
							err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
							if err != nil {
								return false
							}
							return isFailed(createdBinding) && strings.Contains(createdBinding.Status.Conditions[0].Message, "failed to create secret")
						}, timeout, interval).Should(BeTrue())
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
						createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "")
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
						createdBinding, err := createBindingWithoutAssertions(context.Background(), bindingName, bindingTestNamespace, instanceName, "")
						Expect(err).ToNot(HaveOccurred())
						Eventually(func() bool {
							err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
							Expect(err).ToNot(HaveOccurred())
							return isReady(createdBinding)
						}, timeout/2, interval).Should(BeTrue())
					})
				})
			})

			When("external name is not provided", func() {
				It("succeeds and uses the k8s name as external name", func() {
					createdBinding = createBinding(context.Background(), bindingName, bindingTestNamespace, instanceName, "")
					Expect(createdBinding.Spec.ExternalName).To(Equal(createdBinding.Name))
				})
			})

			When("secret name is provided", func() {
				It("should create a secret with the provided name", func() {
					binding := newBindingObject(bindingName, bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					binding.Spec.SecretName = "my-special-secret"

					Expect(k8sClient.Create(context.Background(), binding)).Should(Succeed())

					Eventually(func() bool {
						secret := &corev1.Secret{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{Name: "my-special-secret", Namespace: bindingTestNamespace}, secret)
						return err == nil
					}, timeout, interval).Should(BeTrue())
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

					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
						return err == nil && isReady(createdBinding)
					}, timeout, interval).Should(BeTrue())
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

					createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", false)
					Expect(err).ToNot(HaveOccurred())
					Expect(isInProgress(createdBinding)).To(BeTrue())

					// verify creation is retired and succeeds after instance is ready
					setSuccessConditions(smClientTypes.CREATE, createdInstance)
					createdInstance.Status.OperationType = ""
					createdInstance.Status.OperationURL = ""
					err = k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
						return err == nil && isReady(createdBinding)
					}, timeout, interval).Should(BeTrue())
				})
			})
		})
	})

	Context("Update", func() {
		JustBeforeEach(func() {
			createdBinding = createBinding(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name")
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
			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, binding)
				if apierrors.IsNotFound(err) {
					err := k8sClient.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace}, &corev1.Secret{})
					return apierrors.IsNotFound(err)
				}
				return false
			}, timeout, interval).Should(BeTrue())
		}

		validateBindingNotDeleted := func(binding *v1.ServiceBinding, errorMessage string) {
			secretName := createdBinding.Spec.SecretName
			Expect(secretName).ToNot(BeEmpty())
			Expect(k8sClient.Delete(context.Background(), createdBinding)).To(Succeed())

			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
				if err != nil {
					return false
				}
				return len(createdBinding.Status.Conditions) == 3
			}, timeout, interval).Should(BeTrue())

			Expect(createdBinding.Status.Conditions[0].Reason).To(Equal(getConditionReason(smClientTypes.DELETE, smClientTypes.FAILED)))
			Expect(createdBinding.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(createdBinding.Status.Conditions[1].Reason).To(Equal("Provisioned"))
			Expect(createdBinding.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
			Expect(createdBinding.Status.Conditions[2].Reason).To(Equal(getConditionReason(smClientTypes.DELETE, smClientTypes.FAILED)))
			Expect(createdBinding.Status.Conditions[2].Status).To(Equal(metav1.ConditionTrue))
			Expect(createdBinding.Status.Conditions[2].Message).To(ContainSubstring(errorMessage))

			err = k8sClient.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace}, &corev1.Secret{})
			Expect(err).ToNot(HaveOccurred())
		}

		JustBeforeEach(func() {
			createdBinding = createBinding(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name")
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
				JustBeforeEach(func() {
					fakeClient.UnbindReturns("", fmt.Errorf(errorMessage))
				})

				It("should not remove finalizer and keep the secret", func() {
					validateBindingNotDeleted(createdBinding, errorMessage)
				})
			})

			When("delete in SM fails with transient error", func() {
				JustBeforeEach(func() {
					fakeClient.UnbindReturnsOnCall(0, "", &sm.ServiceManagerError{StatusCode: http.StatusTooManyRequests})
					fakeClient.UnbindReturnsOnCall(1, "", nil)
				})

				It("should eventually succeed", func() {
					validateBindingDeletion(createdBinding)
				})
			})
		})

		Context("Async", func() {
			JustBeforeEach(func() {
				fakeClient.UnbindReturns(sm.BuildOperationURL("an-operation-id", fakeBindingID, smClientTypes.ServiceBindingsURL), nil)
			})

			When("polling ends with success", func() {
				JustBeforeEach(func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED}, nil)
				})

				It("should delete the k8s binding and secret", func() {
					validateBindingDeletion(createdBinding)
				})
			})

			When("polling ends with FAILED state", func() {
				errorMessage := "delete-binding-async-error"
				JustBeforeEach(func() {
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

				JustBeforeEach(func() {
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
				JustBeforeEach(func() {
					fakeClient.ListBindingsReturns(
						&smClientTypes.ServiceBindings{
							ServiceBindings: []smClientTypes.ServiceBinding{*fakeBinding(testCase.lastOpState)},
						}, nil)
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.INPROGRESS}, nil)
				})
				JustAfterEach(func() {
					fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeBindingID, State: smClientTypes.SUCCEEDED}, nil)
					fakeClient.GetBindingByIDReturns(fakeBinding(smClientTypes.SUCCEEDED), nil)
				})
				When(fmt.Sprintf("last operation is %s %s", testCase.lastOpType, testCase.lastOpState), func() {
					It("should resync status", func() {
						var err error
						createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "fake-binding-external-name", false)
						Expect(err).ToNot(HaveOccurred())
						smCallArgs := fakeClient.ListBindingsArgsForCall(0)
						Expect(smCallArgs.LabelQuery).To(HaveLen(1))
						Expect(smCallArgs.LabelQuery[0]).To(ContainSubstring("_k8sname"))

						Expect(smCallArgs.FieldQuery).To(HaveLen(3))
						Expect(smCallArgs.FieldQuery[0]).To(ContainSubstring("name"))
						Expect(smCallArgs.FieldQuery[1]).To(ContainSubstring("context/clusterid"))
						Expect(smCallArgs.FieldQuery[2]).To(ContainSubstring("context/namespace"))

						Expect(createdBinding.Status.Conditions[0].Reason).To(Equal(getConditionReason(testCase.lastOpType, testCase.lastOpState)))

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
			JustBeforeEach(func() {
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
				createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "fake-binding-external-name", false)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("Credential Rotation", func() {
		var ctx context.Context

		JustBeforeEach(func() {
			fakeClient.RenameBindingReturns(nil, nil)
			ctx = context.Background()
			createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "binding-external-name")

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
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
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
				err := k8sClient.Get(ctx, types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, myBinding)
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
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
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
				err := k8sClient.Get(ctx, types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, myBinding)
				return err == nil && myBinding.Status.LastCredentialsRotationTime != nil
			}, timeout, interval).Should(BeTrue())

			_, ok := myBinding.Annotations[api.ForceRotateAnnotation]
			Expect(ok).To(BeFalse())
		})

		When("original binding ready=true", func() {
			It("should delete old binding when stale", func() {
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := &v1.ServiceBinding{}
				staleBinding.Spec = createdBinding.Spec
				staleBinding.Spec.SecretName = createdBinding.Spec.SecretName + "-stale"
				staleBinding.Name = createdBinding.Name + "-stale"
				staleBinding.Namespace = createdBinding.Namespace
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel:         createdBinding.Status.BindingID,
					api.StaleBindingRotationOfLabel: createdBinding.Name,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled: false,
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: staleBinding.Name, Namespace: bindingTestNamespace}, staleBinding)
					return apierrors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			})
		})

		When("original binding ready=false (rotation failed)", func() {
			var failedBinding *v1.ServiceBinding
			JustBeforeEach(func() {
				failedBinding = newBindingObject("failedbinding", bindingTestNamespace)
				failedBinding.Spec.ServiceInstanceName = "notexistinstance"
				Expect(k8sClient.Create(ctx, failedBinding)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: failedBinding.Name, Namespace: bindingTestNamespace}, failedBinding)
					if err != nil {
						return false
					}
					return meta.IsStatusConditionFalse(failedBinding.Status.Conditions, api.ConditionSucceeded)
				}, timeout, interval).Should(BeTrue())
			})
			It("should not delete old binding when stale", func() {
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := &v1.ServiceBinding{}
				staleBinding.Spec = createdBinding.Spec
				staleBinding.Spec.SecretName = createdBinding.Spec.SecretName + "-stale"
				staleBinding.Name = createdBinding.Name + "-stale"
				staleBinding.Namespace = createdBinding.Namespace
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel:         "1234",
					api.StaleBindingRotationOfLabel: failedBinding.Name,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled: false,
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: staleBinding.Name, Namespace: bindingTestNamespace}, staleBinding)
					if err != nil {
						return false
					}
					return meta.IsStatusConditionTrue(staleBinding.Status.Conditions, api.ConditionPendingTermination)

				}, timeout, interval).Should(BeTrue())
			})
		})

		When("stale binding is missing rotationOf label", func() {
			It("should delete the binding", func() {
				Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := &v1.ServiceBinding{}
				staleBinding.Spec = createdBinding.Spec
				staleBinding.Spec.SecretName = createdBinding.Spec.SecretName + "-stale"
				staleBinding.Name = createdBinding.Name + "-stale"
				staleBinding.Namespace = createdBinding.Namespace
				staleBinding.Labels = map[string]string{
					api.StaleBindingIDLabel: createdBinding.Status.BindingID,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled: false,
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: staleBinding.Name, Namespace: bindingTestNamespace}, staleBinding)
					return apierrors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			})
		})
	})
})

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

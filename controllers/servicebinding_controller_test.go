package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	smTypes "github.com/Peripli/service-manager/pkg/types"
	"github.com/Peripli/service-manager/pkg/web"
	"github.com/SAP/sap-btp-service-operator/api/services.cloud.sap.com/v1alpha1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/SAP/sap-btp-service-operator/client/sm/smfakes"
	smclientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

const (
	fakeBindingID        = "fake-binding-id"
	bindingTestNamespace = "test-namespace"
)

var _ = Describe("ServiceBinding controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.

	var createdInstance *v1alpha1.ServiceInstance
	var createdBinding *v1alpha1.ServiceBinding
	var instanceName string
	var bindingName string

	newBinding := func(name, namespace string) *v1alpha1.ServiceBinding {
		return &v1alpha1.ServiceBinding{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "services.cloud.sap.com/v1alpha1",
				Kind:       "ServiceBinding",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
	}

	createBindingWithoutAssertionsAndWait := func(ctx context.Context, name string, namespace string, instanceName string, externalName string, wait bool) (
		*v1alpha1.ServiceBinding, error,
	) {
		binding := newBinding(name, namespace)
		binding.Spec.ServiceInstanceName = instanceName
		binding.Spec.ExternalName = externalName
		binding.Spec.Parameters = &runtime.RawExtension{
			Raw: []byte(`{"key": "value"}`),
		}
		binding.Spec.ParametersFrom = []v1alpha1.ParametersFromSource{
			{
				SecretKeyRef: &v1alpha1.SecretKeyReference{
					Name: "param-secret",
					Key:  "secret-parameter",
				},
			},
		}

		if err := k8sClient.Create(ctx, binding); err != nil {
			return nil, err
		}

		bindingLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
		createdBinding = &v1alpha1.ServiceBinding{}

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

	createBindingWithoutAssertions := func(ctx context.Context, name string, namespace string, instanceName string, externalName string) (
		*v1alpha1.ServiceBinding, error,
	) {
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

	createBindingWithBlockedError := func(ctx context.Context, name, namespace, instanceName, externalName, failureMessage string) *v1alpha1.ServiceBinding {
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

	createBinding := func(ctx context.Context, name, namespace, instanceName, externalName string) *v1alpha1.ServiceBinding {
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

	createInstance := func(ctx context.Context, name, namespace, externalName string) *v1alpha1.ServiceInstance {
		instance := &v1alpha1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "services.cloud.sap.com/v1alpha1",
				Kind:       "ServiceInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha1.ServiceInstanceSpec{
				ExternalName:        externalName,
				ServicePlanName:     "a-plan-name",
				ServiceOfferingName: "an-offering-name",
				CustomTags:          []string{"custom-tag"},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		instanceLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
		createdInstance := &v1alpha1.ServiceInstance{}

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
		createdInstance = createInstance(context.Background(), instanceName, bindingTestNamespace, "")
	})

	BeforeEach(func() {
		guid := uuid.New().String()
		instanceName = "test-instance-" + guid
		bindingName = "test-binding-" + guid
		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: "12345678", Tags: []byte("[\"test\"]")}, nil)
		fakeClient.BindReturns(&smclientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage(`{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`)}, "", nil)

		smInstance := &smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smTypes.Operation{State: smTypes.SUCCEEDED, Type: smTypes.UPDATE}}
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
				JustBeforeEach(func() {
					secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "mysecret", Namespace: bindingTestNamespace}}
					Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
					By("Verify secret created")
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: "mysecret", Namespace: bindingTestNamespace}, &corev1.Secret{})
						return err == nil
					}, timeout, interval).Should(BeTrue())
				})
				JustAfterEach(func() {
					Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
				})
				It("should fail the request and allow the user to replace secret name", func() {
					binding := newBinding("newbinding", bindingTestNamespace)
					binding.Spec.ServiceInstanceName = instanceName
					binding.Spec.SecretName = "mysecret"

					_ = k8sClient.Create(ctx, binding)
					bindingLookupKey := types.NamespacedName{Name: binding.Name, Namespace: binding.Namespace}
					Eventually(func() bool {
						if err := k8sClient.Get(ctx, bindingLookupKey, binding); err != nil {
							return false
						}
						return isInProgress(binding) && binding.GetConditions()[0].Reason == Blocked &&
							strings.Contains(binding.GetConditions()[0].Message, "already taken")
					}, timeout, interval).Should(BeTrue())

					binding.Spec.SecretName = "mynewsecret"
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
					validateSecretData(bindingSecret, "tags", "[\"test\",\"custom-tag\"]")
					validateSecretData(bindingSecret, "instance_name", instanceName)
					Expect(bindingSecret.Data).To(HaveKey("instance_guid"))
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
					validateInstanceInfo(bindingSecret)
				})

				It("should put the raw broker response into the secret if spec.secretKey is provided", func() {
					ctx := context.Background()
					binding := newBinding("binding-with-secretkey", bindingTestNamespace)
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
				})

				It("should put binding data in single key if spec.secretRootKey is provided", func() {
					ctx := context.Background()
					binding := newBinding("binding-with-secretrootkey", bindingTestNamespace)
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
						fakeClient.ListBindingsReturns(&smclientTypes.ServiceBindings{
							ServiceBindings: []smclientTypes.ServiceBinding{
								{
									ID:          bindingID,
									Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
									LastOperation: &smTypes.Operation{
										Type:        smTypes.CREATE,
										State:       smTypes.SUCCEEDED,
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
							fakeClient.BindReturnsOnCall(1, &smclientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
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

				})

				When("SM returned invalid credentials json", func() {
					BeforeEach(func() {
						fakeClient.BindReturns(&smclientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("\"invalidjson\": \"secret_value\"")}, "", nil)

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
						fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeBindingID, State: string(smTypes.SUCCEEDED)}, nil)
						fakeClient.GetBindingByIDReturns(&smclientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, nil)
					})

					It("Should create binding and store the binding credentials in a secret", func() {
						ctx := context.Background()
						createdBinding = createBinding(ctx, bindingName, bindingTestNamespace, instanceName, "")
					})
				})

				When("bind polling returns FAILED state", func() {
					errorMessage := "no binding for you"

					JustBeforeEach(func() {
						fakeClient.StatusReturns(&smclientTypes.Operation{
							Type:        string(smTypes.CREATE),
							State:       string(smTypes.FAILED),
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
						fakeClient.StatusReturnsOnCall(1, &smclientTypes.Operation{ResourceID: fakeBindingID, State: string(smTypes.SUCCEEDED), Type: string(smTypes.CREATE)}, nil)
						fakeClient.GetBindingByIDReturns(&smclientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smTypes.Operation{State: smTypes.SUCCEEDED, Type: smTypes.CREATE}}, nil)
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
					binding := newBinding(bindingName, bindingTestNamespace)
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
					setFailureConditions(smTypes.CREATE, "Failed to create instance (test)", createdInstance)
					err := k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should retry and succeed once the instance is ready", func() {
					// verify create fail with appropriate message
					createBindingWithBlockedError(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name",
						"is not usable")

					// verify creation is retired and succeeds after instance is ready
					setSuccessConditions(smTypes.CREATE, createdInstance)
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
					fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeInstanceID, State: string(smTypes.IN_PROGRESS)}, nil)
					setInProgressConditions(smTypes.CREATE, "", createdInstance)
					createdInstance.Status.OperationURL = "/1234"
					createdInstance.Status.OperationType = smTypes.CREATE
					err := k8sClient.Status().Update(context.Background(), createdInstance)
					Expect(err).ToNot(HaveOccurred())
				})

				It("should retry and succeed once the instance is ready", func() {
					var err error

					createdBinding, err = createBindingWithoutAssertionsAndWait(context.Background(), bindingName, bindingTestNamespace, instanceName, "binding-external-name", false)
					Expect(err).ToNot(HaveOccurred())
					Expect(isInProgress(createdBinding)).To(BeTrue())

					// verify creation is retired and succeeds after instance is ready
					setSuccessConditions(smTypes.CREATE, createdInstance)
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

		validateBindingDeletion := func(binding *v1alpha1.ServiceBinding) {
			secretName := binding.Spec.SecretName
			Expect(secretName).ToNot(BeEmpty())
			Expect(k8sClient.Delete(context.Background(), binding)).To(Succeed())
			Eventually(
				func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, binding)
					if apierrors.IsNotFound(err) {
						err := k8sClient.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: bindingTestNamespace}, &corev1.Secret{})
						return apierrors.IsNotFound(err)
					}
					return false
				}, timeout, interval).Should(BeTrue())
		}

		validateBindingNotDeleted := func(binding *v1alpha1.ServiceBinding, errorMessage string) {
			secretName := createdBinding.Spec.SecretName
			Expect(secretName).ToNot(BeEmpty())
			Expect(k8sClient.Delete(context.Background(), createdBinding)).To(Succeed())

			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
			Expect(err).ToNot(HaveOccurred())

			Eventually(
				func() bool {
					err := k8sClient.Get(context.Background(), types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}, createdBinding)
					if err != nil {
						return false
					}
					return len(createdBinding.Status.Conditions) == 3
				}, timeout, interval).Should(BeTrue())

			Expect(createdBinding.Status.Conditions[0].Reason).To(Equal(getConditionReason(smTypes.DELETE, smTypes.FAILED)))
			Expect(createdBinding.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
			Expect(createdBinding.Status.Conditions[1].Reason).To(Equal("Provisioned"))
			Expect(createdBinding.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
			Expect(createdBinding.Status.Conditions[2].Reason).To(Equal(getConditionReason(smTypes.DELETE, smTypes.FAILED)))
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

					fakeClient.ListBindingsReturns(&smclientTypes.ServiceBindings{
						ServiceBindings: []smclientTypes.ServiceBinding{
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
				fakeClient.UnbindReturns(sm.BuildOperationURL("an-operation-id", fakeBindingID, web.ServiceBindingsURL), nil)
			})

			When("polling ends with success", func() {
				JustBeforeEach(func() {
					fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeBindingID, State: string(smTypes.SUCCEEDED)}, nil)
				})

				It("should delete the k8s binding and secret", func() {
					validateBindingDeletion(createdBinding)
				})
			})

			When("polling ends with FAILED state", func() {
				errorMessage := "delete-binding-async-error"
				JustBeforeEach(func() {
					fakeClient.StatusReturns(&smclientTypes.Operation{
						Type:        string(smTypes.DELETE),
						State:       string(smTypes.FAILED),
						Description: errorMessage,
					}, nil)
				})

				It("should not delete the k8s binding and secret", func() {
					validateBindingNotDeleted(createdBinding, errorMessage)
				})
			})

			When("polling returns error", func() {

				JustBeforeEach(func() {
					fakeClient.UnbindReturnsOnCall(0, sm.BuildOperationURL("an-operation-id", fakeBindingID, web.ServiceBindingsURL), nil)
					fakeClient.StatusReturns(nil, fmt.Errorf("no polling for you"))
					fakeClient.GetBindingByIDReturns(&smclientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smTypes.Operation{State: smTypes.SUCCEEDED, Type: smTypes.CREATE}}, nil)
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
			lastOpType  smTypes.OperationCategory
			lastOpState smTypes.OperationState
		}
		executeTestCase := func(testCase recoveryTestCase) {
			fakeBinding := func(state smTypes.OperationState) *smclientTypes.ServiceBinding {
				return &smclientTypes.ServiceBinding{
					ID:          fakeBindingID,
					Name:        "fake-binding-external-name",
					Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
					LastOperation: &smTypes.Operation{
						Type:        testCase.lastOpType,
						State:       state,
						Description: "fake-description",
					},
				}
			}

			When("binding exists in SM", func() {
				JustBeforeEach(func() {
					fakeClient.ListBindingsReturns(
						&smclientTypes.ServiceBindings{
							ServiceBindings: []smclientTypes.ServiceBinding{*fakeBinding(testCase.lastOpState)},
						}, nil)
					fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeBindingID, State: string(smTypes.IN_PROGRESS)}, nil)
				})
				JustAfterEach(func() {
					fakeClient.StatusReturns(&smclientTypes.Operation{ResourceID: fakeBindingID, State: string(smTypes.SUCCEEDED)}, nil)
					fakeClient.GetBindingByIDReturns(fakeBinding(smTypes.SUCCEEDED), nil)
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
						case smTypes.FAILED:
							Expect(isFailed(createdBinding))
						case smTypes.IN_PROGRESS:
							Expect(isInProgress(createdBinding))
						case smTypes.SUCCEEDED:
							Expect(isReady(createdBinding))
						}
					})
				})
			})
		}

		for _, testCase := range []recoveryTestCase{
			{lastOpType: smTypes.CREATE, lastOpState: smTypes.SUCCEEDED},
			{lastOpType: smTypes.CREATE, lastOpState: smTypes.IN_PROGRESS},
			{lastOpType: smTypes.CREATE, lastOpState: smTypes.FAILED},
			{lastOpType: smTypes.DELETE, lastOpState: smTypes.SUCCEEDED},
			{lastOpType: smTypes.DELETE, lastOpState: smTypes.IN_PROGRESS},
			{lastOpType: smTypes.DELETE, lastOpState: smTypes.FAILED},
		} {
			executeTestCase(testCase)
		}
	})
})

func validateSecretData(secret *corev1.Secret, expectedKey string, expectedValue string) {
	Expect(secret.Data).ToNot(BeNil())
	Expect(secret.Data).To(HaveKey(expectedKey))
	Expect(string(secret.Data[expectedKey])).To(Equal(expectedValue))
}

func getSecret(ctx context.Context, name, namespace string, failOnError bool) *corev1.Secret {
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if failOnError {
		Expect(err).ToNot(HaveOccurred())
	}

	return secret
}

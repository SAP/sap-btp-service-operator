package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/lithammer/dedent"
	authv1 "k8s.io/api/authentication/v1"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/internal/utils"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/SAP/sap-btp-service-operator/client/sm/smfakes"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
		createdInstance *v1.ServiceInstance
		createdBinding  *v1.ServiceBinding

		defaultLookupKey types.NamespacedName

		testUUID             string
		bindingName          string
		instanceName         string
		instanceExternalName string
		paramsSecret         *corev1.Secret
	)

	createBindingWithoutAssertions := func(ctx context.Context, name, namespace, instanceName, instanceNamespace, externalName string, secretTemplate string, wait bool) (*v1.ServiceBinding, error) {
		binding := generateBasicBindingTemplate(name, namespace, instanceName, instanceNamespace, externalName, secretTemplate)
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
				return isResourceReady(createdBinding) || utils.IsFailed(createdBinding)
			} else {
				return len(createdBinding.Status.Conditions) > 0 && createdBinding.Status.Conditions[0].Message != "Pending"
			}
		}, timeout, interval).Should(BeTrue())
		return createdBinding, nil
	}

	createAndValidateBinding := func(ctx context.Context, name, namespace, instanceName, instanceNamespace, externalName string, secretTemplate string) *v1.ServiceBinding {
		createdBinding, err := createBindingWithoutAssertions(ctx, name, namespace, instanceName, instanceNamespace, externalName, secretTemplate, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(createdBinding.Status.InstanceID).ToNot(BeEmpty())
		Expect(createdBinding.Status.BindingID).To(Equal(fakeBindingID))
		Expect(createdBinding.Spec.SecretName).To(Not(BeEmpty()))
		Expect(common.GetObservedGeneration(createdBinding)).To(Equal(int64(1)))
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

	validateSecretMetadata := func(bindingSecret *corev1.Secret, credentialProperties []utils.SecretMetadataProperty) {
		metadata := make(map[string][]utils.SecretMetadataProperty)
		Expect(json.Unmarshal(bindingSecret.Data[".metadata"], &metadata)).To(Succeed())
		if credentialProperties != nil {
			Expect(metadata["credentialProperties"]).To(ContainElements(credentialProperties))
		}
		Expect(metadata["metaDataProperties"]).To(ContainElement(utils.SecretMetadataProperty{Name: "instance_name", Format: string(utils.TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(utils.SecretMetadataProperty{Name: "instance_guid", Format: string(utils.TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(utils.SecretMetadataProperty{Name: "plan", Format: string(utils.TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(utils.SecretMetadataProperty{Name: "label", Format: string(utils.TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(utils.SecretMetadataProperty{Name: "type", Format: string(utils.TEXT)}))
		Expect(metadata["metaDataProperties"]).To(ContainElement(utils.SecretMetadataProperty{Name: "tags", Format: string(utils.JSON)}))
	}

	BeforeEach(func() {
		ctx = context.Background()
		log := ctrl.Log.WithName("bindingTest")
		ctx = context.WithValue(ctx, utils.LogKey{}, log)
		testUUID = uuid.New().String()
		instanceName = "test-instance-" + testUUID
		bindingName = "test-binding-" + testUUID
		instanceExternalName = instanceName + "-external"

		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: "12345678", Tags: []byte("[\"test\"]")}, nil)
		fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage(`{"secret_key": "secret_value", "escaped": "{\"escaped_key\":\"escaped_val\"}"}`)}, "", nil)

		smInstance := &smClientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.UPDATE}}
		fakeClient.GetInstanceByIDReturns(smInstance, nil)

		defaultLookupKey = types.NamespacedName{Namespace: bindingTestNamespace, Name: bindingName}
		createdInstance = createInstance(ctx, instanceName, bindingTestNamespace, instanceExternalName)
		paramsSecret = createParamsSecret(ctx, "binding-params-secret", bindingTestNamespace)

	})

	AfterEach(func() {
		if createdBinding != nil {
			fakeClient.UnbindReturns("", nil)
			deleteAndWait(ctx, createdBinding)
		}

		if createdInstance != nil {
			fakeClient.DeprovisionReturns("", nil)
			deleteAndWait(ctx, createdInstance)
		}

		deleteAndWait(ctx, paramsSecret)

		createdBinding = nil
		createdInstance = nil
	})

	Context("Create", func() {
		Context("invalid parameters", func() {
			When("service instance name is not provided", func() {
				It("should fail", func() {
					_, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, "", "", "", "", false)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("spec.serviceInstanceName in body should be at least 1 chars long"))
				})
			})

			When("referenced service instance does not exist", func() {
				It("should fail", func() {
					binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, "no-such-instance", "", "", "", false)
					Expect(err).ToNot(HaveOccurred())
					waitForResourceCondition(ctx, binding, common.ConditionSucceeded, metav1.ConditionFalse, "", "couldn't find the service instance")
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
					deleteAndWait(ctx, secret)
				})

				When("name is already taken", func() {
					It("should fail the request and allow the user to replace secret name", func() {
						binding := newBindingObject(bindingName, bindingTestNamespace)
						binding.Spec.ServiceInstanceName = instanceName
						binding.Spec.SecretName = secretName
						Expect(k8sClient.Create(ctx, binding)).To(Succeed())
						waitForResourceCondition(ctx, binding, common.ConditionSucceeded, metav1.ConditionFalse, common.Blocked, fmt.Sprintf(secretNameTakenErrorFormat, secretName))
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

						waitForResourceCondition(ctx, binding, common.ConditionSucceeded, metav1.ConditionFalse, common.Blocked, fmt.Sprintf(secretAlreadyOwnedErrorFormat, secretName, owningBindingName))

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
				createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "")
				Expect(createdBinding.Spec.ExternalName).To(Equal("binding-external-name"))
				Expect(createdBinding.Spec.UserInfo).NotTo(BeNil())

				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateSecretData(bindingSecret, "secret_key", "secret_value")
				validateSecretData(bindingSecret, "escaped", `{"escaped_key":"escaped_val"}`)
				validateInstanceInfo(bindingSecret, instanceExternalName)
				credentialProperties := []utils.SecretMetadataProperty{
					{
						Name:   "secret_key",
						Format: string(utils.TEXT),
					},
					{
						Name:   "escaped",
						Format: string(utils.TEXT),
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
				credentialProperties := []utils.SecretMetadataProperty{
					{
						Name:      "mycredentials",
						Format:    string(utils.JSON),
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
					createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "")
					secretLookupKey := types.NamespacedName{Name: createdBinding.Spec.SecretName, Namespace: createdBinding.Namespace}
					bindingSecret := getSecret(ctx, secretLookupKey.Name, secretLookupKey.Namespace, true)
					originalSecretUID := bindingSecret.UID
					Expect(k8sClient.Delete(ctx, bindingSecret)).To(Succeed())

					fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{
						ID:          createdBinding.Status.BindingID,
						Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}"),
						LastOperation: &smClientTypes.Operation{
							Type:        smClientTypes.CREATE,
							State:       smClientTypes.SUCCEEDED,
							Description: "fake-description",
						},
					}, nil)

					//tickle the binding
					createdBinding.Annotations = map[string]string{"tickle": "true"}
					Expect(k8sClient.Update(ctx, createdBinding)).To(Succeed())

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
						binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", false)
						Expect(err).ToNot(HaveOccurred())
						waitForResourceCondition(ctx, binding, common.ConditionFailed, metav1.ConditionTrue, "", errorMessage)
					})
				})

				When("SM returned transient error(429) without retry-after header", func() {
					BeforeEach(func() {
						errorMessage = "too many requests"
						fakeClient.BindReturnsOnCall(0, nil, "", &sm.ServiceManagerError{
							StatusCode:  http.StatusTooManyRequests,
							Description: errorMessage,
						})
						fakeClient.BindReturnsOnCall(1, &smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, "", nil)
					})

					It("should eventually succeed", func() {
						b, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", true)
						Expect(err).ToNot(HaveOccurred())
						Expect(isResourceReady(b)).To(BeTrue())
					})
				})

				When("SM returned non transient error(400)", func() {
					BeforeEach(func() {
						errorMessage = "very bad request"
						fakeClient.BindReturns(nil, "", &sm.ServiceManagerError{
							StatusCode:  http.StatusBadRequest,
							Description: errorMessage,
						})
					})

					It("should fail", func() {
						binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", false)
						Expect(err).ToNot(HaveOccurred())
						waitForResourceCondition(ctx, binding, common.ConditionSucceeded, metav1.ConditionFalse, common.CreateInProgress, errorMessage)
					})
				})

				When("SM returned error 502 and broker returned 429", func() {
					BeforeEach(func() {
						errorMessage = "too many requests from broker"
						fakeClient.BindReturns(nil, "", getTransientBrokerError(errorMessage))
					})

					It("should detect the error as transient and eventually succeed", func() {
						createdBinding, _ := createBindingWithoutAssertions(ctx,
							bindingName,
							bindingTestNamespace,
							instanceName,
							"",
							"binding-external-name", "",
							false,
						)

						cond := meta.FindStatusCondition(createdBinding.GetConditions(), common.ConditionSucceeded)
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
						fakeClient.BindReturns(nil, "", getNonTransientBrokerError(errorMessage))
					})

					It("should detect the error as non-transient and fail", func() {
						binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", false)
						Expect(err).ToNot(HaveOccurred())
						waitForResourceCondition(ctx, binding, common.ConditionSucceeded, metav1.ConditionFalse, common.CreateInProgress, errorMessage)
					})
				})

			})

			When("SM returned invalid credentials json", func() {
				BeforeEach(func() {
					fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("\"invalidjson\": \"secret_value\"")}, "", nil)
				})

				It("creation will fail with appropriate message", func() {
					createdBinding, _ = createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", "", false)
					waitForResourceCondition(ctx, createdBinding, common.ConditionFailed, metav1.ConditionTrue, "CreateFailed", "failed to create secret")
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
					createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "", "")
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

					binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "existing-name", "", false)
					Expect(err).ToNot(HaveOccurred())
					waitForResourceCondition(ctx, binding, common.ConditionFailed, metav1.ConditionTrue, "", errorMessage)
				})
			})

			// TODO redefine test
			XWhen("bind polling returns error", func() {
				BeforeEach(func() {
					fakeClient.StatusReturns(nil, fmt.Errorf("no polling for you"))
					fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				})
				It("should eventually succeed", func() {
					binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", "", false)
					Expect(err).ToNot(HaveOccurred())
					waitForResourceCondition(ctx, binding, common.ConditionFailed, metav1.ConditionTrue, "", "no polling for you")
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
					common.UseInstanceMetadataNameInSecret: "true",
				}
				updateInstance(ctx, createdInstance)
				createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "", "")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateInstanceInfo(bindingSecret, instanceName)
				validateSecretMetadata(bindingSecret, nil)
			})
		})

		When("external name is not provided", func() {
			It("succeeds and uses the k8s name as external name", func() {
				createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "", "")
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
				createdInstance.Status.Ready = metav1.ConditionFalse
				updateInstanceStatus(ctx, createdInstance)

				binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", false)
				Expect(err).ToNot(HaveOccurred())
				waitForResourceCondition(ctx, binding, common.ConditionSucceeded, metav1.ConditionFalse, "", "service instance is not ready")

				createdInstance.Status.Ready = metav1.ConditionTrue
				updateInstanceStatus(ctx, createdInstance)
				waitForResourceToBeReady(ctx, binding)
			})
		})

		When("referenced service instance is not ready", func() {
			It("should retry and succeed once the instance is ready", func() {
				createdInstance.Status.Ready = metav1.ConditionFalse
				fakeClient.StatusReturns(&smClientTypes.Operation{ResourceID: fakeInstanceID, State: smClientTypes.INPROGRESS}, nil)
				utils.SetInProgressConditions(ctx, smClientTypes.CREATE, "", createdInstance, false)
				createdInstance.Status.OperationURL = "/1234"
				createdInstance.Status.OperationType = smClientTypes.CREATE
				updateInstanceStatus(ctx, createdInstance)

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", false)
				Expect(err).ToNot(HaveOccurred())
				waitForResourceCondition(ctx, createdBinding, common.ConditionSucceeded, metav1.ConditionFalse, common.Blocked, "")

				createdInstance.Status.Ready = metav1.ConditionTrue
				utils.SetSuccessConditions(smClientTypes.CREATE, createdInstance, false)
				createdInstance.Status.OperationType = ""
				createdInstance.Status.OperationURL = ""
				updateInstanceStatus(ctx, createdInstance)
				waitForResourceToBeReady(ctx, createdBinding)
			})
		})

		When("referenced service instance is being deleted", func() {
			It("should fail", func() {
				createdInstance.Finalizers = append(createdInstance.Finalizers, "fake/finalizer")
				updateInstance(ctx, createdInstance)
				Expect(k8sClient.Delete(ctx, createdInstance)).To(Succeed())

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "", false)
				Expect(err).ToNot(HaveOccurred())
				waitForResourceCondition(ctx, createdBinding, common.ConditionSucceeded, metav1.ConditionFalse, common.Blocked, "")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, getResourceNamespacedName(createdInstance), createdInstance)
					return err == nil && utils.RemoveFinalizer(ctx, k8sClient, createdInstance, "fake/finalizer") == nil
				}, timeout, interval).Should(BeTrue())
			})
		})

		When("secretTemplate", func() {
			It("should succeed to create the secret", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: {{ .instance.plan }}
  annotations:
    instance_name: {{ .instance.instance_name }}
stringData:
  newKey: {{ .credentials.secret_key }}
  tags: {{ .instance.tags }}`)

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(isResourceReady(createdBinding)).To(BeTrue())
				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateSecretData(bindingSecret, "newKey", "secret_value")
				validateSecretData(bindingSecret, "tags", strings.Join(mergeInstanceTags(createdInstance.Status.Tags, createdInstance.Spec.CustomTags), ","))
				Expect(bindingSecret.Labels["instance_plan"]).To(Equal("a-plan-name"))
				Expect(bindingSecret.Annotations["instance_name"]).To(Equal(instanceExternalName))
			})
			It("should succeed to create the secret- when no kind", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(
					`metadata:
  labels:
    instance_plan: {{ .instance.plan }}
  annotations:
    instance_name: {{ .instance.instance_name }}
stringData:
  newKey: {{ .credentials.secret_key }}
  tags: {{ .instance.tags }}`)

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(isResourceReady(createdBinding)).To(BeTrue())
				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateSecretData(bindingSecret, "newKey", "secret_value")
				validateSecretData(bindingSecret, "tags", strings.Join(mergeInstanceTags(createdInstance.Status.Tags, createdInstance.Spec.CustomTags), ","))
				Expect(bindingSecret.Labels["instance_plan"]).To(Equal("a-plan-name"))
				Expect(bindingSecret.Annotations["instance_name"]).To(Equal(instanceExternalName))
			})
			It("should fail to create the secret if forbidden field is provided under spec.secretTemplate.metadata", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(`
				                                       apiVersion: v1
				                                       kind: Secret
				                                       metadata:
				                                         name: my-secret-name`)
				binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, false)
				Expect(err).ToNot(HaveOccurred())
				waitForResourceCondition(ctx, binding, common.ConditionFailed, metav1.ConditionTrue, "", "the Secret template is invalid: Secret's metadata field")
			})
			It("should fail to create the secret if wrong template key in the spec.secretTemplate is provided", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(`
				                                       apiVersion: v1
				                                       kind: Secret
				                                       stringData:
				                                         foo: {{ .non_existing_key }}`)

				binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, false)
				Expect(err).To(BeNil())
				bindingLookupKey := getResourceNamespacedName(binding)
				Eventually(func() bool {
					if err := k8sClient.Get(ctx, bindingLookupKey, binding); err != nil {
						return false
					}
					cond := meta.FindStatusCondition(binding.GetConditions(), common.ConditionSucceeded)
					return cond != nil && cond.Reason == "CreateFailed" && strings.Contains(cond.Message, "map has no entry for key \"non_existing_key\"")
				}, timeout*2, interval).Should(BeTrue())
			})
			It("should fail to create the secret if secretTemplate is an unexpected type", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(`
				                                       apiVersion: v1
				                                       kind: Pod`)
				binding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, false)
				Expect(err).ToNot(HaveOccurred())
				waitForResourceCondition(ctx, binding, common.ConditionFailed, metav1.ConditionTrue, "", "but needs to be of kind 'Secret'")
			})
			It("should succeed to create the secret- empty data", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: {{ .instance.plan }}
  annotations:
    instance_name: {{ .instance.instance_name }}
stringData:`)

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(isResourceReady(createdBinding)).To(BeTrue())
				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				Expect(bindingSecret.Labels["instance_plan"]).To(Equal("a-plan-name"))
				Expect(bindingSecret.Annotations["instance_name"]).To(Equal(instanceExternalName))
				validateSecretData(bindingSecret, "secret_key", "secret_value")
				validateSecretData(bindingSecret, "escaped", `{"escaped_key":"escaped_val"}`)
				validateInstanceInfo(bindingSecret, instanceExternalName)
				credentialProperties := []utils.SecretMetadataProperty{
					{
						Name:   "secret_key",
						Format: string(utils.TEXT),
					},
					{
						Name:   "escaped",
						Format: string(utils.TEXT),
					},
				}
				validateSecretMetadata(bindingSecret, credentialProperties)
			})
			It("should succeed to create the secret- no data", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: {{ .instance.plan }}
  annotations:
    instance_name: {{ .instance.instance_name }}`)

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(isResourceReady(createdBinding)).To(BeTrue())
				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				Expect(bindingSecret.Labels["instance_plan"]).To(Equal("a-plan-name"))
				Expect(bindingSecret.Annotations["instance_name"]).To(Equal(instanceExternalName))
				validateSecretData(bindingSecret, "secret_key", "secret_value")
				validateSecretData(bindingSecret, "escaped", `{"escaped_key":"escaped_val"}`)
				validateInstanceInfo(bindingSecret, instanceExternalName)
				credentialProperties := []utils.SecretMetadataProperty{
					{
						Name:   "secret_key",
						Format: string(utils.TEXT),
					},
					{
						Name:   "escaped",
						Format: string(utils.TEXT),
					},
				}
				validateSecretMetadata(bindingSecret, credentialProperties)
			})
			It("should succeed to create the secret, with depth key", func() {

				fakeClient.BindReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage(`{ "auth":{ "basic":{ "password":"secret_value","userName":"name"}},"url":"yourluckyday.com"}`)}, "", nil)

				ctx := context.Background()
				secretTemplate := dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: {{ .instance.plan }}
  annotations:
    instance_name: {{ .instance.instance_name }}
stringData:
  newKey: {{ .credentials.auth.basic.password }}
  tags: {{ .instance.tags }}`)

				createdBinding, err := createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "", secretTemplate, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(isResourceReady(createdBinding)).To(BeTrue())
				By("Verify binding secret created")
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				validateSecretData(bindingSecret, "newKey", "secret_value")
				validateSecretData(bindingSecret, "tags", strings.Join(mergeInstanceTags(createdInstance.Status.Tags, createdInstance.Spec.CustomTags), ","))
				Expect(bindingSecret.Labels["instance_plan"]).To(Equal("a-plan-name"))
				Expect(bindingSecret.Annotations["instance_name"]).To(Equal(instanceExternalName))
			})
		})
	})

	Context("Update", func() {
		BeforeEach(func() {
			secretTemplate := dedent.Dedent(
				`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: {{ .instance.plan }}
  annotations:
    instance_name: {{ .instance.instance_name }}
stringData:
  newKey: {{ .credentials.secret_key }}`)
			createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", secretTemplate)
			fakeClient.GetBindingByIDReturns(&smClientTypes.ServiceBinding{ID: fakeBindingID, Credentials: json.RawMessage("{\"secret_key\": \"secret_value\"}")}, nil)
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

		When("secretTemplate is changed", func() {
			It("should succeed to create the secret", func() {
				ctx := context.Background()
				secretTemplate := dedent.Dedent(
					`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: "a-new-plan-name"
  annotations:
    instance_name: "a-new-instance-name"
stringData:
  newKey2: {{ .credentials.secret_key }}`)
				createdBinding.Spec.SecretTemplate = secretTemplate
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).ToNot(HaveOccurred())
				By("Verify binding update succeeded")
				waitForResourceCondition(ctx, createdBinding, common.ConditionSucceeded, metav1.ConditionTrue, common.Updated, "")
				By("Verify binding secret created")
				Eventually(func() bool {
					bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
					return string(bindingSecret.Data["newKey2"]) == "secret_value" && bindingSecret.Labels["instance_plan"] == "a-new-plan-name" && bindingSecret.Annotations["instance_name"] == "a-new-instance-name"
				}, timeout, interval).Should(BeTrue())
			})
			It("secretTemplate removed default secret was created", func() {
				ctx := context.Background()
				createdBinding.Spec.SecretTemplate = ""
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).ToNot(HaveOccurred())
				By("Verify binding update succeeded")
				waitForResourceCondition(ctx, createdBinding, common.ConditionSucceeded, metav1.ConditionTrue, common.Updated, "")
				By("Verify binding secret created")
				Eventually(func() bool {
					bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
					return string(bindingSecret.Data["secret_key"]) == "secret_value"
				}, timeout, interval).Should(BeTrue())
			})

		})
		It("after fail should succeed to create the secret", func() {
			ctx := context.Background()
			secretTemplate := dedent.Dedent(
				`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: "a-new-plan-name"
  annotations:
    instance_name: "a-new-instance-name"
stringData:
  newKey2: {{ .credentials.nonexistingKey }}`)
			createdBinding.Spec.SecretTemplate = secretTemplate
			err := k8sClient.Update(ctx, createdBinding)
			Expect(err).ToNot(HaveOccurred())
			By("Verify binding update failed")
			waitForResourceCondition(ctx, createdBinding, common.ConditionFailed, metav1.ConditionTrue, "UpdateFailed", "failed to create secret")

			secretTemplate = dedent.Dedent(
				`apiVersion: v1
kind: Secret
metadata:
  labels:
    instance_plan: "a-new-plan-name"
  annotations:
    instance_name: "a-new-instance-name"
stringData:
  newKey2: {{ .credentials.secret_key }}`)
			createdBinding.Spec.SecretTemplate = secretTemplate
			err = k8sClient.Update(ctx, createdBinding)
			Expect(err).ToNot(HaveOccurred())

			By("Verify binding secret created")
			Eventually(func() bool {
				bindingSecret := getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
				return string(bindingSecret.Data["newKey2"]) == "secret_value" && bindingSecret.Labels["instance_plan"] == "a-new-plan-name" && bindingSecret.Annotations["instance_name"] == "a-new-instance-name"
			}, timeout, interval).Should(BeTrue())
		})

		When("UserInfo changed", func() {
			It("should fail", func() {
				createdBinding.Spec.UserInfo = &authv1.UserInfo{
					Username: "aaa",
					UID:      "111",
				}
				err := k8sClient.Update(ctx, createdBinding)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("modifying spec.userInfo is not allowed"))
			})
		})
	})

	Context("Delete", func() {

		BeforeEach(func() {
			createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "")
			Expect(isResourceReady(createdBinding)).To(BeTrue())
		})

		Context("Sync", func() {
			When("delete in SM succeeds", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturns("", nil)
				})
				It("should delete the k8s binding and secret", func() {
					deleteAndWait(ctx, createdBinding)
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
					deleteAndWait(ctx, createdBinding)
				})
			})

			When("delete in SM fails with general error", func() {
				errorMessage := "some-error"
				BeforeEach(func() {
					fakeClient.UnbindReturns("", errors.New(errorMessage))
				})
				AfterEach(func() {
					fakeClient.UnbindReturns("", nil)
					deleteAndWait(ctx, createdBinding)
				})

				It("should not remove finalizer and keep the secret", func() {
					Expect(k8sClient.Delete(ctx, createdBinding)).To(Succeed())
					Eventually(func() bool {
						err := k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: createdBinding.Namespace}, createdBinding)
						if err != nil {
							return false
						}
						failedCond := meta.FindStatusCondition(createdBinding.GetConditions(), common.ConditionFailed)
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
					deleteAndWait(ctx, createdBinding)
				})
			})

			When("delete orphan binding with finalizer", func() {
				BeforeEach(func() {
					fakeClient.UnbindReturns("", nil)
				})
				It("should succeed", func() {
					createdBinding, err := createBindingWithoutAssertions(ctx, bindingName+"-new", bindingTestNamespace, "non-exist-instance", "", "binding-external-name", "", false)
					Expect(err).ToNot(HaveOccurred())
					createdBinding.Finalizers = []string{common.FinalizerName}
					Expect(k8sClient.Update(ctx, createdBinding))
					Eventually(func() bool {
						err := k8sClient.Get(ctx, getResourceNamespacedName(createdBinding), createdBinding)
						if err != nil {
							return false
						}
						cond := meta.FindStatusCondition(createdBinding.GetConditions(), common.ConditionSucceeded)
						return cond != nil && cond.Reason == common.Blocked
					}, timeout, interval).Should(BeTrue())
					deleteAndWait(ctx, createdBinding)
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
					deleteAndWait(ctx, createdBinding)
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
						failedCond := meta.FindStatusCondition(createdBinding.GetConditions(), common.ConditionFailed)
						return failedCond != nil && strings.Contains(failedCond.Message, errorMessage)
					}, timeout, interval).Should(BeTrue())
					fakeClient.UnbindReturns("", nil)
					deleteAndWait(ctx, createdBinding)
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
						cond := meta.FindStatusCondition(createdBinding.GetConditions(), common.ConditionSucceeded)
						return cond != nil && strings.Contains(cond.Message, string(smClientTypes.INPROGRESS))
					}, timeout, interval).Should(BeTrue())
					fakeClient.UnbindReturns("", nil)
					deleteAndWait(ctx, createdBinding)
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
						createdBinding, err = createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "fake-binding-external-name", "", false)
						Expect(err).ToNot(HaveOccurred())
						smCallArgs := fakeClient.ListBindingsArgsForCall(0)
						Expect(smCallArgs.LabelQuery).To(HaveLen(1))
						Expect(smCallArgs.LabelQuery[0]).To(ContainSubstring("_k8sname"))

						Expect(smCallArgs.FieldQuery).To(HaveLen(3))
						Expect(smCallArgs.FieldQuery[0]).To(ContainSubstring("name"))
						Expect(smCallArgs.FieldQuery[1]).To(ContainSubstring("context/clusterid"))
						Expect(smCallArgs.FieldQuery[2]).To(ContainSubstring("context/namespace"))

						waitForResourceCondition(ctx, createdBinding, common.ConditionSucceeded, testCase.expectedConditionSucceededStatus, utils.GetConditionReason(testCase.lastOpType, testCase.lastOpState), "")

						switch testCase.lastOpState {
						case smClientTypes.FAILED:
							Expect(utils.IsFailed(createdBinding))
						case smClientTypes.INPROGRESS:
							Expect(utils.IsInProgress(createdBinding))
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
				createdBinding, err = createBindingWithoutAssertions(ctx, bindingName, bindingTestNamespace, instanceName, "", "fake-binding-external-name", "", false)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("Credential Rotation", func() {
		BeforeEach(func() {
			fakeClient.RenameBindingReturns(nil, nil)
			createdBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, "", "binding-external-name", "")
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
				Expect(k8sClient.List(ctx, bindingList, client.MatchingLabels{common.StaleBindingIDLabel: myBinding.Status.BindingID}, client.InNamespace(bindingTestNamespace))).To(Succeed())
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
				common.ForceRotateAnnotation: "true",
			}

			updateBinding(ctx, defaultLookupKey, createdBinding)
			myBinding := &v1.ServiceBinding{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, defaultLookupKey, myBinding)
				return err == nil && myBinding.Status.LastCredentialsRotationTime != nil
			}, timeout, interval).Should(BeTrue())

			_, ok := myBinding.Annotations[common.ForceRotateAnnotation]
			Expect(ok).To(BeFalse())
		})

		When("original binding ready=true", func() {
			It("should delete old binding when stale", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := generateBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					common.StaleBindingIDLabel:         createdBinding.Status.BindingID,
					common.StaleBindingRotationOfLabel: createdBinding.Name,
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
				waitForResourceCondition(ctx, failedBinding, common.ConditionSucceeded, metav1.ConditionFalse, common.Blocked, "")
			})

			It("should not delete old binding when stale", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := generateBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					common.StaleBindingIDLabel:         "1234",
					common.StaleBindingRotationOfLabel: failedBinding.Name,
				}
				staleBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           false,
					RotatedBindingTTL: "0ns",
					RotationFrequency: "0ns",
				}
				Expect(k8sClient.Create(ctx, staleBinding)).To(Succeed())
				waitForResourceCondition(ctx, staleBinding, common.ConditionPendingTermination, metav1.ConditionTrue, common.ConditionPendingTermination, "")
			})
		})

		When("stale binding is missing rotationOf label", func() {
			It("should delete the binding", func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: createdBinding.Name, Namespace: bindingTestNamespace}, createdBinding)).To(Succeed())
				staleBinding := generateBasicStaleBinding(createdBinding)
				staleBinding.Labels = map[string]string{
					common.StaleBindingIDLabel: createdBinding.Status.BindingID,
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
		var serviceInstanceInAnotherNamespace *v1.ServiceInstance
		BeforeEach(func() {
			serviceInstanceInAnotherNamespace = createInstance(ctx, instanceName, testNamespace, instanceExternalName)
		})

		AfterEach(func() {
			deleteAndWait(ctx, serviceInstanceInAnotherNamespace)
		})

		When("binding is created in a different namespace than the instance", func() {
			AfterEach(func() {
				if crossBinding != nil {
					Expect(k8sClient.Delete(ctx, crossBinding))
				}
			})
			It("should succeed", func() {
				crossBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, testNamespace, "cross-binding-external-name", "")

				By("Verify binding secret created")
				getSecret(ctx, createdBinding.Spec.SecretName, createdBinding.Namespace, true)
			})
		})

		Context("cred rotation", func() {
			BeforeEach(func() {
				fakeClient.RenameBindingReturns(nil, nil)
				crossBinding = createAndValidateBinding(ctx, bindingName, bindingTestNamespace, instanceName, testNamespace, "cross-binding-external-name", "")
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
					deleteAndWait(ctx, crossBinding)
				}
			})

			It("should rotate the credentials and create old binding", func() {
				key := types.NamespacedName{Name: bindingName, Namespace: bindingTestNamespace}
				Expect(k8sClient.Get(ctx, key, crossBinding)).To(Succeed())
				crossBinding.Spec.CredRotationPolicy = &v1.CredentialsRotationPolicy{
					Enabled:           true,
					RotatedBindingTTL: "1h",
					RotationFrequency: "1ns",
				}

				var secret *corev1.Secret
				Eventually(func() bool {
					secret = getSecret(ctx, crossBinding.Spec.SecretName, bindingTestNamespace, true)
					secret.Data = map[string][]byte{}
					return k8sClient.Update(ctx, secret) == nil
				}, timeout, interval).Should(BeTrue())

				updateBinding(ctx, key, crossBinding)

				myBinding := &v1.ServiceBinding{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, key, myBinding)
					return err == nil && myBinding.Status.LastCredentialsRotationTime != nil && len(myBinding.Status.Conditions) == 2
				}, timeout, interval).Should(BeTrue())

				secret = getSecret(ctx, myBinding.Spec.SecretName, bindingTestNamespace, true)
				val := secret.Data["secret_key"]
				Expect(string(val)).To(Equal("secret_value"))

				bindingList := &v1.ServiceBindingList{}
				Eventually(func() bool {
					Expect(k8sClient.List(ctx, bindingList, client.MatchingLabels{common.StaleBindingIDLabel: myBinding.Status.BindingID}, client.InNamespace(bindingTestNamespace))).To(Succeed())
					return len(bindingList.Items) > 0
				}, timeout, interval).Should(BeTrue())
				oldBinding := bindingList.Items[0]
				Expect(oldBinding.Spec.CredRotationPolicy.Enabled).To(BeFalse())

				secret = getSecret(ctx, oldBinding.Spec.SecretName, bindingTestNamespace, true)
				val = secret.Data["secret_key2"]
				Expect(string(val)).To(Equal("secret_value2"))
			})
		})
	})
})

func generateBasicBindingTemplate(name, namespace, instanceName, instanceNamespace, externalName string, secretTemplate string) *v1.ServiceBinding {
	binding := newBindingObject(name, namespace)
	binding.Spec.ServiceInstanceName = instanceName
	if len(instanceNamespace) > 0 {
		binding.Spec.ServiceInstanceNamespace = instanceNamespace
	}
	binding.Spec.ExternalName = externalName
	binding.Spec.SecretTemplate = secretTemplate
	binding.Spec.Parameters = &runtime.RawExtension{
		Raw: []byte(`{"key": "value"}`),
	}
	binding.Spec.ParametersFrom = []v1.ParametersFromSource{
		{
			SecretKeyRef: &v1.SecretKeyReference{
				Name: "binding-params-secret",
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

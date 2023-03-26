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
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	shareInstanceTestNamespace = "test-namespace"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("SharedServiceInstance controller", func() {
	var sharedInstanceName string
	var guid string
	var createdSharedServiceInstance *v1.SharedServiceInstance
	var fakeInstanceName string
	var defaultLookupKey types.NamespacedName

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

	createSharedInstanceWithoutAssertionsAndWait := func(ctx context.Context, name string, namespace string, wait bool) (*v1.SharedServiceInstance, error) {
		ssi := newSharedInstanceObject(name, namespace)
		ssi.Spec.ServiceInstanceName = name
		if err := k8sClient.Create(ctx, ssi); err != nil {
			return nil, err
		}
		ssiLookupKey := types.NamespacedName{Name: name, Namespace: shareInstanceTestNamespace}
		createdSharedServiceInstance = &v1.SharedServiceInstance{}

		Eventually(func() bool {
			err := k8sClient.Get(ctx, ssiLookupKey, createdSharedServiceInstance)
			if err != nil {
				return false
			}
			if wait {
				return isReady(createdSharedServiceInstance) || isFailed(createdSharedServiceInstance)
			} else {
				return len(createdSharedServiceInstance.Status.Conditions) > 0 && createdSharedServiceInstance.Status.Conditions[0].Message != "Pending"
			}
		}, timeout, interval).Should(BeTrue())
		return createdSharedServiceInstance, nil
	}

	createSharedInstanceWithError := func(ctx context.Context, name, namespace, failureMessage string) {
		ssi, err := createSharedInstanceWithoutAssertionsAndWait(ctx, name, namespace, true)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring(failureMessage))
		} else {
			Expect(ssi.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		}
	}

	createInstance := func(ctx context.Context, instanceSpec v1.ServiceInstanceSpec) *v1.ServiceInstance {
		instance := &v1.ServiceInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "services.cloud.sap.com/v1",
				Kind:       "ServiceInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fakeInstanceName,
				Namespace: shareInstanceTestNamespace,
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
			return createdInstance.GetObservedGeneration() > 0
		}, timeout, interval).Should(BeTrue())

		return createdInstance
	}

	createSharedInstanceWithoutAssertions := func(ctx context.Context, name string, namespace string) (*v1.SharedServiceInstance, error) {
		return createSharedInstanceWithoutAssertionsAndWait(ctx, name, namespace, true)
	}

	createSharedInstance := func(ctx context.Context, instanceName, namespace string) *v1.SharedServiceInstance {
		createdSharedServiceInstance, err := createSharedInstanceWithoutAssertions(ctx, instanceName, namespace)
		Expect(err).ToNot(HaveOccurred())
		Expect(int(createdSharedServiceInstance.Status.ObservedGeneration)).To(Equal(1))
		return createdSharedServiceInstance
	}

	deleteInstance := func(ctx context.Context, instanceToDelete *v1.SharedServiceInstance, wait bool) {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}, &v1.SharedServiceInstance{})
		if err != nil {
			Expect(apierrors.IsNotFound(err)).To(Equal(true))
			return
		}

		Expect(k8sClient.Delete(ctx, instanceToDelete)).Should(Succeed())

		if wait {
			Eventually(func() bool {
				a := &v1.SharedServiceInstance{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}, a)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}
	}

	ctx := context.Background()

	BeforeEach(func() {
		guid = uuid.New().String()
		sharedInstanceName = "test-ssi-" + guid
		fakeInstanceName = "fake-name" + guid
		defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}
		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: "12345678", Tags: []byte("[\"test\"]")}, nil)
		smInstance := &smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.UPDATE}}
		fakeClient.GetInstanceByIDReturns(smInstance, nil)
		secret := &corev1.Secret{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: shareInstanceTestNamespace, Name: "param-secret"}, secret)
		if apierrors.IsNotFound(err) {
			createParamsSecret(shareInstanceTestNamespace)
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("Create", func() {
		Context("Invalid parameters", func() {
			When("namespace is not provided", func() {
				It("should fail", func() {
					createSharedInstanceWithError(context.Background(), sharedInstanceName, "",
						"an empty namespace may not be set during creation")
				})
			})

			When("serviceInstanceName is not provided", func() {
				It("should fail", func() {
					createSharedInstanceWithError(context.Background(), "", shareInstanceTestNamespace,
						"serviceInstanceName in body should be at least 1 chars long")
				})
			})
		})

		Context("Valid params", func() {
			It("Should eventually create shared service instance", func() {
				fakeInstanceName = "ic-test-" + uuid.New().String()
				defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}
				fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
				serviceInstance := createInstance(ctx, instanceSpec)

				fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				fakeClient.ShareInstanceReturns(nil)
				createdSharedServiceInstance = createSharedInstance(ctx, serviceInstance.Name, shareInstanceTestNamespace)
				Expect(createdSharedServiceInstance.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
				Expect(createdSharedServiceInstance.Status.Conditions[0].Type).To(Equal(api.ConditionReady))
			})
		})
	})

	Context("Update", func() {
		It("should fail in the webhook", func() {
			ctx := context.Background()
			fakeInstanceName = "ic-test-" + uuid.New().String()
			defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}
			fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
			fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
			serviceInstance := createInstance(ctx, instanceSpec)
			fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
			fakeClient.ShareInstanceReturns(nil)
			createdSharedServiceInstance = createSharedInstance(ctx, serviceInstance.Name, shareInstanceTestNamespace)
			createdSharedServiceInstance.Spec.ServiceInstanceName = "new-name"
			fmt.Println("Now updating")
			err := k8sClient.Update(context.Background(), createdSharedServiceInstance)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("updating shared service instance is not supported"))
		})
	})

	Context("Delete", func() {
		When("Un-sharing the instance succeed", func() {
			It("should succeed deleting the sharingServiceInstance resource", func() {
				ctx := context.Background()

				defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}
				fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
				serviceInstance := createInstance(ctx, instanceSpec)

				fakeClient.ShareInstanceReturns(nil)
				createdSharedServiceInstance = createSharedInstance(ctx, serviceInstance.Name, shareInstanceTestNamespace)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: shareInstanceTestNamespace}, createdSharedServiceInstance)
					if err == nil {
						return createdSharedServiceInstance.Status.Shared == metav1.ConditionTrue
					}
					return false
				}, timeout, interval).Should(BeTrue())

				fakeClient.ShareInstanceReturns(nil)
				deleteInstance(ctx, createdSharedServiceInstance, true)
			})
		})

		When("Un-sharing the instance fails", func() {
			It("should fail deleting the sharingServiceInstance resource and update the status", func() {
				ctx := context.Background()

				defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}
				fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
				serviceInstance := createInstance(ctx, instanceSpec)
				fakeClient.ShareInstanceReturns(nil)

				createdSharedServiceInstance = createSharedInstance(ctx, serviceInstance.Name, shareInstanceTestNamespace)
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: shareInstanceTestNamespace}, createdSharedServiceInstance)
					if err == nil {
						return createdSharedServiceInstance.Status.Shared == metav1.ConditionTrue
					}
					return false
				}, timeout, interval).Should(BeTrue())

				err := k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: shareInstanceTestNamespace}, createdSharedServiceInstance)
				Expect(err).To(BeNil())

				fakeClient.ShareInstanceReturns(fmt.Errorf("failed to unshre instance"))
				k8sClient.Delete(ctx, createdSharedServiceInstance)
				Eventually(func() bool {
					k8sClient.Get(ctx, types.NamespacedName{Name: serviceInstance.Name, Namespace: shareInstanceTestNamespace}, createdSharedServiceInstance)
					return len(createdSharedServiceInstance.Status.Conditions) > 2
				}, timeout, interval).Should(BeTrue())
				Expect(createdSharedServiceInstance.Status.Conditions[2].Type == api.ConditionFailed)
				Expect(createdSharedServiceInstance.Status.Conditions[2].Message == "failed un-share")
			})
		})
	})
})

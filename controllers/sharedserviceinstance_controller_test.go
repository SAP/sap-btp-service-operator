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
	var serviceInstanceName string
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

	createSharedInstanceWithoutAssertionsAndWait := func(ctx context.Context, name string, namespace string, instanceName string, wait bool) (*v1.SharedServiceInstance, error) {
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

	createSharedInstanceWithError := func(ctx context.Context, name, namespace, instanceName, failureMessage string) {
		ssi, err := createSharedInstanceWithoutAssertionsAndWait(ctx, name, namespace, instanceName, true)
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

	createSharedInstanceWithoutAssertions := func(ctx context.Context, name string, namespace string, instanceName string) (*v1.SharedServiceInstance, error) {
		return createSharedInstanceWithoutAssertionsAndWait(ctx, name, namespace, instanceName, true)
	}

	createSharedInstance := func(ctx context.Context, instanceName, namespace, sharedInstanceName string) *v1.SharedServiceInstance {
		createdSharedServiceInstance, err := createSharedInstanceWithoutAssertions(ctx, instanceName, namespace, sharedInstanceName)
		Expect(err).ToNot(HaveOccurred())

		Expect(int(createdSharedServiceInstance.Status.ObservedGeneration)).To(Equal(1))
		return createdSharedServiceInstance
	}

	ctx := context.Background()

	BeforeEach(func() {
		guid = uuid.New().String()
		sharedInstanceName = "test-ssi-" + guid
		serviceInstanceName = "fake-instance-name" + guid
		fakeInstanceName = "fake-name" + guid
		defaultLookupKey = types.NamespacedName{Name: serviceInstanceName, Namespace: shareInstanceTestNamespace}
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
			When("service instance name is not provided", func() {
				It("should fail", func() {
					createSharedInstanceWithError(context.Background(), sharedInstanceName, shareInstanceTestNamespace, "",
						"spec.serviceInstanceName in body should be at least 1 chars long")

				})
			})
		})

		FContext("Valid params", func() {
			It("Should eventually create shared service instance", func() {
				fakeInstanceName = "ic-test-" + uuid.New().String()
				defaultLookupKey = types.NamespacedName{Name: fakeInstanceName, Namespace: shareInstanceTestNamespace}
				fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: fakeInstanceID}, nil)
				serviceInstance := createInstance(ctx, instanceSpec)

				fakeClient.GetInstanceByIDReturns(&smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.CREATE}}, nil)
				fakeClient.ShareInstanceReturns(nil)
				createdSharedServiceInstance = createSharedInstance(ctx, serviceInstance.Name, shareInstanceTestNamespace, sharedInstanceName)
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
			createdSharedServiceInstance = createSharedInstance(ctx, serviceInstance.Name, shareInstanceTestNamespace, sharedInstanceName)
			createdSharedServiceInstance.Spec.ServiceInstanceName = "new-name"
			fmt.Println("Now updating")
			err := k8sClient.Update(context.Background(), createdSharedServiceInstance)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("updating shared service instance is not supported"))
		})
	})
})

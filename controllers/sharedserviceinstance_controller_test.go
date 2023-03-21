package controllers

import (
	"context"
	"fmt"
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

	createSharedInstanceWithoutAssertionsAndWait := func(ctx context.Context, name string, namespace string, instanceName string, wait bool) (*v1.SharedServiceInstance, error) {
		ssi := newSharedInstanceObject(name, namespace)
		ssi.Spec.ServiceInstanceName = name
		fmt.Println("creating")
		if err := k8sClient.Create(ctx, ssi); err != nil {
			return nil, err
		}
		fmt.Println("Done creating")
		ssiLookupKey := types.NamespacedName{Name: name, Namespace: namespace}
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
			Expect(ssi.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		}
	}
	BeforeEach(func() {
		guid = uuid.New().String()
		sharedInstanceName = "test-ssi-" + guid
		fakeClient = &smfakes.FakeClient{}
		fakeClient.ProvisionReturns(&sm.ProvisionResponse{InstanceID: "12345678", Tags: []byte("[\"test\"]")}, nil)
		smInstance := &smclientTypes.ServiceInstance{ID: fakeInstanceID, Ready: true, LastOperation: &smClientTypes.Operation{State: smClientTypes.SUCCEEDED, Type: smClientTypes.UPDATE}}
		fakeClient.GetInstanceByIDReturns(smInstance, nil)
		secret := &corev1.Secret{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: bindingTestNamespace, Name: "param-secret"}, secret)
		if apierrors.IsNotFound(err) {
			createParamsSecret(bindingTestNamespace)
		} else {
			Expect(err).ToNot(HaveOccurred())
		}
	})

	FContext("Create", func() {
		Context("Invalid parameters", func() {
			When("service instance name is not provided", func() {
				It("should fail", func() {
					fmt.Println("Starting test")
					createSharedInstanceWithError(context.Background(), sharedInstanceName, shareInstanceTestNamespace, "",
						"spec.serviceInstanceName in body should be at least 1 chars long")

				})
			})
		})
	})
})

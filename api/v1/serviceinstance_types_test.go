package v1

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"

	"github.com/SAP/sap-btp-service-operator/api/common"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

var _ = Describe("Service Instance Type Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
		conditions := instance.GetConditions()
		lastOpCondition := metav1.Condition{Type: common.ConditionSucceeded, Status: metav1.ConditionTrue, Reason: "reason", Message: "message"}
		meta.SetStatusCondition(&conditions, lastOpCondition)
		instance.SetConditions(conditions)
	})

	It("should clone correctly", func() {
		clonedInstance := instance.DeepClone()
		Expect(instance).To(Equal(clonedInstance))

		clonedStatus := instance.Status.DeepCopy()
		Expect(&instance.Status).To(Equal(clonedStatus))

		clonedSpec := instance.Spec.DeepCopy()
		Expect(&instance.Spec).To(Equal(clonedSpec))

		list := &ServiceInstanceList{Items: []ServiceInstance{*instance}}
		clonedList := list.DeepCopy()
		Expect(list).To(Equal(clonedList))
	})

	It("should clone into correctly", func() {
		clonedInstance := &ServiceInstance{}
		instance.DeepCopyInto(clonedInstance)
		Expect(instance).To(Equal(clonedInstance))

		clonedStatus := &ServiceInstanceStatus{}
		instance.Status.DeepCopyInto(clonedStatus)
		Expect(&instance.Status).To(Equal(clonedStatus))

		clonedSpec := &ServiceInstanceSpec{}
		instance.Spec.DeepCopyInto(clonedSpec)
		Expect(&instance.Spec).To(Equal(clonedSpec))

		list := &ServiceInstanceList{Items: []ServiceInstance{*instance}}
		clonedList := &ServiceInstanceList{}
		list.DeepCopyInto(clonedList)
		Expect(list).To(Equal(clonedList))
	})

	It("should deep copy object correctly", func() {
		obj := instance.DeepCopyObject()
		Expect(instance).To(Equal(obj.(*ServiceInstance)))

		list := &ServiceInstanceList{Items: []ServiceInstance{*instance}}
		clonedList := list.DeepCopyObject()
		Expect(list).To(Equal(clonedList))
	})

	It("clone ParametersFromSource", func() {
		params := ParametersFromSource{
			SecretKeyRef: &SecretKeyReference{
				Name: "param-secret",
				Key:  "secret-parameter",
			},
		}

		clonedParams := params.DeepCopy()
		Expect(params.SecretKeyRef).To(Equal(clonedParams.SecretKeyRef))

		clonedParams2 := &ParametersFromSource{}
		params.DeepCopyInto(clonedParams2)
		Expect(params.SecretKeyRef).To(Equal(clonedParams2.SecretKeyRef))
	})

	It("clone SecretKeyRef", func() {
		secret := &SecretKeyReference{
			Name: "param-secret",
			Key:  "secret-parameter",
		}

		clonedSecret := secret.DeepCopy()
		Expect(secret).To(Equal(clonedSecret))

		clonedSecret2 := &SecretKeyReference{}
		secret.DeepCopyInto(clonedSecret2)
		Expect(secret).To(Equal(clonedSecret2))
	})

	It("should return controller name", func() {
		Expect(instance.GetControllerName()).To(Equal(common.ServiceInstanceController))
	})

	It("should update ready", func() {
		Expect(instance.Status.Ready).To(Equal(metav1.ConditionStatus("")))
		instance.SetReady(metav1.ConditionTrue)
		Expect(instance.GetReady()).To(Equal(metav1.ConditionTrue))
	})

	It("should get parameters", func() {
		params := &runtime.RawExtension{
			Raw: []byte("{\"key\":\"val\"}"),
		}
		instance.Spec.Parameters = params
		Expect(instance.GetParameters()).To(Equal(params))
	})

	It("should update status", func() {
		status := ServiceInstanceStatus{InstanceID: "1234"}
		instance.SetStatus(status)
		Expect(instance.GetStatus()).To(Equal(status))
	})

	It("should update annotation", func() {
		annotation := map[string]string{
			"key": "true",
		}
		instance.SetAnnotations(annotation)
		Expect(instance.GetAnnotations()).To(Equal(annotation))
	})

	It("should update WatchParametersFromChanges", func() {
		instance.Spec.WatchParametersFromChanges = &[]bool{true}[0]
		Expect(instance.IsSubscribedToParamSecretsChanges()).To(BeTrue())
	})

	It("should return correct spec hash", func() {
		// Calculate expected hash
		spec := instance.Spec
		spec.Shared = ptr.To(false)
		specBytes, _ := json.Marshal(spec)
		hash := md5.Sum(specBytes)
		expectedHash := hex.EncodeToString(hash[:])

		// Get actual hash
		actualHash := instance.GetSpecHash()

		// Compare hashes
		Expect(actualHash).To(Equal(expectedHash))
	})
	It("should update spec hash when spec changes", func() {
		// Calculate initial hash
		initialHash := instance.GetSpecHash()

		// Modify the spec
		instance.Spec.ServicePlanName = "new-plan"

		// Calculate new hash
		newHash := instance.GetSpecHash()

		// Ensure the hash has changed
		Expect(initialHash).NotTo(Equal(newHash))
	})
	It("should update spec hash when parametersFrom changes", func() {
		// Calculate initial hash
		initialHash := instance.GetSpecHash()

		// Modify the parametersFrom field
		instance.Spec.ParametersFrom = []ParametersFromSource{
			{
				SecretKeyRef: &SecretKeyReference{
					Name: "new-param-secret",
					Key:  "new-secret-parameter",
				},
			},
		}

		// Calculate new hash
		newHash := instance.GetSpecHash()

		// Ensure the hash has changed
		Expect(initialHash).NotTo(Equal(newHash))
	})
	It("should update spec hash when parametersFrom changes with initial object", func() {
		// Initialize ParametersFrom with an object
		instance.Spec.ParametersFrom = []ParametersFromSource{
			{
				SecretKeyRef: &SecretKeyReference{
					Name: "initial-param-secret",
					Key:  "initial-secret-parameter",
				},
			},
		}

		// Calculate initial hash
		initialHash := instance.GetSpecHash()

		// Modify the parametersFrom field
		instance.Spec.ParametersFrom = append(instance.Spec.ParametersFrom, ParametersFromSource{
			SecretKeyRef: &SecretKeyReference{
				Name: "new-param-secret",
				Key:  "new-secret-parameter",
			},
		})

		// Calculate new hash
		newHash := instance.GetSpecHash()

		// Ensure the hash has changed
		Expect(initialHash).NotTo(Equal(newHash))
	})
})

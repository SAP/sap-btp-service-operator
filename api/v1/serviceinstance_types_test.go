package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"time"
)

var _ = Describe("Service Instance Type Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
		conditions := instance.GetConditions()
		lastOpCondition := metav1.Condition{Type: api.ConditionSucceeded, Status: metav1.ConditionTrue, Reason: "reason", Message: "message"}
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
		Expect(instance.GetControllerName()).To(Equal(api.ServiceInstanceController))
	})

	It("should update observed generation", func() {
		Expect(instance.Status.ObservedGeneration).To(Equal(int64(0)))
		instance.SetObservedGeneration(2)
		Expect(instance.GetObservedGeneration()).To(Equal(int64(2)))
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
			api.IgnoreNonTransientErrorAnnotation: "true",
		}
		instance.SetAnnotations(annotation)
		Expect(instance.GetAnnotations()).To(Equal(annotation))
	})

	It("validate timestamp annotation- not date", func() {

		annotation := map[string]string{
			api.IgnoreNonTransientErrorAnnotation:          "true",
			api.IgnoreNonTransientErrorTimestampAnnotation: "true",
		}
		instance.SetAnnotations(annotation)
		err := api.ValidateNonTransientTimestampAnnotation(serviceinstancelog, instance)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("is not a valid timestamp"))
	})
	It("validate timestamp annotation- future date", func() {

		annotation := map[string]string{
			api.IgnoreNonTransientErrorAnnotation:          "true",
			api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Add(48 * time.Hour).Format(time.RFC3339),
		}
		instance.SetAnnotations(annotation)
		err := api.ValidateNonTransientTimestampAnnotation(serviceinstancelog, instance)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot be a future timestamp"))
	})
	It("validate annotation exist and valid", func() {

		annotation := map[string]string{
			api.IgnoreNonTransientErrorAnnotation:          "true",
			api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
		}
		instance.SetAnnotations(annotation)
		exist := api.IsIgnoreNonTransientAnnotationExistAndValid(serviceinstancelog, instance, time.Hour)
		Expect(exist).To(BeTrue())
	})
	It("validate annotation exist and valid", func() {

		annotation := map[string]string{
			api.IgnoreNonTransientErrorAnnotation:          "true",
			api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
		}
		instance.SetAnnotations(annotation)
		err := api.ValidateNonTransientTimestampAnnotation(serviceinstancelog, instance)
		Expect(err).NotTo(HaveOccurred())
	})
	It("validate timeout for Ignore Non Transient Error Annotation", func() {

		annotation := map[string]string{
			api.IgnoreNonTransientErrorAnnotation:          "true",
			api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Truncate(48 * time.Hour).Format(time.RFC3339),
		}
		instance.SetAnnotations(annotation)
		exist := api.IsIgnoreNonTransientAnnotationExistAndValid(serviceinstancelog, instance, time.Hour)
		Expect(exist).To(BeFalse())
	})
	It("validate annotation not exist", func() {

		annotation := map[string]string{
			api.IgnoreNonTransientErrorTimestampAnnotation: time.Now().Format(time.RFC3339),
		}
		instance.SetAnnotations(annotation)
		exist := api.IsIgnoreNonTransientAnnotationExistAndValid(serviceinstancelog, instance, time.Hour)
		Expect(exist).To(BeFalse())
	})
})

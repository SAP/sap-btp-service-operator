package v1

import (
	"github.com/SAP/sap-btp-service-operator/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Service Binding Type Test", func() {
	var binding *ServiceBinding
	BeforeEach(func() {
		binding = getBinding()
		conditions := binding.GetConditions()
		lastOpCondition := metav1.Condition{Type: api.ConditionSucceeded, Status: metav1.ConditionTrue, Reason: "reason", Message: "message"}
		meta.SetStatusCondition(&conditions, lastOpCondition)
		binding.SetConditions(conditions)
	})

	It("should clone correctly", func() {
		clonedBinding := binding.DeepClone()
		Expect(binding).To(Equal(clonedBinding))

		clonedStatus := binding.Status.DeepCopy()
		Expect(&binding.Status).To(Equal(clonedStatus))

		clonedSpec := binding.Spec.DeepCopy()
		Expect(&binding.Spec).To(Equal(clonedSpec))

		list := &ServiceBindingList{Items: []ServiceBinding{*binding}}
		clonedList := list.DeepCopy()
		Expect(list).To(Equal(clonedList))
	})

	It("should clone into correctly", func() {
		clonedBinding := &ServiceBinding{}
		binding.DeepCopyInto(clonedBinding)
		Expect(binding).To(Equal(clonedBinding))

		clonedStatus := &ServiceBindingStatus{}
		binding.Status.DeepCopyInto(clonedStatus)
		Expect(&binding.Status).To(Equal(clonedStatus))

		clonedSpec := &ServiceBindingSpec{}
		binding.Spec.DeepCopyInto(clonedSpec)
		Expect(&binding.Spec).To(Equal(clonedSpec))

		list := &ServiceBindingList{Items: []ServiceBinding{*binding}}
		clonedList := &ServiceBindingList{}
		list.DeepCopyInto(clonedList)
		Expect(list).To(Equal(clonedList))
	})

	It("should deep copy object correctly", func() {
		obj := binding.DeepCopyObject()
		Expect(binding).To(Equal(obj.(*ServiceBinding)))

		list := &ServiceBindingList{Items: []ServiceBinding{*binding}}
		clonedList := list.DeepCopyObject()
		Expect(list).To(Equal(clonedList))
	})

	It("should return controller name", func() {
		Expect(binding.GetControllerName()).To(Equal(api.ServiceBindingController))
	})

	It("should update observed generation", func() {
		Expect(binding.Status.ObservedGeneration).To(Equal(int64(0)))
		binding.SetObservedGeneration(2)
		Expect(binding.GetObservedGeneration()).To(Equal(int64(2)))
	})

	It("should update observed generation", func() {
		Expect(binding.Status.ObservedGeneration).To(Equal(int64(0)))
		binding.SetObservedGeneration(2)
		Expect(binding.Status.ObservedGeneration).To(Equal(int64(2)))
	})

	It("should update ready", func() {
		Expect(binding.Status.Ready).To(Equal(metav1.ConditionStatus("")))
		binding.SetReady(metav1.ConditionTrue)
		Expect(binding.GetReady()).To(Equal(metav1.ConditionTrue))
	})

	It("should get parameters", func() {
		params := &runtime.RawExtension{
			Raw: []byte("{\"key\":\"val\"}"),
		}
		binding.Spec.Parameters = params
		Expect(binding.GetParameters()).To(Equal(params))
	})

	It("should update status", func() {
		status := ServiceBindingStatus{BindingID: "1234"}
		binding.SetStatus(status)
		Expect(binding.GetStatus()).To(Equal(status))
	})

	It("should update annotation", func() {
		annotation := map[string]string{
			api.IgnoreNonTransientErrorAnnotation: "true",
		}
		binding.SetAnnotations(annotation)
		Expect(binding.GetAnnotations()).To(Equal(annotation))
	})
})

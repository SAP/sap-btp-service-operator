package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Service Instance Type Test", func() {
	var instance *ServiceInstance
	BeforeEach(func() {
		instance = getInstance()
		conditions := instance.GetConditions()
		lastOpCondition := metav1.Condition{Type: ConditionLastOpSucceeded, Status: metav1.ConditionTrue, Reason: "reason", Message: "message"}
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

	It("should return controller name", func() {
		Expect(instance.GetControllerName()).To(Equal(ServiceInstanceController))
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
})

/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var serviceinstanceglog = logf.Log.WithName("serviceinstance-resource")

// +kubebuilder:webhook:verbs=update,path=/validate-services-cloud-sap-com-v1-serviceinstance,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,versions=v1,name=vserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

func (si *ServiceInstance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(si).
		Complete()
}

var _ webhook.Validator = &ServiceInstance{}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (si *ServiceInstance) ValidateUpdate(old runtime.Object) error {
	serviceinstanceglog.Info("validate update", "name", si.Name)
	fmt.Println("instance validating webhook!!")
	newSharedState := si.Spec.Shared
	oldShareState := si.getOldSharedState()

	if !si.sharedStateChanged(newSharedState, oldShareState) {
		fmt.Println("no change!!")
		return nil
	}

	if si.specChanged(old) {
		return fmt.Errorf("updating share property is unabled with other spec changes")
	}
	return nil
}

func (si *ServiceInstance) specChanged(old runtime.Object) bool {
	oldInstance := old.(*ServiceInstance)
	oldSpec := oldInstance.Spec.DeepCopy()
	newSpec := si.Spec.DeepCopy()

	// We want to check if anything changed except of the shared
	oldSpec.Shared = false
	newSpec.Shared = false
	return !reflect.DeepEqual(oldSpec, newSpec)
}

func (si *ServiceInstance) ValidateCreate() error {
	return nil
}

func (si *ServiceInstance) ValidateDelete() error {
	return nil
}

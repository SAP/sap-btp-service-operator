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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func (ssi *SharedServiceInstance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(ssi).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=update,path=/validate-services-cloud-sap-com-v1-sharedserviceinstance,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=sharedserviceinstances,versions=v1,name=vsharedserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var _ webhook.Validator = &SharedServiceInstance{}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (ssi *SharedServiceInstance) ValidateUpdate(old runtime.Object) error {
	fmt.Println("Shared service instance validating webhook")
	specChanged := ssi.specChanged(old)
	if specChanged {
		fmt.Println("Err!!!")
		return fmt.Errorf("updating shared service instance is not supported")
	}
	return nil
}

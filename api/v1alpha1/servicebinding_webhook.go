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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var servicebindinglog = logf.Log.WithName("servicebinding-resource")

func (r *ServiceBinding) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-services-cloud-sap-com-v1alpha1-servicebinding,mutating=true,failurePolicy=fail,groups=services.cloud.sap.com,resources=servicebindings,verbs=create;update,versions=v1alpha1,name=mservicebinding.kb.io

var _ webhook.Defaulter = &ServiceBinding{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *ServiceBinding) Default() {
	servicebindinglog.Info("default", "name", r.Name)

	if len(r.Spec.ExternalName) == 0 {
		servicebindinglog.Info("externalName not provided, defaulting to k8s name", "name", r.Name)
		r.Spec.ExternalName = r.Name
	}
	if len(r.Spec.SecretName) == 0 {
		servicebindinglog.Info("secretName not provided, defaulting to k8s name", "name", r.Name)
		r.Spec.SecretName = r.Name
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-services-cloud-sap-com-v1alpha1-servicebinding,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=servicebindings,versions=v1alpha1,name=vservicebinding.kb.io

var _ webhook.Validator = &ServiceBinding{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ServiceBinding) ValidateCreate() error {
	servicebindinglog.Info("validate create", "name", r.Name)
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ServiceBinding) ValidateUpdate(old runtime.Object) error {
	servicebindinglog.Info("validate update", "name", r.Name)

	if r.specChanged(old) && r.Status.BindingID != "" {
		return fmt.Errorf("service binding spec cannot be modified after creation")
	}

	return nil
}

func (r *ServiceBinding) specChanged(old runtime.Object) bool {
	oldBinding := old.(*ServiceBinding)
	return r.Spec.ExternalName != oldBinding.Spec.ExternalName ||
		r.Spec.ServiceInstanceName != oldBinding.Spec.ServiceInstanceName ||
		// TODO + labels
		//r.Spec.Labels != oldBinding.Spec.Labels ||
		r.Spec.Parameters.String() != oldBinding.Spec.Parameters.String() ||
		r.Spec.SecretName != oldBinding.Spec.SecretName
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ServiceBinding) ValidateDelete() error {
	servicebindinglog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

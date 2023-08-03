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
	"github.com/SAP/sap-btp-service-operator/api"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (si *ServiceInstance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(si).
		Complete()
}

// +kubebuilder:webhook:verbs=delete,path=/validate-services-cloud-sap-com-v1-serviceinstance,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,versions=v1,name=vserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var _ webhook.Validator = &ServiceInstance{}

func (si *ServiceInstance) ValidateCreate() (warnings admission.Warnings, err error) {
	return nil, nil
}

func (si *ServiceInstance) ValidateUpdate(old runtime.Object) (warnings admission.Warnings, err error) {
	return nil, nil
}

func (si *ServiceInstance) ValidateDelete() (warnings admission.Warnings, err error) {
	if si.Annotations != nil {
		if _, ok := si.Annotations[api.PreventDeletion]; ok {
			return nil, fmt.Errorf("service instance %s marked as prevent deletion", si.Name)
		}
	}
	return nil, nil
}

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
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/SAP/sap-btp-service-operator/api"
	"github.com/SAP/sap-btp-service-operator/internal/secrets"
)

func (si *ServiceInstance) SetupWebhookWithManager(mgr ctrl.Manager, secretResolver *secrets.SecretResolver) error {
	enableMultipleSubaccounts = secretResolver.EnableMultipleSubaccounts
	return ctrl.NewWebhookManagedBy(mgr).
		For(si).
		Complete()
}

func (si *ServiceInstance) SetEnableMUltipleSubaccounts(e bool) {
	enableMultipleSubaccounts = e
}

// +kubebuilder:webhook:verbs=delete;update;create,path=/validate-services-cloud-sap-com-v1-serviceinstance,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,versions=v1,name=vserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var _ webhook.Validator = &ServiceInstance{}
var enableMultipleSubaccounts = false

// log is for logging in this package.
var serviceinstancelog = logf.Log.WithName("serviceinstance-resource")

func (si *ServiceInstance) ValidateCreate() (warnings admission.Warnings, err error) {
	serviceinstancelog.Info("validate create", "name", si.Name)
	if !enableMultipleSubaccounts && si.Spec.SubaccountID != "" {
		return nil, fmt.Errorf("using multiple subaccounts is disabled")
	}
	return nil, nil
}

func (si *ServiceInstance) ValidateUpdate(old runtime.Object) (warnings admission.Warnings, err error) {
	serviceinstancelog.Info("validate update", "name", si.Name)
	oldInstance := old.(*ServiceInstance)
	if oldInstance.Spec.SubaccountID != si.Spec.SubaccountID {
		return nil, fmt.Errorf("subaccountID spec field can not be changed")
	}
	return nil, nil
}

func (si *ServiceInstance) ValidateDelete() (warnings admission.Warnings, err error) {
	serviceinstancelog.Info("validate delete", "name", si.Name)
	if si.Annotations != nil {
		preventDeletion, ok := si.Annotations[api.PreventDeletion]
		if ok && strings.ToLower(preventDeletion) == "true" {
			return nil, fmt.Errorf("service instance '%s' is marked with \"prevent deletion\"", si.Name)
		}
	}
	return nil, nil
}

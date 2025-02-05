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

	"github.com/SAP/sap-btp-service-operator/api/common"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (si *ServiceInstance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(si).
		Complete()
}

// +kubebuilder:webhook:verbs=delete;update;create,path=/validate-services-cloud-sap-com-v1-serviceinstance,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,versions=v1,name=vserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var _ webhook.Validator = &ServiceInstance{}

// log is for logging in this package.
var serviceinstancelog = logf.Log.WithName("serviceinstance-resource")

func (si *ServiceInstance) ValidateCreate() (warnings admission.Warnings, err error) {
	return nil, nil
}

func (si *ServiceInstance) ValidateUpdate(old runtime.Object) (warnings admission.Warnings, err error) {
	serviceinstancelog.Info("validate update", "name", si.ObjectMeta.Name)

	oldInstance := old.(*ServiceInstance)
	if oldInstance.Spec.BTPAccessCredentialsSecret != si.Spec.BTPAccessCredentialsSecret {
		return nil, fmt.Errorf("changing the btpAccessCredentialsSecret for an existing instance is not allowed")
	}
	return nil, nil
}

func (si *ServiceInstance) ValidateDelete() (warnings admission.Warnings, err error) {
	serviceinstancelog.Info("validate delete", "name", si.ObjectMeta.Name)
	if si.ObjectMeta.Annotations != nil {
		preventDeletion, ok := si.ObjectMeta.Annotations[common.PreventDeletion]
		if ok && strings.ToLower(preventDeletion) == "true" {
			return nil, fmt.Errorf("service instance '%s' is marked with \"prevent deletion\"", si.ObjectMeta.Name)
		}
	}
	return nil, nil
}

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
	"time"

	"github.com/SAP/sap-btp-service-operator/api/common/utils"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var servicebindinglog = logf.Log.WithName("servicebinding-resource")
var secretTemplateError = "spec.secretTemplate is invalid"

func (sb *ServiceBinding) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(sb).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-services-cloud-sap-com-v1-servicebinding,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=servicebindings,versions=v1,name=vservicebinding.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var _ webhook.Validator = &ServiceBinding{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (sb *ServiceBinding) ValidateCreate() (admission.Warnings, error) {
	servicebindinglog.Info("validate create", "name", sb.Name)
	if sb.Spec.CredRotationPolicy != nil {
		if err := sb.validateCredRotatingConfig(); err != nil {
			return nil, err
		}
	}
	if sb.Spec.SecretTemplate != "" {
		if err := sb.validateSecretTemplate(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (sb *ServiceBinding) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	servicebindinglog.Info("validate update", "name", sb.Name)
	if sb.Spec.CredRotationPolicy != nil {
		if err := sb.validateCredRotatingConfig(); err != nil {
			return nil, err
		}
	}

	oldBinding := old.(*ServiceBinding)
	isStale := false
	if oldBinding.Labels != nil {
		if _, ok := oldBinding.Labels[common.StaleBindingIDLabel]; ok {
			if sb.Spec.CredRotationPolicy.Enabled {
				return nil, fmt.Errorf("enabling cred rotation for rotated binding is not allowed")
			}
			if !sb.validateRotationLabels(oldBinding) {
				return nil, fmt.Errorf("modifying rotation labels is not allowed")
			}
			isStale = true
		}
	}

	specChanged := sb.specChanged(oldBinding)
	if specChanged && (sb.Status.BindingID != "" || isStale) {
		return nil, fmt.Errorf("updating service bindings is not supported")
	}
	if sb.Spec.SecretTemplate != "" {
		if err := sb.validateSecretTemplate(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (sb *ServiceBinding) validateRotationLabels(old *ServiceBinding) bool {
	if sb.Labels[common.StaleBindingIDLabel] != old.Labels[common.StaleBindingIDLabel] {
		return false
	}
	return sb.Labels[common.StaleBindingRotationOfLabel] == old.Labels[common.StaleBindingRotationOfLabel]
}

func (sb *ServiceBinding) specChanged(oldBinding *ServiceBinding) bool {
	oldSpec := oldBinding.Spec.DeepCopy()
	newSpec := sb.Spec.DeepCopy()

	//allow changing cred rotation config
	oldSpec.CredRotationPolicy = nil
	newSpec.CredRotationPolicy = nil

	//allow changing SecretTemplate
	oldSpec.SecretTemplate = ""
	newSpec.SecretTemplate = ""

	return !reflect.DeepEqual(oldSpec, newSpec)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (sb *ServiceBinding) ValidateDelete() (admission.Warnings, error) {
	servicebindinglog.Info("validate delete", "name", sb.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (sb *ServiceBinding) validateCredRotatingConfig() error {
	_, err := time.ParseDuration(sb.Spec.CredRotationPolicy.RotatedBindingTTL)
	if err != nil {
		return err
	}
	_, err = time.ParseDuration(sb.Spec.CredRotationPolicy.RotationFrequency)
	if err != nil {
		return err
	}

	return nil
}

func (sb *ServiceBinding) validateSecretTemplate() error {
	servicebindinglog.Info("validate specified secretTemplate")
	x := make(map[string]interface{})
	y := make(map[string]string)
	parameters := utils.GetSecretDataForTemplate(x, y)

	templateName := fmt.Sprintf("%s/%s", sb.Namespace, sb.Name)
	_, err := utils.CreateSecretFromTemplate(templateName, sb.Spec.SecretTemplate, "missingkey=zero", parameters)
	if err != nil {
		servicebindinglog.Error(err, "failed to create secret from template")
		return errors.Wrap(err, secretTemplateError)
	}
	return nil
}

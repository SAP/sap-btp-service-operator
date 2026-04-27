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
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/SAP/sap-btp-service-operator/api/common"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var servicebindinglog = logf.Log.WithName("servicebinding-resource")

func (sb *ServiceBinding) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, sb).WithValidator(sb).Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-services-cloud-sap-com-v1-servicebinding,mutating=false,failurePolicy=fail,groups=services.cloud.sap.com,resources=servicebindings,versions=v1,name=vservicebinding.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var _ admission.Validator[*ServiceBinding] = &ServiceBinding{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (sb *ServiceBinding) ValidateCreate(_ context.Context, obj *ServiceBinding) (admission.Warnings, error) {
	servicebindinglog.Info("validate create", "name", obj.ObjectMeta.Name)
	if obj.Spec.CredRotationPolicy != nil {
		if err := obj.validateCredRotatingConfig(); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (sb *ServiceBinding) ValidateUpdate(_ context.Context, oldObj, newObj *ServiceBinding) (admission.Warnings, error) {
	servicebindinglog.Info("validate update", "name", newObj.ObjectMeta.Name)
	if newObj.Spec.CredRotationPolicy != nil {
		if err := newObj.validateCredRotatingConfig(); err != nil {
			return nil, err
		}
	}
	isStale := false
	if oldObj.Labels != nil {
		if _, ok := oldObj.Labels[common.StaleBindingIDLabel]; ok {
			if newObj.Spec.CredRotationPolicy.Enabled {
				return nil, fmt.Errorf("enabling cred rotation for rotated binding is not allowed")
			}
			if !newObj.validateRotationFields(oldObj) {
				return nil, fmt.Errorf("modifying rotation labels is not allowed")
			}
			isStale = true
		}
	}

	if newObj.Spec.UserInfo == nil {
		newObj.Spec.UserInfo = oldObj.Spec.UserInfo
	} else if !reflect.DeepEqual(newObj.Spec.UserInfo, oldObj.Spec.UserInfo) {
		return nil, fmt.Errorf("modifying spec.userInfo is not allowed")
	}

	isSpecChanged := newObj.specChanged(oldObj)
	if isSpecChanged && (newObj.Status.BindingID != "" || isStale) {

		return nil, fmt.Errorf("updating service bindings is not supported")
	}
	return nil, nil
}

func (sb *ServiceBinding) validateRotationFields(old *ServiceBinding) bool {
	if sb.ObjectMeta.Labels == nil {
		return false
	}

	isValid := sb.ObjectMeta.Labels[common.StaleBindingIDLabel] == old.ObjectMeta.Labels[common.StaleBindingIDLabel] &&
		sb.ObjectMeta.Labels[common.StaleBindingRotationOfLabel] == old.ObjectMeta.Labels[common.StaleBindingRotationOfLabel]

	if old.ObjectMeta.Annotations != nil && len(old.ObjectMeta.Annotations[common.StaleBindingOrigBindingNameAnnotation]) > 0 {
		isValid = isValid && sb.ObjectMeta.Annotations[common.StaleBindingOrigBindingNameAnnotation] == old.ObjectMeta.Annotations[common.StaleBindingOrigBindingNameAnnotation]
	}
	return isValid
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
func (sb *ServiceBinding) ValidateDelete(_ context.Context, _ *ServiceBinding) (admission.Warnings, error) {
	servicebindinglog.Info("validate delete", "name", sb.ObjectMeta.Name)

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

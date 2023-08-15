package webhooks

import (
	"context"
	"encoding/json"
	"net/http"

	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	v1admission "k8s.io/api/admission/v1"
	v1 "k8s.io/api/authentication/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-services-cloud-sap-com-v1-servicebinding,mutating=true,failurePolicy=fail,groups=services.cloud.sap.com,resources=servicebindings,verbs=create;update,versions=v1,name=mservicebinding.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var bindinglog = logf.Log.WithName("servicebinding-webhook")

type ServiceBindingDefaulter struct {
	Decoder *admission.Decoder
}

func (s *ServiceBindingDefaulter) Handle(_ context.Context, req admission.Request) admission.Response {
	bindinglog.Info("Defaulter webhook for servicebinding")
	binding := &servicesv1.ServiceBinding{}
	err := s.Decoder.Decode(req, binding)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// mutate the fields
	if len(binding.Spec.ExternalName) == 0 {
		bindinglog.Info("externalName not provided, defaulting to k8s name", "name", binding.Name)
		binding.Spec.ExternalName = binding.Name
	}
	if len(binding.Spec.SecretName) == 0 {
		bindinglog.Info("secretName not provided, defaulting to k8s name", "name", binding.Name)
		binding.Spec.SecretName = binding.Name
	}

	if binding.Spec.CredRotationPolicy != nil {
		if len(binding.Spec.CredRotationPolicy.RotationFrequency) == 0 {
			binding.Spec.CredRotationPolicy.RotationFrequency = "72h"
		}

		if len(binding.Spec.CredRotationPolicy.RotatedBindingTTL) == 0 {
			binding.Spec.CredRotationPolicy.RotatedBindingTTL = "48h"
		}
	}

	if req.Operation == v1admission.Create || req.Operation == v1admission.Delete {
		binding.Spec.UserInfo = &v1.UserInfo{
			Username: req.UserInfo.Username,
			UID:      req.UserInfo.UID,
			Groups:   req.UserInfo.Groups,
			Extra:    req.UserInfo.Extra,
		}
	}

	marshaledInstance, err := json.Marshal(binding)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledInstance)
}

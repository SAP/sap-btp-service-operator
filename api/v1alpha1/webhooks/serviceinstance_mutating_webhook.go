package webhooks

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/SAP/sap-btp-service-operator/api/v1alpha1"
	v1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-services-cloud-sap-com-v1alpha1-serviceinstance,mutating=true,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,verbs=create;update,versions=v1alpha1,name=mserviceinstance.kb.io

var instancelog = logf.Log.WithName("serviceinstance-resource")

type ServiceInstanceDefaulter struct {
	Client  client.Client
	decoder *admission.Decoder
}

func (s *ServiceInstanceDefaulter) Handle(_ context.Context, req admission.Request) admission.Response {
	instancelog.Info("Defaulter webhook for serviceinstance")
	instance := &v1alpha1.ServiceInstance{}
	err := s.decoder.Decode(req, instance)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// mutate the fields
	if len(instance.Spec.ExternalName) == 0 {
		instancelog.Info("externalName not provided, defaulting to k8s name", "name", instance.Name)
		instance.Spec.ExternalName = instance.Name
	}
	instance.Spec.UserInfo = &v1.UserInfo{
		Username: req.UserInfo.Username,
		UID:      req.UserInfo.UID,
		Groups:   req.UserInfo.Groups,
		Extra:    req.UserInfo.Extra,
	}

	marshaledInstance, err := json.Marshal(instance)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledInstance)
}

func (s *ServiceInstanceDefaulter) InjectDecoder(d *admission.Decoder) error {
	s.decoder = d
	return nil
}

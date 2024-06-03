package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	v1admission "k8s.io/api/admission/v1"
	v1 "k8s.io/api/authentication/v1"

	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-services-cloud-sap-com-v1-serviceinstance,mutating=true,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,verbs=create;update,versions=v1,name=mserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var instancelog = logf.Log.WithName("serviceinstance-webhook")

type ServiceInstanceDefaulter struct {
	Decoder admission.Decoder
}

func (s *ServiceInstanceDefaulter) Handle(_ context.Context, req admission.Request) admission.Response {
	instancelog.Info("Defaulter webhook for serviceinstance")
	instance := &servicesv1.ServiceInstance{}
	if err := s.Decoder.Decode(req, instance); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if req.Operation == v1admission.Create {
		instance.Spec.UserInfo = &v1.UserInfo{
			Username: req.UserInfo.Username,
			UID:      req.UserInfo.UID,
			Groups:   req.UserInfo.Groups,
			Extra:    req.UserInfo.Extra,
		}
	} else {
		oldInstance := &servicesv1.ServiceInstance{}
		err := s.Decoder.DecodeRaw(req.OldObject, oldInstance)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}

		if !reflect.DeepEqual(instance.Spec.UserInfo, oldInstance.Spec.UserInfo) {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("modifying spec.userInfo is not allowed"))
		} else if !reflect.DeepEqual(oldInstance.Spec, instance.Spec) { //UserInfo is updated only when spec is changed
			instance.Spec.UserInfo = &v1.UserInfo{
				Username: req.UserInfo.Username,
				UID:      req.UserInfo.UID,
				Groups:   req.UserInfo.Groups,
				Extra:    req.UserInfo.Extra,
			}
		}
	}
	if len(instance.Spec.ExternalName) == 0 {
		instancelog.Info(fmt.Sprintf("externalName not provided, defaulting to k8s name: %s", instance.Name))
		instance.Spec.ExternalName = instance.Name
	}

	marshaledInstance, err := json.Marshal(instance)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledInstance)
}

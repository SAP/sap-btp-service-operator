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
	Decoder *admission.Decoder
}

func (s *ServiceInstanceDefaulter) Handle(_ context.Context, req admission.Request) admission.Response {
	instancelog.Info("Defaulter webhook for serviceinstance")
	instance := &servicesv1.ServiceInstance{}
	if err := s.Decoder.Decode(req, instance); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if len(instance.Spec.ExternalName) == 0 {
		instancelog.Info(fmt.Sprintf("externalName not provided, defaulting to k8s name: %s", instance.Name))
		instance.Spec.ExternalName = instance.Name
	}

	if err := s.setServiceInstanceUserInfo(req, instance); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	marshaledInstance, err := json.Marshal(instance)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledInstance)
}

func (s *ServiceInstanceDefaulter) setServiceInstanceUserInfo(req admission.Request, instance *servicesv1.ServiceInstance) error {
	userInfo := &v1.UserInfo{
		Username: req.UserInfo.Username,
		UID:      req.UserInfo.UID,
		Groups:   req.UserInfo.Groups,
		Extra:    req.UserInfo.Extra,
	}

	if req.Operation == v1admission.Create || req.Operation == v1admission.Delete {
		instance.Spec.UserInfo = userInfo
	} else if req.Operation == v1admission.Update {
		oldInstance := &servicesv1.ServiceInstance{}
		err := s.Decoder.DecodeRaw(req.OldObject, oldInstance)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(oldInstance.Spec, instance.Spec) {
			instance.Spec.UserInfo = userInfo
		}
	}
	return nil
}

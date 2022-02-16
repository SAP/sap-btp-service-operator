package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/SAP/sap-btp-service-operator/api"

	v1admission "k8s.io/api/admission/v1"
	v1 "k8s.io/api/authentication/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/SAP/sap-btp-service-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-services-cloud-sap-com-v1alpha1-serviceinstance,mutating=true,failurePolicy=fail,groups=services.cloud.sap.com,resources=serviceinstances,verbs=create;update,versions=v1alpha1,name=mserviceinstance.kb.io,sideEffects=None,admissionReviewVersions=v1beta1;v1

var instancelog = logf.Log.WithName("serviceinstance-webhook")

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

	if instance.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(instance, api.FinalizerName) {
		controllerutil.AddFinalizer(instance, api.FinalizerName)
		instancelog.Info(fmt.Sprintf("added finalizer '%s' to service instance", api.FinalizerName))
	}

	// mutate the fields
	if len(instance.Spec.ExternalName) == 0 {
		instancelog.Info("externalName not provided, defaulting to k8s name", "name", instance.Name)
		instance.Spec.ExternalName = instance.Name
	}

	err = s.setServiceInstanceUserInfo(req, instance)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	marshaledInstance, err := json.Marshal(instance)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledInstance)
}

func (s *ServiceInstanceDefaulter) setServiceInstanceUserInfo(req admission.Request, instance *v1alpha1.ServiceInstance) error {
	userInfo := &v1.UserInfo{
		Username: req.UserInfo.Username,
		UID:      req.UserInfo.UID,
		Groups:   req.UserInfo.Groups,
		Extra:    req.UserInfo.Extra,
	}
	if req.Operation == v1admission.Create || req.Operation == v1admission.Delete {
		instance.Spec.UserInfo = userInfo
	} else if req.Operation == v1admission.Update {
		oldInstance := &v1alpha1.ServiceInstance{}
		err := s.decoder.DecodeRaw(req.OldObject, oldInstance)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(oldInstance.Spec, instance.Spec) {
			instance.Spec.UserInfo = userInfo
		}
	}
	return nil
}

func (s *ServiceInstanceDefaulter) InjectDecoder(d *admission.Decoder) error {
	s.decoder = d
	return nil
}

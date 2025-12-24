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

package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	commonutils "github.com/SAP/sap-btp-service-operator/api/common/utils"
	"github.com/SAP/sap-btp-service-operator/internal/utils/logutils"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pkg/errors"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/internal/config"
	"github.com/SAP/sap-btp-service-operator/internal/utils"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"fmt"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/google/uuid"

	"github.com/SAP/sap-btp-service-operator/client/sm"

	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	secretNameTakenErrorFormat    = "the specified secret name '%s' is already taken. Choose another name and try again"
	secretAlreadyOwnedErrorFormat = "secret %s belongs to another binding %s, choose a different name"
)

// ServiceBindingReconciler reconciles a ServiceBinding object
type ServiceBindingReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	GetSMClient func(ctx context.Context, instance *v1.ServiceInstance) (sm.Client, error)
	Config      config.Config
	Recorder    record.EventRecorder
}

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=servicebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *ServiceBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("servicebinding", req.NamespacedName).WithValues("correlation_id", uuid.New().String(), req.Name, req.Namespace)
	ctx = context.WithValue(ctx, logutils.LogKey, log)

	serviceBinding := &v1.ServiceBinding{}
	if err := r.Client.Get(ctx, req.NamespacedName, serviceBinding); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch ServiceBinding")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	serviceBinding = serviceBinding.DeepCopy()
	log.Info(fmt.Sprintf("Current generation is %v and observed is %v", serviceBinding.Generation, common.GetObservedGeneration(serviceBinding)))

	if len(serviceBinding.GetConditions()) == 0 {
		if err := utils.InitConditions(ctx, r.Client, serviceBinding); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		readyCond := meta.FindStatusCondition(serviceBinding.Status.Conditions, common.ConditionReady)
		if readyCond != nil && readyCond.Reason == common.ResourceNotFound {
			log.Info(fmt.Sprintf("binding id %s is not found for this cluster, deleting credentials", serviceBinding.Status.BindingID))
			if err := r.deleteBindingSecret(ctx, serviceBinding); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	serviceInstance, instanceErr := r.getServiceInstanceForBinding(ctx, serviceBinding)
	if instanceErr != nil {
		if !apierrors.IsNotFound(instanceErr) {
			log.Error(instanceErr, "failed to get service instance for binding")
			return ctrl.Result{}, instanceErr
		} else if !utils.IsMarkedForDeletion(serviceBinding.ObjectMeta) {
			//instance is not found and binding is not marked for deletion
			instanceNamespace := serviceBinding.Namespace
			if len(serviceBinding.Spec.ServiceInstanceNamespace) > 0 {
				instanceNamespace = serviceBinding.Spec.ServiceInstanceNamespace
			}
			errMsg := fmt.Sprintf("couldn't find the service instance '%s' in namespace '%s'", serviceBinding.Spec.ServiceInstanceName, instanceNamespace)
			utils.SetBlockedCondition(ctx, errMsg, serviceBinding)
			if updateErr := utils.UpdateStatus(ctx, r.Client, serviceBinding); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, instanceErr
		}
	}

	smClient, err := r.GetSMClient(ctx, serviceInstance)
	if err != nil {
		return utils.HandleOperationFailure(ctx, r.Client, serviceBinding, common.Unknown, err)
	}

	if len(serviceBinding.Status.BindingID) > 0 {
		if _, err := smClient.GetBindingByID(serviceBinding.Status.BindingID, nil); err != nil {
			var smError *sm.ServiceManagerError
			if ok := errors.As(err, &smError); ok {
				if smError.StatusCode == http.StatusNotFound {
					condition := metav1.Condition{
						Type:               common.ConditionReady,
						Status:             metav1.ConditionFalse,
						ObservedGeneration: serviceBinding.Generation,
						Reason:             common.ResourceNotFound,
						Message:            fmt.Sprintf("binding %s not found in Service Manager", serviceInstance.Status.InstanceID),
					}
					meta.SetStatusCondition(&serviceBinding.Status.Conditions, condition)
				}
				return ctrl.Result{RequeueAfter: time.Hour * 3}, nil
			}
			log.Error(err, "failed to get binding by id from SM with unknown error")
			return ctrl.Result{}, err
		}
	}

	if utils.IsMarkedForDeletion(serviceBinding.ObjectMeta) {
		return r.delete(ctx, serviceBinding, serviceInstance)
	}

	if controllerutil.AddFinalizer(serviceBinding, common.FinalizerName) {
		log.Info(fmt.Sprintf("added finalizer '%s' to service binding", common.FinalizerName))
		if err := r.Client.Update(ctx, serviceBinding); err != nil {
			return ctrl.Result{}, err
		}
	}

	if len(serviceBinding.Status.OperationURL) > 0 {
		// ongoing operation - poll status from SM
		return r.poll(ctx, serviceBinding, serviceInstance)
	}

	if utils.IsMarkedForDeletion(serviceInstance.ObjectMeta) {
		log.Info(fmt.Sprintf("service instance name: %s namespace: %s is marked for deletion, unable to create binding", serviceInstance.Name, serviceInstance.Namespace))
		utils.SetBlockedCondition(ctx, "instance is in deletion process", serviceBinding)
		return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
	}

	if !serviceInstanceReady(serviceInstance) {
		log.Info(fmt.Sprintf("service instance name: %s namespace: %s is not ready, unable to create binding", serviceInstance.Name, serviceInstance.Namespace))
		utils.SetBlockedCondition(ctx, "service instance is not ready", serviceBinding)
		return ctrl.Result{Requeue: true}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
	}

	// should rotate creds
	if meta.IsStatusConditionTrue(serviceBinding.Status.Conditions, common.ConditionCredRotationInProgress) {
		log.Info("rotating credentials")
		if shouldUpdateStatus, err := r.rotateCredentials(ctx, serviceBinding, serviceInstance); err != nil {
			if !shouldUpdateStatus {
				log.Error(err, "internal error occurred during cred rotation, requeuing binding")
				return ctrl.Result{}, err
			}
			return utils.HandleCredRotationError(ctx, r.Client, serviceBinding, err)
		}
	}

	// is binding ready
	if meta.IsStatusConditionTrue(serviceBinding.Status.Conditions, common.ConditionReady) {
		if isStaleServiceBinding(serviceBinding) {
			log.Info("binding is stale, handling")
			return r.handleStaleServiceBinding(ctx, serviceBinding)
		}

		if initCredRotationIfRequired(serviceBinding) {
			log.Info("cred rotation required, updating status")
			return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
		}

		log.Info("binding in final state, maintaining secret")
		return r.maintain(ctx, serviceBinding, serviceInstance)
	}

	if serviceBinding.Status.BindingID == "" {
		if err := r.validateSecretNameIsAvailable(ctx, serviceBinding); err != nil {
			log.Error(err, "secret validation failed")
			utils.SetBlockedCondition(ctx, err.Error(), serviceBinding)
			return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
		}

		smBinding, err := r.getBindingForRecovery(ctx, smClient, serviceBinding)
		if err != nil {
			log.Error(err, "failed to check binding recovery")
			return utils.HandleServiceManagerError(ctx, r.Client, serviceBinding, smClientTypes.CREATE, err)
		}
		if smBinding != nil {
			return r.recover(ctx, serviceBinding, smBinding)
		}

		return r.createBinding(ctx, smClient, serviceInstance, serviceBinding)
	}

	log.Error(fmt.Errorf("update binding is not allowed, this line should not be reached"), "")
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.ServiceBinding{}).
		WithOptions(controller.Options{RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](r.Config.RetryBaseDelay, r.Config.RetryMaxDelay)}).
		Complete(r)
}

func (r *ServiceBindingReconciler) createBinding(ctx context.Context, smClient sm.Client, serviceInstance *v1.ServiceInstance, serviceBinding *v1.ServiceBinding) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	log.Info("Creating smBinding in SM")
	serviceBinding.Status.InstanceID = serviceInstance.Status.InstanceID
	bindingParameters, _, err := utils.BuildSMRequestParameters(serviceBinding.Namespace, serviceBinding.Spec.Parameters, serviceBinding.Spec.ParametersFrom)
	if err != nil {
		log.Error(err, "failed to parse smBinding parameters")
		return utils.HandleOperationFailure(ctx, r.Client, serviceBinding, smClientTypes.CREATE, err)
	}

	smBinding, operationURL, bindErr := smClient.Bind(&smClientTypes.ServiceBinding{
		Name: serviceBinding.Spec.ExternalName,
		Labels: smClientTypes.Labels{
			common.NamespaceLabel: []string{serviceBinding.Namespace},
			common.K8sNameLabel:   []string{serviceBinding.Name},
			common.ClusterIDLabel: []string{r.Config.ClusterID},
		},
		ServiceInstanceID: serviceInstance.Status.InstanceID,
		Parameters:        bindingParameters,
	}, nil, utils.BuildUserInfo(ctx, serviceBinding.Spec.UserInfo))

	if bindErr != nil {
		log.Error(err, "failed to create service binding", "serviceInstanceID", serviceInstance.Status.InstanceID)
		return utils.HandleServiceManagerError(ctx, r.Client, serviceBinding, smClientTypes.CREATE, bindErr)
	}

	if operationURL != "" {
		var bindingID string
		if bindingID = sm.ExtractBindingID(operationURL); len(bindingID) == 0 {
			return utils.HandleOperationFailure(ctx, r.Client, serviceBinding, smClientTypes.CREATE, fmt.Errorf("failed to extract smBinding ID from operation URL %s", operationURL))
		}
		serviceBinding.Status.BindingID = bindingID

		log.Info("Create smBinding request is async")
		serviceBinding.Status.OperationURL = operationURL
		serviceBinding.Status.OperationType = smClientTypes.CREATE
		utils.SetInProgressConditions(ctx, smClientTypes.CREATE, "", serviceBinding, false)
		if err := utils.UpdateStatus(ctx, r.Client, serviceBinding); err != nil {
			log.Error(err, "unable to update ServiceBinding status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.Config.PollInterval}, nil
	}

	log.Info("Binding created successfully")

	if err := r.storeBindingSecret(ctx, serviceBinding, smBinding); err != nil {
		return r.handleSecretError(ctx, smClientTypes.CREATE, err, serviceBinding)
	}

	subaccountID := ""
	if len(smBinding.Labels["subaccount_id"]) > 0 {
		subaccountID = smBinding.Labels["subaccount_id"][0]
	}

	serviceBinding.Status.BindingID = smBinding.ID
	serviceBinding.Status.SubaccountID = subaccountID
	serviceBinding.Status.Ready = metav1.ConditionTrue
	utils.SetSuccessConditions(smClientTypes.CREATE, serviceBinding, false)
	log.Info("Updating binding", "bindingID", smBinding.ID)

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
}

func (r *ServiceBindingReconciler) delete(ctx context.Context, serviceBinding *v1.ServiceBinding, serviceInstance *v1.ServiceInstance) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	if controllerutil.ContainsFinalizer(serviceBinding, common.FinalizerName) {
		smClient, err := r.GetSMClient(ctx, serviceInstance)
		if err != nil {
			return utils.HandleOperationFailure(ctx, r.Client, serviceBinding, smClientTypes.DELETE, err)
		}

		if len(serviceBinding.Status.BindingID) == 0 {
			log.Info("No binding id found validating binding does not exists in SM before removing finalizer")
			smBinding, err := r.getBindingForRecovery(ctx, smClient, serviceBinding)
			if err != nil {
				return utils.HandleServiceManagerError(ctx, r.Client, serviceBinding, smClientTypes.DELETE, err)
			}
			if smBinding != nil {
				log.Info("binding exists in SM continue with deletion")
				serviceBinding.Status.BindingID = smBinding.ID
				utils.SetInProgressConditions(ctx, smClientTypes.DELETE, "delete after recovery", serviceBinding, false)
				return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
			}

			// make sure there's no secret stored for the binding
			if err := r.deleteBindingSecret(ctx, serviceBinding); err != nil {
				return ctrl.Result{}, err
			}

			log.Info("Binding does not exists in SM, removing finalizer")
			if err := utils.RemoveFinalizer(ctx, r.Client, serviceBinding, common.FinalizerName); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		if len(serviceBinding.Status.OperationURL) > 0 && serviceBinding.Status.OperationType == smClientTypes.DELETE {
			// ongoing delete operation - poll status from SM
			return r.poll(ctx, serviceBinding, serviceInstance)
		}

		log.Info(fmt.Sprintf("Deleting binding with id %v from SM", serviceBinding.Status.BindingID))
		operationURL, unbindErr := smClient.Unbind(serviceBinding.Status.BindingID, nil, utils.BuildUserInfo(ctx, serviceBinding.Spec.UserInfo))
		if unbindErr != nil {
			return utils.HandleServiceManagerError(ctx, r.Client, serviceBinding, smClientTypes.DELETE, unbindErr)
		}

		if operationURL != "" {
			log.Info("Deleting binding async")
			serviceBinding.Status.OperationURL = operationURL
			serviceBinding.Status.OperationType = smClientTypes.DELETE
			utils.SetInProgressConditions(ctx, smClientTypes.DELETE, "", serviceBinding, false)
			if err := utils.UpdateStatus(ctx, r.Client, serviceBinding); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: r.Config.PollInterval}, nil
		}

		log.Info("Binding was deleted successfully")
		return r.deleteSecretAndRemoveFinalizer(ctx, serviceBinding)
	}
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) poll(ctx context.Context, serviceBinding *v1.ServiceBinding, serviceInstance *v1.ServiceInstance) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	log.Info(fmt.Sprintf("resource is in progress, found operation url %s", serviceBinding.Status.OperationURL))

	smClient, err := r.GetSMClient(ctx, serviceInstance)
	if err != nil {
		return utils.HandleOperationFailure(ctx, r.Client, serviceBinding, common.Unknown, err)
	}

	status, statusErr := smClient.Status(serviceBinding.Status.OperationURL, nil)
	if statusErr != nil {
		log.Info(fmt.Sprintf("failed to fetch operation, got error from SM: %s", statusErr.Error()), "operationURL", serviceBinding.Status.OperationURL)
		utils.SetInProgressConditions(ctx, serviceBinding.Status.OperationType, string(smClientTypes.INPROGRESS), serviceBinding, false)
		freshStatus := v1.ServiceBindingStatus{
			Conditions: serviceBinding.GetConditions(),
		}
		if utils.IsMarkedForDeletion(serviceBinding.ObjectMeta) {
			freshStatus.BindingID = serviceBinding.Status.BindingID
		}
		serviceBinding.Status = freshStatus
		if err := utils.UpdateStatus(ctx, r.Client, serviceBinding); err != nil {
			log.Error(err, "failed to update status during polling")
		}
		return ctrl.Result{}, statusErr
	}

	if status == nil {
		return utils.HandleOperationFailure(ctx, r.Client, serviceBinding, serviceBinding.Status.OperationType, fmt.Errorf("failed to get last operation status of %s", serviceBinding.Name))
	}
	switch status.State {
	case smClientTypes.INPROGRESS:
		fallthrough
	case smClientTypes.PENDING:
		if len(status.Description) != 0 {
			utils.SetInProgressConditions(ctx, status.Type, status.Description, serviceBinding, true)
			if err := utils.UpdateStatus(ctx, r.Client, serviceBinding); err != nil {
				log.Error(err, "unable to update ServiceBinding polling description")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: r.Config.PollInterval}, nil
	case smClientTypes.FAILED:
		// if async operation failed we should not retry
		utils.SetFailureConditions(status.Type, status.Description, serviceBinding, true)
		if serviceBinding.Status.OperationType == smClientTypes.DELETE {
			serviceBinding.Status.OperationURL = ""
			serviceBinding.Status.OperationType = ""
			if err := utils.UpdateStatus(ctx, r.Client, serviceBinding); err != nil {
				log.Error(err, "unable to update ServiceBinding status")
				return ctrl.Result{}, err
			}
			errMsg := "Async unbind operation failed"
			if status.Errors != nil {
				errMsg = fmt.Sprintf("Async unbind operation failed, errors: %s", string(status.Errors))
			}
			return ctrl.Result{}, errors.New(errMsg)
		}
	case smClientTypes.SUCCEEDED:
		utils.SetSuccessConditions(status.Type, serviceBinding, true)
		switch serviceBinding.Status.OperationType {
		case smClientTypes.CREATE:
			smBinding, err := smClient.GetBindingByID(serviceBinding.Status.BindingID, nil)
			if err != nil || smBinding == nil {
				log.Error(err, fmt.Sprintf("binding %s succeeded but could not fetch it from SM", serviceBinding.Status.BindingID))
				return ctrl.Result{}, err
			}
			if len(smBinding.Labels["subaccount_id"]) > 0 {
				serviceBinding.Status.SubaccountID = smBinding.Labels["subaccount_id"][0]
			}

			if err := r.storeBindingSecret(ctx, serviceBinding, smBinding); err != nil {
				return r.handleSecretError(ctx, smClientTypes.CREATE, err, serviceBinding)
			}
			utils.SetSuccessConditions(status.Type, serviceBinding, false)
		case smClientTypes.DELETE:
			return r.deleteSecretAndRemoveFinalizer(ctx, serviceBinding)
		}
	}

	log.Info(fmt.Sprintf("finished polling operation %s '%s'", serviceBinding.Status.OperationType, serviceBinding.Status.OperationURL))
	serviceBinding.Status.OperationURL = ""
	serviceBinding.Status.OperationType = ""

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
}

func (r *ServiceBindingReconciler) getBindingForRecovery(ctx context.Context, smClient sm.Client, serviceBinding *v1.ServiceBinding) (*smClientTypes.ServiceBinding, error) {
	log := logutils.GetLogger(ctx)
	nameQuery := fmt.Sprintf("name eq '%s'", serviceBinding.Spec.ExternalName)
	clusterIDQuery := fmt.Sprintf("context/clusterid eq '%s'", r.Config.ClusterID)
	namespaceQuery := fmt.Sprintf("context/namespace eq '%s'", serviceBinding.Namespace)
	k8sNameQuery := fmt.Sprintf("%s eq '%s'", common.K8sNameLabel, serviceBinding.Name)
	parameters := sm.Parameters{
		FieldQuery:    []string{nameQuery, clusterIDQuery, namespaceQuery},
		LabelQuery:    []string{k8sNameQuery},
		GeneralParams: []string{"attach_last_operations=true"},
	}
	log.Info(fmt.Sprintf("binding recovery query params: %s, %s, %s, %s", nameQuery, clusterIDQuery, namespaceQuery, k8sNameQuery))

	bindings, err := smClient.ListBindings(&parameters)
	if err != nil {
		log.Error(err, "failed to list bindings in SM")
		return nil, err
	}
	if bindings != nil {
		log.Info(fmt.Sprintf("found %d bindings", len(bindings.ServiceBindings)))
		if len(bindings.ServiceBindings) == 1 {
			return &bindings.ServiceBindings[0], nil
		}
	}
	return nil, nil
}

func (r *ServiceBindingReconciler) maintain(ctx context.Context, binding *v1.ServiceBinding, instance *v1.ServiceInstance) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	if err := r.maintainSecret(ctx, binding, instance); err != nil {
		log.Error(err, "failed to maintain secret")
		return r.handleSecretError(ctx, smClientTypes.UPDATE, err, binding)
	}

	log.Info("maintain finished successfully")
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) maintainSecret(ctx context.Context, serviceBinding *v1.ServiceBinding, serviceInstance *v1.ServiceInstance) error {
	log := logutils.GetLogger(ctx)
	if common.GetObservedGeneration(serviceBinding) == serviceBinding.Generation {
		log.Info("observed generation is up to date, checking if secret exists")
		if _, err := r.getSecret(ctx, serviceBinding.Namespace, serviceBinding.Spec.SecretName); err == nil {
			log.Info("secret exists, no need to maintain secret")
			return nil
		}

		log.Info("binding's secret was not found")
		r.Recorder.Event(serviceBinding, corev1.EventTypeWarning, "SecretDeleted", "SecretDeleted")
	}

	log.Info("maintaining binding's secret")
	smClient, err := r.GetSMClient(ctx, serviceInstance)
	if err != nil {
		return err
	}
	smBinding, err := smClient.GetBindingByID(serviceBinding.Status.BindingID, nil)
	if err != nil {
		log.Error(err, "failed to get binding for update secret")
		return err
	}
	if smBinding != nil {
		if smBinding.Credentials != nil {
			if err = r.storeBindingSecret(ctx, serviceBinding, smBinding); err != nil {
				return err
			}
			log.Info("Updating binding", "bindingID", smBinding.ID)
			utils.SetSuccessConditions(smClientTypes.UPDATE, serviceBinding, false)
		}
	}

	return utils.UpdateStatus(ctx, r.Client, serviceBinding)
}

func (r *ServiceBindingReconciler) getServiceInstanceForBinding(ctx context.Context, binding *v1.ServiceBinding) (*v1.ServiceInstance, error) {
	log := logutils.GetLogger(ctx)
	serviceInstance := &v1.ServiceInstance{}
	namespace := binding.Namespace
	if len(binding.Spec.ServiceInstanceNamespace) > 0 {
		namespace = binding.Spec.ServiceInstanceNamespace
	}
	log.Info(fmt.Sprintf("getting service instance named %s in namespace %s for binding %s in namespace %s", binding.Spec.ServiceInstanceName, namespace, binding.Name, binding.Namespace))
	if err := r.Client.Get(ctx, types.NamespacedName{Name: binding.Spec.ServiceInstanceName, Namespace: namespace}, serviceInstance); err != nil {
		return serviceInstance, err
	}

	return serviceInstance.DeepCopy(), nil
}

func (r *ServiceBindingReconciler) resyncBindingStatus(ctx context.Context, k8sBinding *v1.ServiceBinding, smBinding *smClientTypes.ServiceBinding) {
	k8sBinding.Status.BindingID = smBinding.ID
	k8sBinding.Status.InstanceID = smBinding.ServiceInstanceID
	k8sBinding.Status.OperationURL = ""
	k8sBinding.Status.OperationType = ""

	bindingStatus := smClientTypes.SUCCEEDED
	operationType := smClientTypes.CREATE
	description := ""
	if smBinding.LastOperation != nil {
		bindingStatus = smBinding.LastOperation.State
		operationType = smBinding.LastOperation.Type
		description = smBinding.LastOperation.Description
	} else if !smBinding.Ready {
		bindingStatus = smClientTypes.FAILED
	}
	switch bindingStatus {
	case smClientTypes.PENDING:
		fallthrough
	case smClientTypes.INPROGRESS:
		k8sBinding.Status.OperationURL = sm.BuildOperationURL(smBinding.LastOperation.ID, smBinding.ID, smClientTypes.ServiceBindingsURL)
		k8sBinding.Status.OperationType = smBinding.LastOperation.Type
		utils.SetInProgressConditions(ctx, smBinding.LastOperation.Type, smBinding.LastOperation.Description, k8sBinding, false)
	case smClientTypes.SUCCEEDED:
		utils.SetSuccessConditions(operationType, k8sBinding, false)
	case smClientTypes.FAILED:
		utils.SetFailureConditions(operationType, description, k8sBinding, false)
	}
}

func (r *ServiceBindingReconciler) storeBindingSecret(ctx context.Context, k8sBinding *v1.ServiceBinding, smBinding *smClientTypes.ServiceBinding) error {
	log := logutils.GetLogger(ctx)
	logger := log.WithValues("bindingName", k8sBinding.Name, "secretName", k8sBinding.Spec.SecretName)

	var secret *corev1.Secret
	var err error

	if k8sBinding.Spec.SecretTemplate != "" {
		secret, err = r.createBindingSecretFromSecretTemplate(ctx, k8sBinding, smBinding)
	} else {
		secret, err = r.createBindingSecret(ctx, k8sBinding, smBinding)
	}

	if err != nil {
		return err
	}
	if err = controllerutil.SetControllerReference(k8sBinding, secret, r.Scheme); err != nil {
		logger.Error(err, "Failed to set secret owner")
		return err
	}

	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[common.ManagedByBTPOperatorLabel] = "true"
	if len(k8sBinding.Labels) > 0 && len(k8sBinding.Labels[common.StaleBindingIDLabel]) > 0 {
		secret.Labels[common.StaleBindingIDLabel] = k8sBinding.Labels[common.StaleBindingIDLabel]
	}

	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations["binding"] = k8sBinding.Name

	return r.createOrUpdateBindingSecret(ctx, k8sBinding, secret)
}

func (r *ServiceBindingReconciler) createBindingSecret(ctx context.Context, k8sBinding *v1.ServiceBinding, smBinding *smClientTypes.ServiceBinding) (*corev1.Secret, error) {
	credentialsMap, err := r.getSecretDefaultData(ctx, k8sBinding, smBinding)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        k8sBinding.Spec.SecretName,
			Annotations: map[string]string{"binding": k8sBinding.Name},
			Labels:      map[string]string{common.ManagedByBTPOperatorLabel: "true"},
			Namespace:   k8sBinding.Namespace,
		},
		Data: credentialsMap,
	}
	return secret, nil
}

func (r *ServiceBindingReconciler) getSecretDefaultData(ctx context.Context, k8sBinding *v1.ServiceBinding, smBinding *smClientTypes.ServiceBinding) (map[string][]byte, error) {
	log := logutils.GetLogger(ctx).WithValues("bindingName", k8sBinding.Name, "secretName", k8sBinding.Spec.SecretName)

	var credentialsMap map[string][]byte
	var credentialProperties []utils.SecretMetadataProperty

	if len(smBinding.Credentials) == 0 {
		log.Info("Binding credentials are empty")
		credentialsMap = make(map[string][]byte)
	} else if k8sBinding.Spec.SecretKey != nil {
		credentialsMap = map[string][]byte{
			*k8sBinding.Spec.SecretKey: smBinding.Credentials,
		}
		credentialProperties = []utils.SecretMetadataProperty{
			{
				Name:      *k8sBinding.Spec.SecretKey,
				Format:    string(utils.JSON),
				Container: true,
			},
		}
	} else {
		var err error
		credentialsMap, credentialProperties, err = utils.NormalizeCredentials(smBinding.Credentials)
		if err != nil {
			log.Error(err, "Failed to store binding secret")
			return nil, fmt.Errorf("failed to create secret. Error: %v", err.Error())
		}
	}

	metaDataProperties, err := r.addInstanceInfo(ctx, k8sBinding, credentialsMap)
	if err != nil {
		log.Error(err, "failed to enrich binding with service instance info")
	}

	if k8sBinding.Spec.SecretRootKey != nil {
		var err error
		credentialsMap, err = singleKeyMap(credentialsMap, *k8sBinding.Spec.SecretRootKey)
		if err != nil {
			return nil, err
		}
	} else {
		metadata := map[string][]utils.SecretMetadataProperty{
			"metaDataProperties":   metaDataProperties,
			"credentialProperties": credentialProperties,
		}
		metadataByte, err := json.Marshal(metadata)
		if err != nil {
			log.Error(err, "failed to enrich binding with metadata")
		} else {
			credentialsMap[".metadata"] = metadataByte
		}
	}
	return credentialsMap, nil
}

func (r *ServiceBindingReconciler) createBindingSecretFromSecretTemplate(ctx context.Context, k8sBinding *v1.ServiceBinding, smBinding *smClientTypes.ServiceBinding) (*corev1.Secret, error) {
	log := logutils.GetLogger(ctx)
	logger := log.WithValues("bindingName", k8sBinding.Name, "secretName", k8sBinding.Spec.SecretName)

	logger.Info("Create Object using SecretTemplate from ServiceBinding Specs")
	inputSmCredentials := smBinding.Credentials
	smBindingCredentials := make(map[string]interface{})
	if inputSmCredentials != nil {
		err := json.Unmarshal(inputSmCredentials, &smBindingCredentials)
		if err != nil {
			logger.Error(err, "failed to unmarshal given service binding credentials")
			return nil, errors.Wrap(err, "failed to unmarshal given service binding credentials")
		}
	}

	instanceInfos, err := r.getInstanceInfo(ctx, k8sBinding)
	if err != nil {
		logger.Error(err, "failed to addInstanceInfo")
		return nil, errors.Wrap(err, "failed to add service instance info")
	}

	parameters := commonutils.GetSecretDataForTemplate(smBindingCredentials, instanceInfos)
	templateName := fmt.Sprintf("%s/%s", k8sBinding.Namespace, k8sBinding.Name)
	secret, err := commonutils.CreateSecretFromTemplate(templateName, k8sBinding.Spec.SecretTemplate, "missingkey=error", parameters)
	if err != nil {
		logger.Error(err, "failed to create secret from template")
		return nil, errors.Wrap(err, "failed to create secret from template")
	}
	secret.SetNamespace(k8sBinding.Namespace)
	secret.SetName(k8sBinding.Spec.SecretName)
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}
	secret.Labels[common.ManagedByBTPOperatorLabel] = "true"

	// if no data provided use the default data
	if len(secret.Data) == 0 && len(secret.StringData) == 0 {
		credentialsMap, err := r.getSecretDefaultData(ctx, k8sBinding, smBinding)
		if err != nil {
			return nil, err
		}
		secret.Data = credentialsMap
	}
	return secret, nil
}

func (r *ServiceBindingReconciler) createOrUpdateBindingSecret(ctx context.Context, binding *v1.ServiceBinding, secret *corev1.Secret) error {
	log := logutils.GetLogger(ctx)
	dbSecret := &corev1.Secret{}
	create := false
	if err := r.Client.Get(ctx, types.NamespacedName{Name: binding.Spec.SecretName, Namespace: binding.Namespace}, dbSecret); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		create = true
	}

	if create {
		log.Info("Creating binding secret", "name", secret.Name)
		if err := r.Client.Create(ctx, secret); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			return nil
		}
		r.Recorder.Event(binding, corev1.EventTypeNormal, "SecretCreated", "SecretCreated")
		return nil
	}

	log.Info("Updating existing binding secret", "name", secret.Name)
	dbSecret.Data = secret.Data
	dbSecret.StringData = secret.StringData
	dbSecret.Labels = secret.Labels
	dbSecret.Annotations = secret.Annotations
	return r.Client.Update(ctx, dbSecret)
}

func (r *ServiceBindingReconciler) deleteBindingSecret(ctx context.Context, binding *v1.ServiceBinding) error {
	log := logutils.GetLogger(ctx)
	log.Info("Deleting binding secret")
	bindingSecret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: binding.Namespace,
		Name:      binding.Spec.SecretName,
	}, bindingSecret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch binding secret")
			return err
		}

		// secret not found, nothing more to do
		log.Info("secret was deleted successfully")
		return nil
	}
	bindingSecret = bindingSecret.DeepCopy()

	if err := r.Client.Delete(ctx, bindingSecret); err != nil {
		log.Error(err, "Failed to delete binding secret")
		return err
	}

	log.Info("secret was deleted successfully")
	return nil
}

func (r *ServiceBindingReconciler) deleteSecretAndRemoveFinalizer(ctx context.Context, serviceBinding *v1.ServiceBinding) (ctrl.Result, error) {
	// delete binding secret if exist
	if err := r.deleteBindingSecret(ctx, serviceBinding); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, utils.RemoveFinalizer(ctx, r.Client, serviceBinding, common.FinalizerName)
}

func (r *ServiceBindingReconciler) getSecret(ctx context.Context, namespace string, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := utils.GetSecretWithFallback(ctx, types.NamespacedName{Namespace: namespace, Name: name}, secret)
	return secret, err
}

func (r *ServiceBindingReconciler) validateSecretNameIsAvailable(ctx context.Context, binding *v1.ServiceBinding) error {
	currentSecret, err := r.getSecret(ctx, binding.Namespace, binding.Spec.SecretName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if metav1.IsControlledBy(currentSecret, binding) {
		return nil
	}

	ownerRef := metav1.GetControllerOf(currentSecret)
	if ownerRef != nil {
		owner, err := schema.ParseGroupVersion(ownerRef.APIVersion)
		if err != nil {
			return err
		}

		if owner.Group == binding.GroupVersionKind().Group && ownerRef.Kind == binding.Kind {
			return fmt.Errorf(secretAlreadyOwnedErrorFormat, binding.Spec.SecretName, ownerRef.Name)
		}
	}

	return fmt.Errorf(secretNameTakenErrorFormat, binding.Spec.SecretName)
}

func (r *ServiceBindingReconciler) handleSecretError(ctx context.Context, op smClientTypes.OperationCategory, err error, binding *v1.ServiceBinding) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	log.Error(err, fmt.Sprintf("failed to store secret %s for binding %s", binding.Spec.SecretName, binding.Name))
	return utils.HandleOperationFailure(ctx, r.Client, binding, op, err)
}

func (r *ServiceBindingReconciler) getInstanceInfo(ctx context.Context, binding *v1.ServiceBinding) (map[string]string, error) {
	instance, err := r.getServiceInstanceForBinding(ctx, binding)
	if err != nil {
		return nil, err
	}
	instanceInfos := make(map[string]string)
	instanceInfos["instance_name"] = string(getInstanceNameForSecretCredentials(instance))
	instanceInfos["instance_guid"] = instance.Status.InstanceID
	instanceInfos["plan"] = instance.Spec.ServicePlanName
	instanceInfos["label"] = instance.Spec.ServiceOfferingName
	instanceInfos["type"] = instance.Spec.ServiceOfferingName
	if len(instance.Status.Tags) > 0 || len(instance.Spec.CustomTags) > 0 {
		tags := mergeInstanceTags(instance.Status.Tags, instance.Spec.CustomTags)
		instanceInfos["tags"] = strings.Join(tags, ",")
	}
	return instanceInfos, nil
}

func (r *ServiceBindingReconciler) addInstanceInfo(ctx context.Context, binding *v1.ServiceBinding, credentialsMap map[string][]byte) ([]utils.SecretMetadataProperty, error) {
	instance, err := r.getServiceInstanceForBinding(ctx, binding)
	if err != nil {
		return nil, err
	}

	credentialsMap["instance_name"] = getInstanceNameForSecretCredentials(instance)
	credentialsMap["instance_guid"] = []byte(instance.Status.InstanceID)
	credentialsMap["plan"] = []byte(instance.Spec.ServicePlanName)
	credentialsMap["label"] = []byte(instance.Spec.ServiceOfferingName)
	credentialsMap["type"] = []byte(instance.Spec.ServiceOfferingName)
	if len(instance.Status.Tags) > 0 || len(instance.Spec.CustomTags) > 0 {
		tagsBytes, err := json.Marshal(mergeInstanceTags(instance.Status.Tags, instance.Spec.CustomTags))
		if err != nil {
			return nil, err
		}
		credentialsMap["tags"] = tagsBytes
	}

	metadata := []utils.SecretMetadataProperty{
		{
			Name:   "instance_name",
			Format: string(utils.TEXT),
		},
		{
			Name:   "instance_guid",
			Format: string(utils.TEXT),
		},
		{
			Name:   "plan",
			Format: string(utils.TEXT),
		},
		{
			Name:   "label",
			Format: string(utils.TEXT),
		},
		{
			Name:   "type",
			Format: string(utils.TEXT),
		},
	}
	if _, ok := credentialsMap["tags"]; ok {
		metadata = append(metadata, utils.SecretMetadataProperty{Name: "tags", Format: string(utils.JSON)})
	}

	return metadata, nil
}

func (r *ServiceBindingReconciler) rotateCredentials(ctx context.Context, binding *v1.ServiceBinding, serviceInstance *v1.ServiceInstance) (bool, error) {
	log := logutils.GetLogger(ctx)
	if err := r.removeForceRotateAnnotationIfNeeded(ctx, binding, log); err != nil {
		log.Info("Credentials rotation - failed to delete force rotate annotation")
		return false, err
	}

	credInProgressCondition := meta.FindStatusCondition(binding.GetConditions(), common.ConditionCredRotationInProgress)
	if credInProgressCondition.Reason == common.CredRotating {
		if len(binding.Status.BindingID) > 0 && binding.Status.Ready == metav1.ConditionTrue {
			log.Info("Credentials rotation - finished successfully")
			now := metav1.NewTime(time.Now())
			binding.Status.LastCredentialsRotationTime = &now
			return false, r.stopRotation(ctx, binding)
		} else if utils.IsFailed(binding) {
			log.Info("Credentials rotation - binding failed stopping rotation")
			return false, r.stopRotation(ctx, binding)
		}
		log.Info("Credentials rotation - waiting to finish")
		return false, nil
	}

	if len(binding.Status.BindingID) == 0 {
		log.Info("Credentials rotation - no binding id found nothing to do")
		return false, r.stopRotation(ctx, binding)
	}

	bindings := &v1.ServiceBindingList{}
	err := r.Client.List(ctx, bindings, client.MatchingLabels{common.StaleBindingIDLabel: binding.Status.BindingID}, client.InNamespace(binding.Namespace))
	if err != nil {
		return false, err
	}

	if len(bindings.Items) == 0 {
		// create the backup binding
		smClient, err := r.GetSMClient(ctx, serviceInstance)
		if err != nil {
			return false, err
		}

		// rename current binding
		suffix := "-" + utils.RandStringRunes(6)
		log.Info("Credentials rotation - renaming binding to old in SM", "current", binding.Spec.ExternalName)
		if _, errRenaming := smClient.RenameBinding(binding.Status.BindingID, binding.Spec.ExternalName+suffix, binding.Name+suffix); errRenaming != nil {
			log.Error(errRenaming, "Credentials rotation - failed renaming binding to old in SM", "binding", binding.Spec.ExternalName)
			return true, errRenaming
		}

		log.Info("Credentials rotation - backing up old binding in K8S", "name", binding.Name+suffix)
		if err := r.createOldBinding(ctx, suffix, binding); err != nil {
			log.Error(err, "Credentials rotation - failed to back up old binding in K8S")
			return true, err
		}
	}

	binding.Status.BindingID = ""
	binding.Status.Ready = metav1.ConditionFalse
	utils.SetInProgressConditions(ctx, smClientTypes.CREATE, "rotating binding credentials", binding, false)
	utils.SetCredRotationInProgressConditions(common.CredRotating, "", binding)
	return false, utils.UpdateStatus(ctx, r.Client, binding)
}

func (r *ServiceBindingReconciler) removeForceRotateAnnotationIfNeeded(ctx context.Context, binding *v1.ServiceBinding, log logr.Logger) error {
	if binding.Annotations != nil {
		if _, ok := binding.Annotations[common.ForceRotateAnnotation]; ok {
			log.Info("Credentials rotation - deleting force rotate annotation")
			delete(binding.Annotations, common.ForceRotateAnnotation)
			return r.Client.Update(ctx, binding)
		}
	}
	return nil
}

func (r *ServiceBindingReconciler) stopRotation(ctx context.Context, binding *v1.ServiceBinding) error {
	conditions := binding.GetConditions()
	meta.RemoveStatusCondition(&conditions, common.ConditionCredRotationInProgress)
	binding.Status.Conditions = conditions
	return utils.UpdateStatus(ctx, r.Client, binding)
}

func (r *ServiceBindingReconciler) createOldBinding(ctx context.Context, suffix string, binding *v1.ServiceBinding) error {
	oldBinding := newBindingObject(binding.Name+suffix, binding.Namespace)
	err := controllerutil.SetControllerReference(binding, oldBinding, r.Scheme)
	if err != nil {
		return err
	}
	oldBinding.Labels = map[string]string{
		common.StaleBindingIDLabel:         binding.Status.BindingID,
		common.StaleBindingRotationOfLabel: truncateString(binding.Name, 63),
	}
	oldBinding.Annotations = map[string]string{
		common.StaleBindingOrigBindingNameAnnotation: binding.Name,
	}
	spec := binding.Spec.DeepCopy()
	spec.CredRotationPolicy.Enabled = false
	spec.SecretName = spec.SecretName + suffix
	spec.ExternalName = spec.ExternalName + suffix
	oldBinding.Spec = *spec
	return r.Client.Create(ctx, oldBinding)
}

func (r *ServiceBindingReconciler) handleStaleServiceBinding(ctx context.Context, serviceBinding *v1.ServiceBinding) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	originalBindingName, ok := serviceBinding.Annotations[common.StaleBindingOrigBindingNameAnnotation]
	if !ok {
		//if the user removed the "OrigBindingName" annotation and rotationOf label not exist as well
		//the stale binding should be deleted otherwise it will remain forever
		if originalBindingName, ok = serviceBinding.Labels[common.StaleBindingRotationOfLabel]; !ok {
			log.Info("missing rotationOf label/annotation, unable to fetch original binding, deleting stale")
			return ctrl.Result{}, r.Client.Delete(ctx, serviceBinding)
		}
	}
	origBinding := &v1.ServiceBinding{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: serviceBinding.Namespace, Name: originalBindingName}, origBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("original binding not found, deleting stale binding")
			return ctrl.Result{}, r.Client.Delete(ctx, serviceBinding)
		}
		return ctrl.Result{}, err
	}
	if meta.IsStatusConditionTrue(origBinding.Status.Conditions, common.ConditionReady) {
		return ctrl.Result{}, r.Client.Delete(ctx, serviceBinding)
	}

	log.Info("not deleting stale binding since original binding is not ready")
	if !meta.IsStatusConditionPresentAndEqual(serviceBinding.Status.Conditions, common.ConditionPendingTermination, metav1.ConditionTrue) {
		pendingTerminationCondition := metav1.Condition{
			Type:               common.ConditionPendingTermination,
			Status:             metav1.ConditionTrue,
			Reason:             common.ConditionPendingTermination,
			Message:            "waiting for new credentials to be ready",
			ObservedGeneration: serviceBinding.GetGeneration(),
		}
		meta.SetStatusCondition(&serviceBinding.Status.Conditions, pendingTerminationCondition)
		return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
	}
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) recover(ctx context.Context, serviceBinding *v1.ServiceBinding, smBinding *smClientTypes.ServiceBinding) (ctrl.Result, error) {
	log := logutils.GetLogger(ctx)
	log.Info(fmt.Sprintf("found existing smBinding in SM with id %s, updating status", smBinding.ID))

	if smBinding.Credentials != nil {
		if err := r.storeBindingSecret(ctx, serviceBinding, smBinding); err != nil {
			operationType := smClientTypes.CREATE
			if smBinding.LastOperation != nil {
				operationType = smBinding.LastOperation.Type
			}
			return r.handleSecretError(ctx, operationType, err, serviceBinding)
		}
	}
	r.resyncBindingStatus(ctx, serviceBinding, smBinding)

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceBinding)
}

func isStaleServiceBinding(binding *v1.ServiceBinding) bool {
	if utils.IsMarkedForDeletion(binding.ObjectMeta) {
		return false
	}

	if binding.Labels != nil {
		if _, ok := binding.Labels[common.StaleBindingIDLabel]; ok {
			if binding.Spec.CredRotationPolicy != nil {
				keepFor, _ := time.ParseDuration(binding.Spec.CredRotationPolicy.RotatedBindingTTL)
				if time.Since(binding.CreationTimestamp.Time) > keepFor {
					return true
				}
			}
		}
	}
	return false
}

func initCredRotationIfRequired(binding *v1.ServiceBinding) bool {
	if utils.IsFailed(binding) || !credRotationEnabled(binding) {
		return false
	}
	_, forceRotate := binding.Annotations[common.ForceRotateAnnotation]

	lastCredentialRotationTime := binding.Status.LastCredentialsRotationTime
	if lastCredentialRotationTime == nil {
		ts := metav1.NewTime(binding.CreationTimestamp.Time)
		lastCredentialRotationTime = &ts
	}

	rotationInterval, _ := time.ParseDuration(binding.Spec.CredRotationPolicy.RotationFrequency)
	if time.Since(lastCredentialRotationTime.Time) > rotationInterval || forceRotate {
		utils.SetCredRotationInProgressConditions(common.CredPreparing, "", binding)
		return true
	}

	return false
}

func credRotationEnabled(binding *v1.ServiceBinding) bool {
	return binding.Spec.CredRotationPolicy != nil && binding.Spec.CredRotationPolicy.Enabled
}

func mergeInstanceTags(offeringTags, customTags []string) []string {
	var tags []string

	for _, tag := range append(offeringTags, customTags...) {
		if !utils.SliceContains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func newBindingObject(name, namespace string) *v1.ServiceBinding {
	return &v1.ServiceBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.GroupVersion.String(),
			Kind:       "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func serviceInstanceReady(instance *v1.ServiceInstance) bool {
	return instance.Status.Ready == metav1.ConditionTrue
}

func getInstanceNameForSecretCredentials(instance *v1.ServiceInstance) []byte {
	if useMetaName, ok := instance.Annotations[common.UseInstanceMetadataNameInSecret]; ok && useMetaName == "true" {
		return []byte(instance.Name)
	}
	return []byte(instance.Spec.ExternalName)
}

func singleKeyMap(credentialsMap map[string][]byte, key string) (map[string][]byte, error) {
	stringCredentialsMap := make(map[string]string)
	for k, v := range credentialsMap {
		stringCredentialsMap[k] = string(v)
	}

	credBytes, err := json.Marshal(stringCredentialsMap)
	if err != nil {
		return nil, err
	}

	return map[string][]byte{
		key: credBytes,
	}, nil
}

func truncateString(str string, length int) string {
	if len(str) > length {
		return str[:length]
	}
	return str
}

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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/utils/ptr"

	"github.com/SAP/sap-btp-service-operator/api/common"
	"github.com/SAP/sap-btp-service-operator/internal/config"
	"github.com/SAP/sap-btp-service-operator/internal/utils"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/google/uuid"

	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceInstanceReconciler reconciles a ServiceInstance object
type ServiceInstanceReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	GetSMClient func(ctx context.Context, serviceInstance *v1.ServiceInstance) (sm.Client, error)
	Config      config.Config
	Recorder    record.EventRecorder
}

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *ServiceInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("serviceinstance", req.NamespacedName).WithValues("correlation_id", uuid.New().String())
	ctx = context.WithValue(ctx, utils.LogKey{}, log)

	serviceInstance := &v1.ServiceInstance{}
	if err := r.Client.Get(ctx, req.NamespacedName, serviceInstance); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch ServiceInstance")
		}
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	serviceInstance = serviceInstance.DeepCopy()

	if len(serviceInstance.GetConditions()) == 0 {
		err := utils.InitConditions(ctx, r.Client, serviceInstance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if isFinalState(ctx, serviceInstance) {
		if len(serviceInstance.Status.HashedSpec) == 0 {
			updateHashedSpecValue(serviceInstance)
			err := r.Client.Status().Update(ctx, serviceInstance)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	if utils.IsMarkedForDeletion(serviceInstance.ObjectMeta) {
		// delete updates the generation
		serviceInstance.SetObservedGeneration(serviceInstance.Generation)
		return r.deleteInstance(ctx, serviceInstance)
	}

	if len(serviceInstance.Status.OperationURL) > 0 {
		// ongoing operation - poll status from SM
		return r.poll(ctx, serviceInstance)
	}

	if !controllerutil.ContainsFinalizer(serviceInstance, common.FinalizerName) {
		controllerutil.AddFinalizer(serviceInstance, common.FinalizerName)
		log.Info(fmt.Sprintf("added finalizer '%s' to service instance", common.FinalizerName))
		if err := r.Client.Update(ctx, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}
	}

	log.Info(fmt.Sprintf("instance is not in final state, handling... (generation: %d, observedGen: %d", serviceInstance.Generation, serviceInstance.Status.ObservedGeneration))
	serviceInstance.SetObservedGeneration(serviceInstance.Generation)

	smClient, err := r.GetSMClient(ctx, serviceInstance)
	if err != nil {
		log.Error(err, "failed to get sm client")
		return utils.MarkAsTransientError(ctx, r.Client, common.Unknown, err, serviceInstance)
	}

	if serviceInstance.Status.InstanceID == "" {
		log.Info("Instance ID is empty, checking if instance exist in SM")
		smInstance, err := r.getInstanceForRecovery(ctx, smClient, serviceInstance)
		if err != nil {
			log.Error(err, "failed to check instance recovery")
			return utils.MarkAsTransientError(ctx, r.Client, common.Unknown, err, serviceInstance)
		}
		if smInstance != nil {
			return r.recover(ctx, smClient, serviceInstance, smInstance)
		}

		// if instance was not recovered then create new instance
		return r.createInstance(ctx, smClient, serviceInstance)
	}

	// Update
	if updateRequired(serviceInstance) {
		if res, err := r.updateInstance(ctx, smClient, serviceInstance); err != nil {
			log.Info("got error while trying to update instance")
			return ctrl.Result{}, err
		} else if res.Requeue {
			return res, nil
		}
	}

	// Handle instance share if needed
	if sharingUpdateRequired(serviceInstance) {
		return r.handleInstanceSharing(ctx, serviceInstance, smClient)
	}

	return ctrl.Result{}, nil
}

func (r *ServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	secretPredicate := SecretPredicate{
		Funcs: predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return utils.IsSecretWatched(e.ObjectNew) && isSecretDataChanged(e)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.ServiceInstance{}).
		WithOptions(controller.Options{RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(r.Config.RetryBaseDelay, r.Config.RetryMaxDelay)}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findRequestsForSecret),
			builder.WithPredicates(secretPredicate),
		).Complete(r)
}

func (r *ServiceInstanceReconciler) createInstance(ctx context.Context, smClient sm.Client, serviceInstance *v1.ServiceInstance) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)
	log.Info("Creating instance in SM")
	updateHashedSpecValue(serviceInstance)
	instanceParameters, err := r.buildSMRequestParameters(ctx, serviceInstance)
	if err != nil {
		// if parameters are invalid there is nothing we can do, the user should fix it according to the error message in the condition
		log.Error(err, "failed to parse instance parameters")
		return utils.MarkAsNonTransientError(ctx, r.Client, smClientTypes.CREATE, err, serviceInstance)
	}

	provision, provisionErr := smClient.Provision(&smClientTypes.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
		Labels: smClientTypes.Labels{
			common.NamespaceLabel: []string{serviceInstance.Namespace},
			common.K8sNameLabel:   []string{serviceInstance.Name},
			common.ClusterIDLabel: []string{r.Config.ClusterID},
		},
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil, utils.BuildUserInfo(ctx, serviceInstance.Spec.UserInfo), serviceInstance.Spec.DataCenter)

	if provisionErr != nil {
		log.Error(provisionErr, "failed to create service instance", "serviceOfferingName", serviceInstance.Spec.ServiceOfferingName,
			"servicePlanName", serviceInstance.Spec.ServicePlanName)
		return utils.HandleError(ctx, r.Client, smClientTypes.CREATE, provisionErr, serviceInstance)
	}

	serviceInstance.Status.InstanceID = provision.InstanceID
	serviceInstance.Status.SubaccountID = provision.SubaccountID
	if len(provision.Tags) > 0 {
		tags, err := getTags(provision.Tags)
		if err != nil {
			log.Error(err, "failed to unmarshal tags")
		} else {
			serviceInstance.Status.Tags = tags
		}
	}

	if provision.Location != "" {
		log.Info("Provision request is in progress (async)")
		serviceInstance.Status.OperationURL = provision.Location
		serviceInstance.Status.OperationType = smClientTypes.CREATE
		utils.SetInProgressConditions(ctx, smClientTypes.CREATE, "", serviceInstance)

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, utils.UpdateStatus(ctx, r.Client, serviceInstance)
	}

	log.Info(fmt.Sprintf("Instance provisioned successfully, instanceID: %s, subaccountID: %s", serviceInstance.Status.InstanceID,
		serviceInstance.Status.SubaccountID))
	utils.SetSuccessConditions(smClientTypes.CREATE, serviceInstance)

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceInstance)
}

func (r *ServiceInstanceReconciler) updateInstance(ctx context.Context, smClient sm.Client, serviceInstance *v1.ServiceInstance) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)
	log.Info(fmt.Sprintf("updating instance %s in SM", serviceInstance.Status.InstanceID))

	updateHashedSpecValue(serviceInstance)

	instanceParameters, err := r.buildSMRequestParameters(ctx, serviceInstance)
	if err != nil {
		log.Error(err, "failed to parse instance parameters")
		return utils.MarkAsNonTransientError(ctx, r.Client, smClientTypes.UPDATE, err, serviceInstance)
	}

	_, operationURL, err := smClient.UpdateInstance(serviceInstance.Status.InstanceID, &smClientTypes.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil, utils.BuildUserInfo(ctx, serviceInstance.Spec.UserInfo), serviceInstance.Spec.DataCenter)

	if err != nil {
		log.Error(err, fmt.Sprintf("failed to update service instance with ID %s", serviceInstance.Status.InstanceID))
		return utils.HandleError(ctx, r.Client, smClientTypes.UPDATE, err, serviceInstance)
	}

	if operationURL != "" {
		log.Info(fmt.Sprintf("Update request accepted, operation URL: %s", operationURL))
		serviceInstance.Status.OperationURL = operationURL
		serviceInstance.Status.OperationType = smClientTypes.UPDATE
		utils.SetInProgressConditions(ctx, smClientTypes.UPDATE, "", serviceInstance)

		if err := utils.UpdateStatus(ctx, r.Client, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance updated successfully")
	utils.SetSuccessConditions(smClientTypes.UPDATE, serviceInstance)

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceInstance)
}

func (r *ServiceInstanceReconciler) deleteInstance(ctx context.Context, serviceInstance *v1.ServiceInstance) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)

	if controllerutil.ContainsFinalizer(serviceInstance, common.FinalizerName) {
		smClient, err := r.GetSMClient(ctx, serviceInstance)
		if err != nil {
			log.Error(err, "failed to get sm client")
			return utils.MarkAsTransientError(ctx, r.Client, common.Unknown, err, serviceInstance)
		}
		if len(serviceInstance.Status.InstanceID) == 0 {
			log.Info("No instance id found validating instance does not exists in SM before removing finalizer")
			smInstance, err := r.getInstanceForRecovery(ctx, smClient, serviceInstance)
			if err != nil {
				return ctrl.Result{}, err
			}
			if smInstance != nil {
				log.Info("instance exists in SM continue with deletion")
				serviceInstance.Status.InstanceID = smInstance.ID
				utils.SetInProgressConditions(ctx, smClientTypes.DELETE, "delete after recovery", serviceInstance)
				return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceInstance)
			}
			log.Info("instance does not exists in SM, removing finalizer")
			return ctrl.Result{}, utils.RemoveFinalizer(ctx, r.Client, serviceInstance, common.FinalizerName)
		}

		if len(serviceInstance.Status.OperationURL) > 0 && serviceInstance.Status.OperationType == smClientTypes.DELETE {
			// ongoing delete operation - poll status from SM
			return r.poll(ctx, serviceInstance)
		}

		log.Info(fmt.Sprintf("Deleting instance with id %v from SM", serviceInstance.Status.InstanceID))
		operationURL, deprovisionErr := smClient.Deprovision(serviceInstance.Status.InstanceID, nil, utils.BuildUserInfo(ctx, serviceInstance.Spec.UserInfo))
		if deprovisionErr != nil {
			// delete will proceed anyway
			return utils.HandleDeleteError(ctx, r.Client, deprovisionErr, serviceInstance)
		}

		if operationURL != "" {
			log.Info("Deleting instance async")
			return r.handleAsyncDelete(ctx, serviceInstance, operationURL)
		}
		for labelKey := range serviceInstance.Labels {
			if strings.HasPrefix(labelKey, common.InstanceSecretLabel) {
				err = utils.RemoveSecretWatch(ctx, r.Client, serviceInstance.Namespace, serviceInstance.Labels[labelKey], serviceInstance.Name)
				if err != nil {
					log.Error(err, fmt.Sprintf("failed to decrease secret watch label with key %s", labelKey))
					return ctrl.Result{}, err
				}
			}
		}
		log.Info("Instance was deleted successfully, removing finalizer")
		// remove our finalizer from the list and update it.
		return ctrl.Result{}, utils.RemoveFinalizer(ctx, r.Client, serviceInstance, common.FinalizerName)
	}
	return ctrl.Result{}, nil
}

func (r *ServiceInstanceReconciler) handleInstanceSharing(ctx context.Context, serviceInstance *v1.ServiceInstance, smClient sm.Client) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)
	log.Info("Handling change in instance sharing")

	if serviceInstance.ShouldBeShared() {
		log.Info("Service instance is shouldBeShared, sharing the instance")
		err := smClient.ShareInstance(serviceInstance.Status.InstanceID, utils.BuildUserInfo(ctx, serviceInstance.Spec.UserInfo))
		if err != nil {
			log.Error(err, "failed to share instance")
			return r.handleInstanceSharingError(ctx, serviceInstance, metav1.ConditionFalse, common.ShareFailed, err)
		}
		log.Info("instance shared successfully")
		setSharedCondition(serviceInstance, metav1.ConditionTrue, common.ShareSucceeded, "instance shared successfully")
	} else { //un-share
		log.Info("Service instance is un-shouldBeShared, un-sharing the instance")
		err := smClient.UnShareInstance(serviceInstance.Status.InstanceID, utils.BuildUserInfo(ctx, serviceInstance.Spec.UserInfo))
		if err != nil {
			log.Error(err, "failed to un-share instance")
			return r.handleInstanceSharingError(ctx, serviceInstance, metav1.ConditionTrue, common.UnShareFailed, err)
		}
		log.Info("instance un-shared successfully")
		if serviceInstance.Spec.Shared != nil {
			setSharedCondition(serviceInstance, metav1.ConditionFalse, common.UnShareSucceeded, "instance un-shared successfully")
		} else {
			log.Info("removing Shared condition since shared is undefined in instance")
			conditions := serviceInstance.GetConditions()
			meta.RemoveStatusCondition(&conditions, common.ConditionShared)
			serviceInstance.SetConditions(conditions)
		}
	}

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceInstance)
}

func (r *ServiceInstanceReconciler) poll(ctx context.Context, serviceInstance *v1.ServiceInstance) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)
	log.Info(fmt.Sprintf("resource is in progress, found operation url %s", serviceInstance.Status.OperationURL))
	smClient, err := r.GetSMClient(ctx, serviceInstance)
	if err != nil {
		log.Error(err, "failed to get sm client")
		return utils.MarkAsTransientError(ctx, r.Client, common.Unknown, err, serviceInstance)
	}

	status, statusErr := smClient.Status(serviceInstance.Status.OperationURL, nil)
	if statusErr != nil {
		log.Info(fmt.Sprintf("failed to fetch operation, got error from SM: %s", statusErr.Error()), "operationURL", serviceInstance.Status.OperationURL)
		utils.SetInProgressConditions(ctx, serviceInstance.Status.OperationType, string(smClientTypes.INPROGRESS), serviceInstance)
		// if failed to read operation status we cleanup the status to trigger re-sync from SM
		freshStatus := v1.ServiceInstanceStatus{Conditions: serviceInstance.GetConditions(), ObservedGeneration: serviceInstance.Generation}
		if utils.IsMarkedForDeletion(serviceInstance.ObjectMeta) {
			freshStatus.InstanceID = serviceInstance.Status.InstanceID
		}
		serviceInstance.Status = freshStatus
		if err := utils.UpdateStatus(ctx, r.Client, serviceInstance); err != nil {
			log.Error(err, "failed to update status during polling")
		}
		return ctrl.Result{}, statusErr
	}

	if status == nil {
		log.Error(fmt.Errorf("last operation is nil"), fmt.Sprintf("polling %s returned nil", serviceInstance.Status.OperationURL))
		return ctrl.Result{}, fmt.Errorf("last operation is nil")
	}
	switch status.State {
	case smClientTypes.INPROGRESS:
		fallthrough
	case smClientTypes.PENDING:
		if len(status.Description) > 0 {
			log.Info(fmt.Sprintf("last operation description is '%s'", status.Description))
			utils.SetInProgressConditions(ctx, status.Type, status.Description, serviceInstance)
			if err := utils.UpdateStatus(ctx, r.Client, serviceInstance); err != nil {
				log.Error(err, "unable to update ServiceInstance polling description")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	case smClientTypes.FAILED:
		errMsg := getErrorMsgFromLastOperation(status)
		utils.SetFailureConditions(status.Type, errMsg, serviceInstance)
		// in order to delete eventually the object we need return with error
		if serviceInstance.Status.OperationType == smClientTypes.DELETE {
			serviceInstance.Status.OperationURL = ""
			serviceInstance.Status.OperationType = ""
			if err := utils.UpdateStatus(ctx, r.Client, serviceInstance); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, fmt.Errorf(errMsg)
		}
	case smClientTypes.SUCCEEDED:
		if serviceInstance.Status.OperationType == smClientTypes.CREATE {
			smInstance, err := smClient.GetInstanceByID(serviceInstance.Status.InstanceID, nil)
			if err != nil {
				log.Error(err, fmt.Sprintf("instance %s succeeded but could not fetch it from SM", serviceInstance.Status.InstanceID))
				return ctrl.Result{}, err
			}
			if len(smInstance.Labels["subaccount_id"]) > 0 {
				serviceInstance.Status.SubaccountID = smInstance.Labels["subaccount_id"][0]
			}
			serviceInstance.Status.Ready = metav1.ConditionTrue
		} else if serviceInstance.Status.OperationType == smClientTypes.DELETE {
			// delete was successful - remove our finalizer from the list and update it.
			if err := utils.RemoveFinalizer(ctx, r.Client, serviceInstance, common.FinalizerName); err != nil {
				return ctrl.Result{}, err
			}
		}
		utils.SetSuccessConditions(status.Type, serviceInstance)
	}

	serviceInstance.Status.OperationURL = ""
	serviceInstance.Status.OperationType = ""

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, serviceInstance)
}

func (r *ServiceInstanceReconciler) handleAsyncDelete(ctx context.Context, serviceInstance *v1.ServiceInstance, opURL string) (ctrl.Result, error) {
	serviceInstance.Status.OperationURL = opURL
	serviceInstance.Status.OperationType = smClientTypes.DELETE
	utils.SetInProgressConditions(ctx, smClientTypes.DELETE, "", serviceInstance)

	if err := utils.UpdateStatus(ctx, r.Client, serviceInstance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
}

func (r *ServiceInstanceReconciler) getInstanceForRecovery(ctx context.Context, smClient sm.Client, serviceInstance *v1.ServiceInstance) (*smClientTypes.ServiceInstance, error) {
	log := utils.GetLogger(ctx)
	parameters := sm.Parameters{
		FieldQuery: []string{
			fmt.Sprintf("name eq '%s'", serviceInstance.Spec.ExternalName),
			fmt.Sprintf("context/clusterid eq '%s'", r.Config.ClusterID),
			fmt.Sprintf("context/namespace eq '%s'", serviceInstance.Namespace)},
		LabelQuery: []string{
			fmt.Sprintf("%s eq '%s'", common.K8sNameLabel, serviceInstance.Name)},
		GeneralParams: []string{"attach_last_operations=true"},
	}

	instances, err := smClient.ListInstances(&parameters)
	if err != nil {
		log.Error(err, "failed to list instances in SM")
		return nil, err
	}

	if instances != nil && len(instances.ServiceInstances) > 0 {
		return &instances.ServiceInstances[0], nil
	}
	log.Info("instance not found in SM")
	return nil, nil
}

func (r *ServiceInstanceReconciler) recover(ctx context.Context, smClient sm.Client, k8sInstance *v1.ServiceInstance, smInstance *smClientTypes.ServiceInstance) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)

	log.Info(fmt.Sprintf("found existing instance in SM with id %s, updating status", smInstance.ID))
	updateHashedSpecValue(k8sInstance)
	// set observed generation to 0 because we dont know which generation the current state in SM represents,
	// unless the generation is 1 and SM is in the same state as operator
	if k8sInstance.Generation == 1 {
		k8sInstance.SetObservedGeneration(1)
	} else {
		k8sInstance.SetObservedGeneration(0)
	}

	if smInstance.Ready {
		k8sInstance.Status.Ready = metav1.ConditionTrue
	}
	if smInstance.Shared {
		setSharedCondition(k8sInstance, metav1.ConditionTrue, common.ShareSucceeded, "Instance shared successfully")
	}
	k8sInstance.Status.InstanceID = smInstance.ID
	k8sInstance.Status.OperationURL = ""
	k8sInstance.Status.OperationType = ""
	tags, err := getOfferingTags(smClient, smInstance.ServicePlanID)
	if err != nil {
		log.Error(err, "could not recover offering tags")
	}
	if len(tags) > 0 {
		k8sInstance.Status.Tags = tags
	}

	instanceState := smClientTypes.SUCCEEDED
	operationType := smClientTypes.CREATE
	description := ""
	if smInstance.LastOperation != nil {
		instanceState = smInstance.LastOperation.State
		operationType = smInstance.LastOperation.Type
		description = smInstance.LastOperation.Description
	} else if !smInstance.Ready {
		instanceState = smClientTypes.FAILED
	}

	switch instanceState {
	case smClientTypes.PENDING:
		fallthrough
	case smClientTypes.INPROGRESS:
		k8sInstance.Status.OperationURL = sm.BuildOperationURL(smInstance.LastOperation.ID, smInstance.ID, smClientTypes.ServiceInstancesURL)
		k8sInstance.Status.OperationType = smInstance.LastOperation.Type
		utils.SetInProgressConditions(ctx, smInstance.LastOperation.Type, smInstance.LastOperation.Description, k8sInstance)
	case smClientTypes.SUCCEEDED:
		utils.SetSuccessConditions(operationType, k8sInstance)
	case smClientTypes.FAILED:
		utils.SetFailureConditions(operationType, description, k8sInstance)
	}

	return ctrl.Result{}, utils.UpdateStatus(ctx, r.Client, k8sInstance)
}

func (r *ServiceInstanceReconciler) handleInstanceSharingError(ctx context.Context, object common.SAPBTPResource, status metav1.ConditionStatus, reason string, err error) (ctrl.Result, error) {
	log := utils.GetLogger(ctx)

	errMsg := err.Error()
	isTransient := false

	if smError, ok := err.(*sm.ServiceManagerError); ok {
		log.Info(fmt.Sprintf("SM returned error with status code %d", smError.StatusCode))
		isTransient = utils.IsTransientError(smError, log)
		errMsg = smError.Error()

		if smError.StatusCode == http.StatusTooManyRequests {
			errMsg = "in progress"
			reason = common.InProgress
		} else if reason == common.ShareFailed &&
			(smError.StatusCode == http.StatusBadRequest || smError.StatusCode == http.StatusInternalServerError) {
			/* non-transient error may occur only when sharing
			   SM return 400 when plan is not sharable
			   SM returns 500 when TOGGLES_ENABLE_INSTANCE_SHARE_FROM_OPERATOR feature toggle is off */
			reason = common.ShareNotSupported
		}
	}

	setSharedCondition(object, status, reason, errMsg)
	return ctrl.Result{Requeue: isTransient}, utils.UpdateStatus(ctx, r.Client, object)
}

func (r *ServiceInstanceReconciler) buildSMRequestParameters(ctx context.Context, serviceInstance *v1.ServiceInstance) ([]byte, error) {
	log := utils.GetLogger(ctx)
	instanceParameters, newSecretsMap, err := utils.BuildSMRequestParameters(serviceInstance.Namespace, serviceInstance.Spec.Parameters, serviceInstance.Spec.ParametersFrom)
	if err != nil {
		log.Error(err, "failed to build instance parameters")
		return nil, err
	}
	shouldUpdate := false
	if serviceInstance.IsSubscribedToSecretKeyRefChange() {
		existingSecrets := make(map[string]string)
		if serviceInstance.Labels == nil {
			serviceInstance.Labels = make(map[string]string)
		} else { // remove old secret labels
			for labelKey := range serviceInstance.Labels {
				if strings.HasPrefix(labelKey, common.InstanceSecretLabel) {
					existingSecrets[labelKey] = "false"
				}
			}
		}
		for key := range newSecretsMap {
			if _, ok := existingSecrets[common.InstanceSecretLabel+"-"+key]; ok {
				// this secret was already on the instance and should stay
				existingSecrets[common.InstanceSecretLabel+"-"+key] = "true"
			} else {
				// this is a new secret on the instance
				shouldUpdate = true
				secret := newSecretsMap[key]
				serviceInstance.Labels[common.InstanceSecretLabel+"-"+key] = secret.Name
				err = utils.AddSecretHaveWatch(ctx, secret, r.Client, serviceInstance.Name)
				if err != nil {
					log.Error(err, fmt.Sprintf("failed to increase secret watch label with key %s", key))
					return nil, err
				}
			}
		}
		for key := range existingSecrets {
			if existingSecrets[key] == "false" {
				// this secret is not on the instance anymore and should be deleted
				shouldUpdate = true
				err = utils.RemoveSecretWatch(ctx, r.Client, serviceInstance.Namespace, serviceInstance.Labels[key], serviceInstance.Name)
				if err != nil {
					log.Error(err, fmt.Sprintf("failed to decrease secret watch label with key %s", key))
					return nil, err
				}
				delete(serviceInstance.Labels, key)
			}
		}
	} else {
		if serviceInstance.Labels != nil {
			// remove all secret labels
			for key := range serviceInstance.Labels {
				if strings.HasPrefix(key, common.InstanceSecretLabel) {
					shouldUpdate = true
					err = utils.RemoveSecretWatch(ctx, r.Client, serviceInstance.Namespace, serviceInstance.Labels[key], serviceInstance.Name)
					if err != nil {
						log.Error(err, fmt.Sprintf("failed to decrease secret watch label with key %s", key))
						return nil, err
					}
					delete(serviceInstance.Labels, key)
				}
			}
		}
	}
	if shouldUpdate {
		err := r.Client.Update(ctx, serviceInstance)
		if err != nil {
			log.Error(err, "failed to Update instance with secret labels")
			return nil, err
		}
	}
	return instanceParameters, err
}

func isFinalState(ctx context.Context, serviceInstance *v1.ServiceInstance) bool {
	log := utils.GetLogger(ctx)
	if utils.IsMarkedForDeletion(serviceInstance.ObjectMeta) {
		log.Info("instance is not in final state, it is marked for deletion")
		return false
	}
	if len(serviceInstance.Status.OperationURL) > 0 {
		log.Info(fmt.Sprintf("instance is not in final state, async operation is in progress (%s)", serviceInstance.Status.OperationURL))
		return false
	}
	if serviceInstance.Generation != serviceInstance.GetObservedGeneration() {
		log.Info(fmt.Sprintf("instance is not in final state, generation: %d, observedGen: %d", serviceInstance.Generation, serviceInstance.GetObservedGeneration()))
		return false
	}

	// succeeded=false for current generation, and without failed=true --> transient error retry
	if utils.IsInProgress(serviceInstance) {
		log.Info("instance is not in final state, sync operation is in progress")
		return false
	}

	if sharingUpdateRequired(serviceInstance) {
		log.Info("instance is not in final state, need to sync sharing status")
		if len(serviceInstance.Status.HashedSpec) == 0 {
			updateHashedSpecValue(serviceInstance)
		}
		return false
	}
	if serviceInstance.Spec.SubscribeToSecretChanges != nil && *serviceInstance.Spec.SubscribeToSecretChanges {
		log.Info("instance is not in final state, SubscribeToSecretChanges is true")
		return false
	}
	log.Info(fmt.Sprintf("instance is in final state (generation: %d)", serviceInstance.Generation))
	return true
}

func updateRequired(serviceInstance *v1.ServiceInstance) bool {
	//update is not supported for failed instances (this can occur when instance creation was asynchronously)
	if serviceInstance.Status.Ready != metav1.ConditionTrue {
		return false
	}

	cond := meta.FindStatusCondition(serviceInstance.Status.Conditions, common.ConditionSucceeded)
	if cond != nil && cond.Reason == common.UpdateInProgress { //in case of transient error occurred
		return true
	}
	if serviceInstance.Spec.SubscribeToSecretChanges != nil && *serviceInstance.Spec.SubscribeToSecretChanges {
		return true
	}

	return getSpecHash(serviceInstance) != serviceInstance.Status.HashedSpec
}

func sharingUpdateRequired(serviceInstance *v1.ServiceInstance) bool {
	//relevant only for non-shared instances - sharing instance is possible only for usable instances
	if serviceInstance.Status.Ready != metav1.ConditionTrue {
		return false
	}

	sharedCondition := meta.FindStatusCondition(serviceInstance.GetConditions(), common.ConditionShared)
	shouldBeShared := serviceInstance.ShouldBeShared()

	if sharedCondition == nil {
		return shouldBeShared
	}

	if sharedCondition.Reason == common.ShareNotSupported {
		return false
	}

	if sharedCondition.Reason == common.InProgress || sharedCondition.Reason == common.ShareFailed || sharedCondition.Reason == common.UnShareFailed {
		return true
	}

	if shouldBeShared {
		return sharedCondition.Status == metav1.ConditionFalse
	}

	return sharedCondition.Status == metav1.ConditionTrue
}

func getOfferingTags(smClient sm.Client, planID string) ([]string, error) {
	planQuery := &sm.Parameters{
		FieldQuery: []string{fmt.Sprintf("id eq '%s'", planID)},
	}
	plans, err := smClient.ListPlans(planQuery)
	if err != nil {
		return nil, err
	}

	if plans == nil || len(plans.ServicePlans) != 1 {
		return nil, fmt.Errorf("could not find plan with id %s", planID)
	}

	offeringQuery := &sm.Parameters{
		FieldQuery: []string{fmt.Sprintf("id eq '%s'", plans.ServicePlans[0].ServiceOfferingID)},
	}

	offerings, err := smClient.ListOfferings(offeringQuery)
	if err != nil {
		return nil, err
	}
	if offerings == nil || len(offerings.ServiceOfferings) != 1 {
		return nil, fmt.Errorf("could not find offering with id %s", plans.ServicePlans[0].ServiceOfferingID)
	}

	var tags []string
	if err := json.Unmarshal(offerings.ServiceOfferings[0].Tags, &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func getTags(tags []byte) ([]string, error) {
	var tagsArr []string
	if err := json.Unmarshal(tags, &tagsArr); err != nil {
		return nil, err
	}
	return tagsArr, nil
}

func getSpecHash(serviceInstance *v1.ServiceInstance) string {
	spec := serviceInstance.Spec
	spec.Shared = ptr.To(false)
	specBytes, _ := json.Marshal(spec)
	s := string(specBytes)
	return generateEncodedMD5Hash(s)
}

func generateEncodedMD5Hash(str string) string {
	hash := md5.Sum([]byte(str))
	return hex.EncodeToString(hash[:])
}

func setSharedCondition(object common.SAPBTPResource, status metav1.ConditionStatus, reason, msg string) {
	conditions := object.GetConditions()
	// align all conditions to latest generation
	for _, cond := range object.GetConditions() {
		if cond.Type != common.ConditionShared {
			cond.ObservedGeneration = object.GetGeneration()
			meta.SetStatusCondition(&conditions, cond)
		}
	}

	shareCondition := metav1.Condition{
		Type:    common.ConditionShared,
		Status:  status,
		Reason:  reason,
		Message: msg,
		// shared condition does not contain observed generation
	}

	// remove shared condition and add it as new (in case it has observed generation)
	meta.RemoveStatusCondition(&conditions, common.ConditionShared)
	meta.SetStatusCondition(&conditions, shareCondition)

	object.SetConditions(conditions)
}

func updateHashedSpecValue(serviceInstance *v1.ServiceInstance) {
	serviceInstance.Status.HashedSpec = getSpecHash(serviceInstance)
}

func getErrorMsgFromLastOperation(status *smClientTypes.Operation) string {
	errMsg := "async operation error"
	if status == nil || len(status.Errors) == 0 {
		return errMsg
	}
	var errMap map[string]interface{}

	if err := json.Unmarshal(status.Errors, &errMap); err != nil {
		return errMsg
	}

	if description, found := errMap["description"]; found {
		if descStr, ok := description.(string); ok {
			errMsg = descStr
		}
	}
	return errMsg
}

type SecretPredicate struct {
	predicate.Funcs
}

func (r *ServiceInstanceReconciler) findRequestsForSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	instancesToUpdate := make([]reconcile.Request, 0)
	var instances v1.ServiceInstanceList
	labelSelector := client.MatchingLabels{common.InstanceSecretLabel + "-" + string(secret.GetUID()): "true"}
	if err := r.Client.List(ctx, &instances, labelSelector); err != nil {
		r.Log.Error(err, "failed to list service instances")
		return nil
	}
	for _, instance := range instances.Items {
		instancesToUpdate = append(instancesToUpdate, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		})
	}
	return instancesToUpdate
}

func isSecretDataChanged(e event.UpdateEvent) bool {
	// Type assert to *v1.Secret
	oldSecret, okOld := e.ObjectOld.(*corev1.Secret)
	newSecret, okNew := e.ObjectNew.(*corev1.Secret)
	if !okOld || !okNew {
		// If the objects are not Secrets, skip the event
		return false
	}

	// Compare the Data field (byte slices)
	return !reflect.DeepEqual(oldSecret.Data, newSecret.Data)
}

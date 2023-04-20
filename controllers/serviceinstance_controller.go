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
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"

	hash "github.com/mitchellh/hashstructure"

	"github.com/SAP/sap-btp-service-operator/api"

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
	*BaseReconciler
}

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *ServiceInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("serviceinstance", req.NamespacedName).WithValues("correlation_id", uuid.New().String())
	ctx = context.WithValue(ctx, LogKey{}, log)

	serviceInstance := &servicesv1.ServiceInstance{}
	if err := r.Get(ctx, req.NamespacedName, serviceInstance); err != nil {
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
		err := r.init(ctx, serviceInstance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	smClient, err := r.getSMClient(ctx, serviceInstance)
	if err != nil {
		return r.markAsTransientError(ctx, Unknown, err, serviceInstance)
	}

	if len(serviceInstance.Status.OperationURL) > 0 {
		// ongoing operation - poll status from SM
		return r.poll(ctx, smClient, serviceInstance)
	}

	if isDelete(serviceInstance.ObjectMeta) {
		return r.deleteInstance(ctx, smClient, serviceInstance)
	}

	if !controllerutil.ContainsFinalizer(serviceInstance, api.FinalizerName) {
		controllerutil.AddFinalizer(serviceInstance, api.FinalizerName)
		log.Info(fmt.Sprintf("added finalizer '%s' to service instance", api.FinalizerName))
		if err := r.Update(ctx, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}
	}

	if isFinalState(serviceInstance) {
		log.Info(fmt.Sprintf("Final state, spec did not change, and we are not in progress - ignoring... Generation is - %v", serviceInstance.Generation))
		return ctrl.Result{}, nil
	}

	log.Info(fmt.Sprintf("Current generation is %v and observed is %v", serviceInstance.Generation, serviceInstance.Status.ObservedGeneration))
	serviceInstance.SetObservedGeneration(serviceInstance.Generation)

	if serviceInstance.Status.InstanceID == "" {
		// Recovery
		log.Info("Instance ID is empty, checking if instance exist in SM")
		instance, err := r.getInstanceForRecovery(ctx, smClient, serviceInstance)
		if err != nil {
			log.Error(err, "failed to check instance recovery")
			return r.markAsTransientError(ctx, Unknown, err, serviceInstance)
		}
		if instance != nil {
			log.Info(fmt.Sprintf("found existing instance in SM with id %s, updating status", instance.ID))
			r.resyncInstanceStatus(ctx, smClient, serviceInstance, instance)
			return ctrl.Result{}, r.updateStatus(ctx, serviceInstance)
		}

		// if instance was not recovered then create new instance
		return r.createInstance(ctx, smClient, serviceInstance)
	}

	// Update
	if needsToUpdate(serviceInstance) {
		res, err := r.updateInstance(ctx, smClient, serviceInstance)
		if err != nil {
			log.Info("got error while trying to update instance")
			return res, err
		}
	}

	// Handle instance share change if needed
	if instanceShareChangeNeedsToBeHandled(serviceInstance) {
		return r.handleInstanceSharingChange(ctx, serviceInstance, smClient)
	}

	return ctrl.Result{}, nil
}

func getSpecHash(serviceInstance *servicesv1.ServiceInstance) uint64 {
	spec := serviceInstance.Spec
	spec.Shared = pointer.BoolPtr(false)
	hash, _ := hash.Hash(spec, nil)
	return hash
}

func needsToUpdate(serviceInstance *servicesv1.ServiceInstance) bool {
	if serviceInstance.Status.Ready != metav1.ConditionTrue {
		return false
	}

	if getSpecHash(serviceInstance) == serviceInstance.Status.Signature {
		return false
	}
	return true
}

func instanceShareChangeNeedsToBeHandled(serviceInstance *servicesv1.ServiceInstance) bool {
	return servicesv1.ShouldHandleSharing(serviceInstance) &&
		serviceInstance.Status.Ready == metav1.ConditionTrue
}

func isFinalState(serviceInstance *servicesv1.ServiceInstance) bool {

	for _, cond := range serviceInstance.GetConditions() {
		if cond.ObservedGeneration != serviceInstance.Generation {
			return false
		}
	}

	if isInProgress(serviceInstance) {
		return false
	}
	if servicesv1.ShouldHandleSharing(serviceInstance) {
		return false
	}
	return true
}

func (r *ServiceInstanceReconciler) handleInstanceSharingChange(ctx context.Context, serviceInstance *servicesv1.ServiceInstance, smClient sm.Client) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info("Handling change in instance sharing")

	shouldBeShared := serviceInstance.Spec.Shared != nil && *serviceInstance.Spec.Shared

	if shouldBeShared {
		log.Info("Service instance is shouldBeShared, sharing the instance")
		err := smClient.ShareInstance(serviceInstance.Status.InstanceID, buildUserInfo(ctx, serviceInstance.Spec.UserInfo))
		if err != nil {
			log.Error(err, "failed to share instance")
			setSharedCondition(serviceInstance, metav1.ConditionFalse, ShareFail, err.Error())
			if err := r.updateStatus(ctx, serviceInstance); err != nil {
				log.Info("got error while trying to update status")
				return ctrl.Result{}, err
			}
			return r.handleError(ctx, smClientTypes.SHARE, err, serviceInstance)
		}
		log.Info("instance shared successfully")
		setSharedCondition(serviceInstance, metav1.ConditionTrue, ShareSucceeded, "instance shared successfully")
	} else { //un-share
		log.Info("Service instance is un-shouldBeShared, un-sharing the instance")
		err := smClient.UnShareInstance(serviceInstance.Status.InstanceID, buildUserInfo(ctx, serviceInstance.Spec.UserInfo))
		if err != nil {
			log.Error(err, "failed to un-share instance")
			setSharedCondition(serviceInstance, metav1.ConditionTrue, UnShareFail, err.Error())
			if err := r.updateStatus(ctx, serviceInstance); err != nil {
				log.Info("got error while trying to update status")
				return ctrl.Result{}, err
			}
			return r.handleError(ctx, smClientTypes.UNSHARE, err, serviceInstance)
		}
		log.Info("instance un-shared successfully")
		setSharedCondition(serviceInstance, metav1.ConditionFalse, UnShareSucceeded, "instance un-shared successfully")
	}

	return ctrl.Result{}, r.updateStatus(ctx, serviceInstance)
}

func (r *ServiceInstanceReconciler) poll(ctx context.Context, smClient sm.Client, serviceInstance *servicesv1.ServiceInstance) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("resource is in progress, found operation url %s", serviceInstance.Status.OperationURL))
	status, statusErr := smClient.Status(serviceInstance.Status.OperationURL, nil)
	if statusErr != nil {
		log.Info(fmt.Sprintf("failed to fetch operation, got error from SM: %s", statusErr.Error()), "operationURL", serviceInstance.Status.OperationURL)
		setInProgressConditions(serviceInstance.Status.OperationType, statusErr.Error(), serviceInstance)
		// if failed to read operation status we cleanup the status to trigger re-sync from SM
		freshStatus := servicesv1.ServiceInstanceStatus{Conditions: serviceInstance.GetConditions()}
		if isDelete(serviceInstance.ObjectMeta) {
			freshStatus.InstanceID = serviceInstance.Status.InstanceID
		}
		serviceInstance.Status = freshStatus
		if err := r.updateStatus(ctx, serviceInstance); err != nil {
			log.Error(err, "failed to update status during polling")
		}
		return ctrl.Result{}, statusErr
	}

	switch status.State {
	case smClientTypes.INPROGRESS:
		fallthrough
	case smClientTypes.PENDING:
		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	case smClientTypes.FAILED:
		setFailureConditions(smClientTypes.OperationCategory(status.Type), status.Description, serviceInstance)
		// in order to delete eventually the object we need return with error
		if serviceInstance.Status.OperationType == smClientTypes.DELETE {
			serviceInstance.Status.OperationURL = ""
			serviceInstance.Status.OperationType = ""
			if err := r.updateStatus(ctx, serviceInstance); err != nil {
				return ctrl.Result{}, err
			}
			errMsg := "async deprovisioning operation failed"
			if status.Errors != nil {
				errMsg = fmt.Sprintf("%s. Errors: %s", errMsg, string(status.Errors))
			}
			return ctrl.Result{}, fmt.Errorf(errMsg)
		}
	case smClientTypes.SUCCEEDED:
		updateSignatureHash(serviceInstance)
		setSuccessConditions(status.Type, serviceInstance)
		if serviceInstance.Status.OperationType == smClientTypes.DELETE {
			// delete was successful - remove our finalizer from the list and update it.
			if err := r.removeFinalizer(ctx, serviceInstance, api.FinalizerName); err != nil {
				return ctrl.Result{}, err
			}
		} else if serviceInstance.Status.OperationType == smClientTypes.CREATE {
			serviceInstance.Status.Ready = metav1.ConditionTrue
			updateSignatureHash(serviceInstance)
			setSuccessConditions(status.Type, serviceInstance)
		}
	}

	serviceInstance.Status.OperationURL = ""
	serviceInstance.Status.OperationType = ""

	return ctrl.Result{}, r.updateStatus(ctx, serviceInstance)
}

func (r *ServiceInstanceReconciler) createInstance(ctx context.Context, smClient sm.Client, serviceInstance *servicesv1.ServiceInstance) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info("Creating instance in SM")
	_, instanceParameters, err := buildParameters(r.Client, serviceInstance.Namespace, serviceInstance.Spec.ParametersFrom, serviceInstance.Spec.Parameters)
	if err != nil {
		// if parameters are invalid there is nothing we can do, the user should fix it according to the error message in the condition
		log.Error(err, "failed to parse instance parameters")
		return r.markAsNonTransientError(ctx, smClientTypes.CREATE, err, serviceInstance)
	}

	provision, provisionErr := smClient.Provision(&smClientTypes.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
		Labels: smClientTypes.Labels{
			namespaceLabel: []string{serviceInstance.Namespace},
			k8sNameLabel:   []string{serviceInstance.Name},
			clusterIDLabel: []string{r.Config.ClusterID},
		},
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil, buildUserInfo(ctx, serviceInstance.Spec.UserInfo), serviceInstance.Spec.DataCenter)

	if provisionErr != nil {
		log.Error(provisionErr, "failed to create service instance", "serviceOfferingName", serviceInstance.Spec.ServiceOfferingName,
			"servicePlanName", serviceInstance.Spec.ServicePlanName)
		if isTransientError(ctx, provisionErr) {
			return r.markAsTransientError(ctx, smClientTypes.CREATE, provisionErr, serviceInstance)
		}
		return r.markAsNonTransientError(ctx, smClientTypes.CREATE, provisionErr, serviceInstance)
	}

	if serviceInstance.Spec.Shared != nil && *serviceInstance.Spec.Shared {
		setConditionSharingNeedsToBeDone(serviceInstance)
	}

	if provision.Location != "" {
		serviceInstance.Status.InstanceID = provision.InstanceID
		if len(provision.Tags) > 0 {
			tags, err := getTags(provision.Tags)
			if err != nil {
				log.Error(err, "failed to unmarshal tags")
			} else {
				serviceInstance.Status.Tags = tags
			}
		}

		log.Info("Provision request is in progress")
		serviceInstance.Status.OperationURL = provision.Location
		serviceInstance.Status.OperationType = smClientTypes.CREATE
		setInProgressConditions(smClientTypes.CREATE, "", serviceInstance)

		if err := r.updateStatus(ctx, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance provisioned successfully")
	serviceInstance.Status.InstanceID = provision.InstanceID

	if len(provision.Tags) > 0 {
		tags, err := getTags(provision.Tags)
		if err != nil {
			log.Error(err, "failed to unmarshal tags")
		} else {
			serviceInstance.Status.Tags = tags
		}
	}

	serviceInstance.Status.Ready = metav1.ConditionTrue
	setSuccessConditions(smClientTypes.CREATE, serviceInstance)
	updateSignatureHash(serviceInstance)
	return ctrl.Result{}, r.updateStatus(ctx, serviceInstance)
}

func setConditionForSharedTrue(instance *servicesv1.ServiceInstance) {
	conditions := instance.GetConditions()

	shareCondition := metav1.Condition{
		Type:               api.ConditionShared,
		Status:             metav1.ConditionTrue,
		Reason:             getConditionReason(smClientTypes.UPDATE, smClientTypes.SUCCEEDED),
		Message:            "Instance is shared",
		ObservedGeneration: instance.GetGeneration(),
	}
	updateNewConditionAndRemovePrevious(conditions, instance, shareCondition)
}

func setConditionSharingNeedsToBeDone(object api.SAPBTPResource) {
	conditions := object.GetConditions()

	shareCondition := metav1.Condition{
		Type:               api.ConditionShared,
		Status:             metav1.ConditionFalse,
		Reason:             getConditionReason(smClientTypes.UPDATE, smClientTypes.PENDING),
		Message:            "Sharing of instance needs to be performed",
		ObservedGeneration: object.GetGeneration(),
	}
	updateNewConditionAndRemovePrevious(conditions, object, shareCondition)
}

func setSharedCondition(object api.SAPBTPResource, status metav1.ConditionStatus, reason, msg string) {
	conditions := object.GetConditions()
	shareCondition := metav1.Condition{
		Type:               api.ConditionShared,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: object.GetGeneration(),
	}
	updateNewConditionAndRemovePrevious(conditions, object, shareCondition)
}

func updateNewConditionAndRemovePrevious(conditions []metav1.Condition, object api.SAPBTPResource, shareCondition metav1.Condition) {
	meta.RemoveStatusCondition(&conditions, api.ConditionShared)
	meta.SetStatusCondition(&conditions, shareCondition)
	object.SetConditions(conditions)
}

func getTags(tags []byte) ([]string, error) {
	var tagsArr []string
	if err := json.Unmarshal(tags, &tagsArr); err != nil {
		return nil, err
	}
	return tagsArr, nil
}

func (r *ServiceInstanceReconciler) updateInstance(ctx context.Context, smClient sm.Client, serviceInstance *servicesv1.ServiceInstance) (ctrl.Result, error) {
	var err error
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("updating instance %s in SM", serviceInstance.Status.InstanceID))

	_, instanceParameters, err := buildParameters(r.Client, serviceInstance.Namespace, serviceInstance.Spec.ParametersFrom, serviceInstance.Spec.Parameters)
	if err != nil {
		log.Error(err, "failed to parse instance parameters")
		return r.markAsNonTransientError(ctx, smClientTypes.UPDATE, fmt.Errorf("failed to parse parameters: %v", err.Error()), serviceInstance)
	}

	_, operationURL, err := smClient.UpdateInstance(serviceInstance.Status.InstanceID, &smClientTypes.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil, buildUserInfo(ctx, serviceInstance.Spec.UserInfo), serviceInstance.Spec.DataCenter)

	if err != nil {
		log.Error(err, fmt.Sprintf("failed to update service instance with ID %s", serviceInstance.Status.InstanceID))
		if isTransientError(ctx, err) {
			return r.markAsTransientError(ctx, smClientTypes.UPDATE, err, serviceInstance)
		}
		return r.markAsNonTransientError(ctx, smClientTypes.UPDATE, err, serviceInstance)
	}

	if operationURL != "" {
		log.Info(fmt.Sprintf("Update request accepted, operation URL: %s", operationURL))
		serviceInstance.Status.OperationURL = operationURL
		serviceInstance.Status.OperationType = smClientTypes.UPDATE
		setInProgressConditions(smClientTypes.UPDATE, "", serviceInstance)

		if err := r.updateStatus(ctx, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance updated successfully")
	setSuccessConditions(smClientTypes.UPDATE, serviceInstance)
	updateSignatureHash(serviceInstance)
	return ctrl.Result{}, r.updateStatus(ctx, serviceInstance)
}

func updateSignatureHash(serviceInstance *servicesv1.ServiceInstance) {
	serviceInstance.Status.Signature = getSpecHash(serviceInstance)
}

func (r *ServiceInstanceReconciler) deleteInstance(ctx context.Context, smClient sm.Client, serviceInstance *servicesv1.ServiceInstance) (ctrl.Result, error) {
	log := GetLogger(ctx)
	if controllerutil.ContainsFinalizer(serviceInstance, api.FinalizerName) {
		if len(serviceInstance.Status.InstanceID) == 0 {
			log.Info("No instance id found validating instance does not exists in SM before removing finalizer")

			smInstance, err := r.getInstanceForRecovery(ctx, smClient, serviceInstance)
			if err != nil {
				return ctrl.Result{}, err
			}
			if smInstance != nil {
				log.Info("instance exists in SM continue with deletion")
				serviceInstance.Status.InstanceID = smInstance.ID
				setInProgressConditions(smClientTypes.DELETE, "delete after recovery", serviceInstance)
				return ctrl.Result{}, r.updateStatus(ctx, serviceInstance)
			}
			log.Info("instance does not exists in SM, removing finalizer")
			return ctrl.Result{}, r.removeFinalizer(ctx, serviceInstance, api.FinalizerName)
		}

		log.Info(fmt.Sprintf("Deleting instance with id %v from SM", serviceInstance.Status.InstanceID))
		operationURL, deprovisionErr := smClient.Deprovision(serviceInstance.Status.InstanceID, nil, buildUserInfo(ctx, serviceInstance.Spec.UserInfo))
		if deprovisionErr != nil {
			// delete will proceed anyway
			return r.markAsNonTransientError(ctx, smClientTypes.DELETE, deprovisionErr, serviceInstance)
		}

		if operationURL != "" {
			log.Info("Deleting instance async")
			serviceInstance.Status.OperationURL = operationURL
			serviceInstance.Status.OperationType = smClientTypes.DELETE
			setInProgressConditions(smClientTypes.DELETE, "", serviceInstance)

			if err := r.updateStatus(ctx, serviceInstance); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
		}
		log.Info("Instance was deleted successfully")
		serviceInstance.Status.InstanceID = ""
		setSuccessConditions(smClientTypes.DELETE, serviceInstance)
		if err := r.updateStatus(ctx, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}

		// remove our finalizer from the list and update it.
		if err := r.removeFinalizer(ctx, serviceInstance, api.FinalizerName); err != nil {
			return ctrl.Result{}, err
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil

	}
	return ctrl.Result{}, nil
}

func (r *ServiceInstanceReconciler) resyncInstanceStatus(ctx context.Context, smClient sm.Client, k8sInstance *servicesv1.ServiceInstance, smInstance *smClientTypes.ServiceInstance) {
	log := GetLogger(ctx)
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
		setConditionForSharedTrue(k8sInstance)
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
		setInProgressConditions(smInstance.LastOperation.Type, smInstance.LastOperation.Description, k8sInstance)
	case smClientTypes.SUCCEEDED:
		setSuccessConditions(operationType, k8sInstance)
		updateSignatureHash(k8sInstance)
	case smClientTypes.FAILED:
		setFailureConditions(operationType, description, k8sInstance)
	}
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

func (r *ServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicesv1.ServiceInstance{}).
		WithOptions(controller.Options{RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(r.Config.RetryBaseDelay, r.Config.RetryMaxDelay)}).
		Complete(r)
}

func (r *ServiceInstanceReconciler) getInstanceForRecovery(ctx context.Context, smClient sm.Client, serviceInstance *servicesv1.ServiceInstance) (*smClientTypes.ServiceInstance, error) {
	log := GetLogger(ctx)
	parameters := sm.Parameters{
		FieldQuery: []string{
			fmt.Sprintf("name eq '%s'", serviceInstance.Spec.ExternalName),
			fmt.Sprintf("context/clusterid eq '%s'", r.Config.ClusterID),
			fmt.Sprintf("context/namespace eq '%s'", serviceInstance.Namespace)},
		LabelQuery: []string{
			fmt.Sprintf("%s eq '%s'", k8sNameLabel, serviceInstance.Name)},
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

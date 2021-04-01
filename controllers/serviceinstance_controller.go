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
	"fmt"

	smTypes "github.com/Peripli/service-manager/pkg/types"
	"github.com/Peripli/service-manager/pkg/web"
	"github.com/SAP/sap-btp-service-operator/api/v1alpha1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/go-logr/logr"
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
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *ServiceInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("serviceinstance", req.NamespacedName)

	serviceInstance := &v1alpha1.ServiceInstance{}
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
		err := r.init(ctx, log, serviceInstance)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	smClient, err := r.getSMClient(ctx, log, serviceInstance)
	if err != nil {
		return r.markAsTransientError(ctx, smTypes.CREATE, err, serviceInstance, log)
	}

	if len(serviceInstance.Status.OperationURL) > 0 {
		// ongoing operation - poll status from SM
		return r.poll(ctx, smClient, serviceInstance, log)
	}

	if isDelete(serviceInstance.ObjectMeta) {
		return r.deleteInstance(ctx, smClient, serviceInstance, log)
	}

	if serviceInstance.Generation == serviceInstance.Status.ObservedGeneration && !isInProgress(serviceInstance) {
		log.Info(fmt.Sprintf("Spec is not changed - ignoring... Generation is - %v", serviceInstance.Generation))
		return ctrl.Result{}, nil
	}

	log.Info(fmt.Sprintf("Current generation is %v and observed is %v", serviceInstance.Generation, serviceInstance.Status.ObservedGeneration))
	serviceInstance.SetObservedGeneration(serviceInstance.Generation)

	if serviceInstance.Status.InstanceID == "" {
		//Recovery
		log.Info("Instance ID is empty, checking if instance exist in SM")
		instance, err := r.getInstanceForRecovery(smClient, serviceInstance, log)
		if err != nil {
			log.Error(err, "failed to check instance recovery")
			return r.markAsTransientError(ctx, smTypes.CREATE, err, serviceInstance, log)
		}
		if instance != nil {
			log.Info(fmt.Sprintf("found existing instance in SM with id %s, updating status", instance.ID))
			r.resyncInstanceStatus(serviceInstance, instance)
			return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
		}

		//if instance was not recovered then create new instance
		return r.createInstance(ctx, smClient, serviceInstance, log)
	}

	//Update
	if serviceInstance.Status.Ready == metav1.ConditionTrue {
		return r.updateInstance(ctx, smClient, serviceInstance, log)
	}
	return ctrl.Result{}, nil
}

func (r *ServiceInstanceReconciler) poll(ctx context.Context, smClient sm.Client, serviceInstance *v1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	log.Info(fmt.Sprintf("resource is in progress, found operation url %s", serviceInstance.Status.OperationURL))
	status, statusErr := smClient.Status(serviceInstance.Status.OperationURL, nil)
	if statusErr != nil {
		log.Info(fmt.Sprintf("failed to fetch operation, got error from SM: %s", statusErr.Error()), "operationURL", serviceInstance.Status.OperationURL)
		setInProgressConditions(serviceInstance.Status.OperationType, statusErr.Error(), serviceInstance)
		// if failed to read operation status we cleanup the status to trigger re-sync from SM
		freshStatus := v1alpha1.ServiceInstanceStatus{Conditions: serviceInstance.GetConditions()}
		if isDelete(serviceInstance.ObjectMeta) {
			freshStatus.InstanceID = serviceInstance.Status.InstanceID
		}
		serviceInstance.Status = freshStatus
		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			log.Error(err, "failed to update status during polling")
		}
		return ctrl.Result{}, statusErr
	}

	switch status.State {
	case string(smTypes.IN_PROGRESS):
		fallthrough
	case string(smTypes.PENDING):
		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	case string(smTypes.FAILED):
		setFailureConditions(smTypes.OperationCategory(status.Type), status.Description, serviceInstance)
		// in order to delete eventually the object we need return with error
		if serviceInstance.Status.OperationType == smTypes.DELETE {
			serviceInstance.Status.OperationURL = ""
			serviceInstance.Status.OperationType = ""
			if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
				return ctrl.Result{}, err
			}
			errMsg := "async deprovisioning operation failed"
			if status.Errors != nil {
				errMsg = fmt.Sprintf("%s. Errors: %s", errMsg, string(status.Errors))
			}
			return ctrl.Result{}, fmt.Errorf(errMsg)
		}
	case string(smTypes.SUCCEEDED):
		setSuccessConditions(smTypes.OperationCategory(status.Type), serviceInstance)
		if serviceInstance.Status.OperationType == smTypes.DELETE {
			// delete was successful - remove our finalizer from the list and update it.
			if err := r.removeFinalizer(ctx, serviceInstance, v1alpha1.FinalizerName, log); err != nil {
				return ctrl.Result{}, err
			}
		} else if serviceInstance.Status.OperationType == smTypes.CREATE {
			serviceInstance.Status.Ready = metav1.ConditionTrue
			setSuccessConditions(smTypes.OperationCategory(status.Type), serviceInstance)
		}
	}

	serviceInstance.Status.OperationURL = ""
	serviceInstance.Status.OperationType = ""

	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) createInstance(ctx context.Context, smClient sm.Client, serviceInstance *v1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	log.Info("Creating instance in SM")
	_, instanceParameters, err := buildParameters(r.Client, serviceInstance.Namespace, serviceInstance.Spec.ParametersFrom, serviceInstance.Spec.Parameters)
	if err != nil {
		//if parameters are invalid there is nothing we can do, the user should fix it according to the error message in the condition
		log.Error(err, "failed to parse instance parameters")
		return r.markAsNonTransientError(ctx, smTypes.CREATE, err, serviceInstance, log)
	}

	smInstanceID, operationURL, provisionErr := smClient.Provision(&types.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
		Labels: smTypes.Labels{
			namespaceLabel: []string{serviceInstance.Namespace},
			k8sNameLabel:   []string{serviceInstance.Name},
			clusterIDLabel: []string{r.Config.ClusterID},
		},
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil, buildUserInfo(serviceInstance.Spec.UserInfo, log))

	if provisionErr != nil {
		log.Error(provisionErr, "failed to create service instance", "serviceOfferingName", serviceInstance.Spec.ServiceOfferingName,
			"servicePlanName", serviceInstance.Spec.ServicePlanName)
		if isTransientError(provisionErr, log) {
			return r.markAsTransientError(ctx, smTypes.CREATE, provisionErr, serviceInstance, log)
		}
		return r.markAsNonTransientError(ctx, smTypes.CREATE, provisionErr, serviceInstance, log)
	}

	if operationURL != "" {
		serviceInstance.Status.InstanceID = smInstanceID
		log.Info("Provision request is in progress")
		serviceInstance.Status.OperationURL = operationURL
		serviceInstance.Status.OperationType = smTypes.CREATE
		setInProgressConditions(smTypes.CREATE, "", serviceInstance)
		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance provisioned successfully")
	serviceInstance.Status.InstanceID = smInstanceID
	serviceInstance.Status.Ready = metav1.ConditionTrue
	setSuccessConditions(smTypes.CREATE, serviceInstance)
	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) updateInstance(ctx context.Context, smClient sm.Client, serviceInstance *v1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	var err error

	log.Info(fmt.Sprintf("updating instance %s in SM", serviceInstance.Status.InstanceID))
	_, instanceParameters, err := buildParameters(r.Client, serviceInstance.Namespace, serviceInstance.Spec.ParametersFrom, serviceInstance.Spec.Parameters)
	if err != nil {
		log.Error(err, "failed to parse instance parameters")
		return r.markAsNonTransientError(ctx, smTypes.UPDATE, fmt.Errorf("failed to parse parameters: %v", err.Error()), serviceInstance, log)
	}

	_, operationURL, err := smClient.UpdateInstance(serviceInstance.Status.InstanceID, &types.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil, buildUserInfo(serviceInstance.Spec.UserInfo, log))
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to update service instance with ID %s", serviceInstance.Status.InstanceID))
		if isTransientError(err, log) {
			return r.markAsTransientError(ctx, smTypes.UPDATE, err, serviceInstance, log)
		}
		return r.markAsNonTransientError(ctx, smTypes.UPDATE, err, serviceInstance, log)
	}

	if operationURL != "" {
		log.Info(fmt.Sprintf("Update request accepted, operation URL: %s", operationURL))
		serviceInstance.Status.OperationURL = operationURL
		serviceInstance.Status.OperationType = smTypes.UPDATE
		setInProgressConditions(smTypes.UPDATE, "", serviceInstance)

		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance updated successfully")
	setSuccessConditions(smTypes.UPDATE, serviceInstance)
	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) deleteInstance(ctx context.Context, smClient sm.Client, serviceInstance *v1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(serviceInstance, v1alpha1.FinalizerName) {
		if len(serviceInstance.Status.InstanceID) == 0 {
			log.Info("No instance id found validating instance does not exists in SM before removing finalizer")

			smInstance, err := r.getInstanceForRecovery(smClient, serviceInstance, log)
			if err != nil {
				return ctrl.Result{}, err
			}
			if smInstance != nil {
				log.Info("instance exists in SM continue with deletion")
				serviceInstance.Status.InstanceID = smInstance.ID
				setInProgressConditions(smTypes.DELETE, "delete after recovery", serviceInstance)
				return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
			}
			log.Info("instance does not exists in SM, removing finalizer")
			return ctrl.Result{}, r.removeFinalizer(ctx, serviceInstance, v1alpha1.FinalizerName, log)
		}

		log.Info(fmt.Sprintf("Deleting instance with id %v from SM", serviceInstance.Status.InstanceID))
		operationURL, deprovisionErr := smClient.Deprovision(serviceInstance.Status.InstanceID, nil, buildUserInfo(serviceInstance.Spec.UserInfo, log))
		if deprovisionErr != nil {
			// delete will proceed anyway
			return r.markAsNonTransientError(ctx, smTypes.DELETE, deprovisionErr, serviceInstance, log)
		}

		if operationURL != "" {
			log.Info("Deleting instance async")
			serviceInstance.Status.OperationURL = operationURL
			serviceInstance.Status.OperationType = smTypes.DELETE
			setInProgressConditions(smTypes.DELETE, "", serviceInstance)

			if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
		}
		log.Info("Instance was deleted successfully")
		serviceInstance.Status.InstanceID = ""
		setSuccessConditions(smTypes.DELETE, serviceInstance)
		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			return ctrl.Result{}, err
		}

		// remove our finalizer from the list and update it.
		if err := r.removeFinalizer(ctx, serviceInstance, v1alpha1.FinalizerName, log); err != nil {
			return ctrl.Result{}, err
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil

	}
	return ctrl.Result{}, nil
}

func (r *ServiceInstanceReconciler) resyncInstanceStatus(k8sInstance *v1alpha1.ServiceInstance, smInstance *types.ServiceInstance) {
	//set observed generation to 0 because we dont know which generation the current state in SM represents,
	//unless the generation is 1 and SM is in the same state as operator
	if k8sInstance.Generation == 1 {
		k8sInstance.SetObservedGeneration(1)
	} else {
		k8sInstance.SetObservedGeneration(0)
	}

	if smInstance.Ready {
		k8sInstance.Status.Ready = metav1.ConditionTrue
	}
	k8sInstance.Status.InstanceID = smInstance.ID
	k8sInstance.Status.OperationURL = ""
	k8sInstance.Status.OperationType = ""
	switch smInstance.LastOperation.State {
	case smTypes.PENDING:
		fallthrough
	case smTypes.IN_PROGRESS:
		k8sInstance.Status.OperationURL = sm.BuildOperationURL(smInstance.LastOperation.ID, smInstance.ID, web.ServiceInstancesURL)
		k8sInstance.Status.OperationType = smInstance.LastOperation.Type
		setInProgressConditions(smInstance.LastOperation.Type, smInstance.LastOperation.Description, k8sInstance)
	case smTypes.SUCCEEDED:
		setSuccessConditions(smInstance.LastOperation.Type, k8sInstance)
	case smTypes.FAILED:
		setFailureConditions(smInstance.LastOperation.Type, smInstance.LastOperation.Description, k8sInstance)
	}
}

func (r *ServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ServiceInstance{}).
		Complete(r)
}

func (r *ServiceInstanceReconciler) getInstanceForRecovery(smClient sm.Client, serviceInstance *v1alpha1.ServiceInstance, log logr.Logger) (*types.ServiceInstance, error) {
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

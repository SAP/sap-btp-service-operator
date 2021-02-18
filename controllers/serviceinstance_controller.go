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

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	smTypes "github.com/Peripli/service-manager/pkg/types"
	"github.com/Peripli/service-manager/pkg/web"
	servicesv1alpha1 "github.com/SAP/sap-btp-service-operator/api/v1alpha1"
	"github.com/SAP/sap-btp-service-operator/internal/smclient"
	"github.com/SAP/sap-btp-service-operator/internal/smclient/types"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const instanceFinalizerName string = "storage.finalizers.peripli.io.service-manager.serviceInstance"

// ServiceInstanceReconciler reconciles a ServiceInstance object
type ServiceInstanceReconciler struct {
	*BaseReconciler
}

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *ServiceInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("serviceinstance", req.NamespacedName)

	if r.Config.Suspend {
		log.Info("operator is suspended")
		return ctrl.Result{}, nil
	}

	serviceInstance := &servicesv1alpha1.ServiceInstance{}
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

	smClient, err := r.getSMClient(ctx, log, serviceInstance)
	if err != nil {
		setFailureConditions(smTypes.CREATE, err.Error(), serviceInstance)
		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if len(serviceInstance.Status.OperationURL) > 0 {
		// ongoing operation - poll status from SM
		return r.poll(ctx, smClient, serviceInstance, log)
	}

	if isDelete(serviceInstance.ObjectMeta) {
		return r.deleteInstance(ctx, smClient, serviceInstance, log)
	}
	// The object is not being deleted, so if it does not have our finalizer,
	// then lets init it
	if !controllerutil.ContainsFinalizer(serviceInstance, instanceFinalizerName) {
		if err := r.init(ctx, instanceFinalizerName, log, serviceInstance); err != nil {
			return ctrl.Result{}, err
		}
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
			setFailureConditions(smTypes.CREATE, err.Error(), serviceInstance)
			if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: r.Config.SyncPeriod}, nil
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
	return r.updateInstance(ctx, smClient, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) poll(ctx context.Context, smClient smclient.Client, serviceInstance *servicesv1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	log.Info(fmt.Sprintf("resource is in progress, found operation url %s", serviceInstance.Status.OperationURL))
	status, statusErr := smClient.Status(serviceInstance.Status.OperationURL, nil)
	if statusErr != nil {
		log.Info(fmt.Sprintf("failed to fetch operation, got error from SM: %s", statusErr.Error()), "operationURL", serviceInstance.Status.OperationURL)
		setFailureConditions(serviceInstance.Status.OperationType, statusErr.Error(), serviceInstance)
		// if failed to read operation status we cleanup the status to trigger re-sync from SM
		freshStatus := servicesv1alpha1.ServiceInstanceStatus{Conditions: serviceInstance.GetConditions()}
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
			errMsg := "Async deprovision operation failed"
			if status.Errors != nil {
				errMsg = fmt.Sprintf("Async deprovision operation failed, errors: %s", string(status.Errors))
			}
			return ctrl.Result{}, fmt.Errorf(errMsg)
		}
	case string(smTypes.SUCCEEDED):
		setSuccessConditions(smTypes.OperationCategory(status.Type), serviceInstance)
		if serviceInstance.Status.OperationType == smTypes.DELETE {
			// delete was successful - remove our finalizer from the list and update it.
			if err := r.removeFinalizer(ctx, serviceInstance, instanceFinalizerName, log); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	serviceInstance.Status.OperationURL = ""
	serviceInstance.Status.OperationType = ""

	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) createInstance(ctx context.Context, smClient smclient.Client, serviceInstance *servicesv1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	log.Info("Creating instance in SM")
	instanceParameters, err := getParameters(serviceInstance)
	if err != nil {
		//if parameters are invalid there is nothing we can do, the user should fix it according to the error message in the condition
		log.Error(err, "failed to parse instance parameters")
		setFailureConditions(smTypes.CREATE, fmt.Sprintf("failed to parse instance parameters: %s", err.Error()), serviceInstance)
		return ctrl.Result{}, nil
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
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil)

	if provisionErr != nil {
		log.Error(provisionErr, "failed to create service instance", "serviceOfferingName", serviceInstance.Spec.ServiceOfferingName,
			"servicePlanName", serviceInstance.Spec.ServicePlanName)
		if isTransientError(provisionErr, log) {
			return r.markAsTransientError(ctx, smTypes.CREATE, provisionErr.Error(), serviceInstance, log)
		}
		return r.markAsNonTransientError(ctx, smTypes.CREATE, provisionErr.Error(), serviceInstance, log)
	}

	if operationURL != "" {
		serviceInstance.Status.InstanceID = smInstanceID
		log.Info("Provision request is in progress")
		serviceInstance.Status.OperationURL = operationURL
		serviceInstance.Status.OperationType = smTypes.CREATE
		setInProgressCondition(smTypes.CREATE, "", serviceInstance)
		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance provisioned successfully")
	setSuccessConditions(smTypes.CREATE, serviceInstance)
	serviceInstance.Status.InstanceID = smInstanceID

	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) updateInstance(ctx context.Context, smClient smclient.Client, serviceInstance *servicesv1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	var err error

	log.Info(fmt.Sprintf("updating instance %s in SM", serviceInstance.Status.InstanceID))
	instanceParameters, err := getParameters(serviceInstance)
	if err != nil {
		log.Error(err, "failed to parse instance parameters")
		return r.markAsNonTransientError(ctx, smTypes.UPDATE, fmt.Sprintf("failed to parse parameters: %v", err.Error()), serviceInstance, log)
	}

	_, operationURL, err := smClient.UpdateInstance(serviceInstance.Status.InstanceID, &types.ServiceInstance{
		Name:          serviceInstance.Spec.ExternalName,
		ServicePlanID: serviceInstance.Spec.ServicePlanID,
		Parameters:    instanceParameters,
	}, serviceInstance.Spec.ServiceOfferingName, serviceInstance.Spec.ServicePlanName, nil)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to update service instance with ID %s", serviceInstance.Status.InstanceID))
		if isTransientError(err, log) {
			return r.markAsTransientError(ctx, smTypes.UPDATE, err.Error(), serviceInstance, log)
		}
		return r.markAsNonTransientError(ctx, smTypes.UPDATE, err.Error(), serviceInstance, log)
	}

	if operationURL != "" {
		log.Info(fmt.Sprintf("Update request accepted, operation URL: %s", operationURL))
		serviceInstance.Status.OperationURL = operationURL
		serviceInstance.Status.OperationType = smTypes.UPDATE
		setInProgressCondition(smTypes.UPDATE, "", serviceInstance)

		if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}
	log.Info("Instance updated successfully")
	setSuccessConditions(smTypes.UPDATE, serviceInstance)
	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
}

func (r *ServiceInstanceReconciler) deleteInstance(ctx context.Context, smClient smclient.Client, serviceInstance *servicesv1alpha1.ServiceInstance, log logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(serviceInstance, instanceFinalizerName) {
		if len(serviceInstance.Status.InstanceID) == 0 {
			log.Info("No instance id found validating instance does not exists in SM before removing finalizer")

			smInstance, err := r.getInstanceForRecovery(smClient, serviceInstance, log)
			if err != nil {
				return ctrl.Result{}, err
			}
			if smInstance != nil {
				log.Info("instance exists in SM continue with deletion")
				serviceInstance.Status.InstanceID = smInstance.ID
				setInProgressCondition(smTypes.DELETE, "delete after recovery", serviceInstance)
				return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceInstance, log)
			}
			log.Info("instance does not exists in SM, removing finalizer")
			return ctrl.Result{}, r.removeFinalizer(ctx, serviceInstance, instanceFinalizerName, log)
		}

		log.Info(fmt.Sprintf("Deleting instance with id %v from SM", serviceInstance.Status.InstanceID))
		operationURL, deprovisionErr := smClient.Deprovision(serviceInstance.Status.InstanceID, nil)
		if deprovisionErr != nil {
			if isTransientError(deprovisionErr, log) {
				return r.markAsTransientError(ctx, smTypes.DELETE, deprovisionErr.Error(), serviceInstance, log)
			}

			setFailureConditions(smTypes.DELETE, fmt.Sprintf("failed to delete instance %s: %s", serviceInstance.Status.InstanceID, deprovisionErr.Error()), serviceInstance)
			if err := r.updateStatusWithRetries(ctx, serviceInstance, log); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, deprovisionErr
		}

		if operationURL != "" {
			log.Info("Deleting instance async")
			serviceInstance.Status.OperationURL = operationURL
			serviceInstance.Status.OperationType = smTypes.DELETE
			setInProgressCondition(smTypes.DELETE, "", serviceInstance)

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
		if err := r.removeFinalizer(ctx, serviceInstance, instanceFinalizerName, log); err != nil {
			return ctrl.Result{}, err
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil

	}
	return ctrl.Result{}, nil
}

func (r *ServiceInstanceReconciler) resyncInstanceStatus(k8sInstance *servicesv1alpha1.ServiceInstance, smInstance *types.ServiceInstance) {
	//set observed generation to 0 because we dont know which generation the current state in SM represents,
	//unless the generation is 1 and SM is in the same state as operator
	if k8sInstance.Generation == 1 {
		k8sInstance.SetObservedGeneration(1)
	} else {
		k8sInstance.SetObservedGeneration(0)
	}

	k8sInstance.Status.InstanceID = smInstance.ID
	k8sInstance.Status.OperationURL = ""
	k8sInstance.Status.OperationType = ""
	switch smInstance.LastOperation.State {
	case smTypes.PENDING:
		fallthrough
	case smTypes.IN_PROGRESS:
		k8sInstance.Status.OperationURL = smclient.BuildOperationURL(smInstance.LastOperation.ID, smInstance.ID, web.ServiceInstancesURL)
		k8sInstance.Status.OperationType = smInstance.LastOperation.Type
		setInProgressCondition(smInstance.LastOperation.Type, smInstance.LastOperation.Description, k8sInstance)
	case smTypes.SUCCEEDED:
		setSuccessConditions(smInstance.LastOperation.Type, k8sInstance)
	case smTypes.FAILED:
		setFailureConditions(smInstance.LastOperation.Type, smInstance.LastOperation.Description, k8sInstance)
	}
}

func (r *ServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicesv1alpha1.ServiceInstance{}).
		Complete(r)
}

func (r *ServiceInstanceReconciler) getInstanceForRecovery(smClient smclient.Client, serviceInstance *servicesv1alpha1.ServiceInstance, log logr.Logger) (*types.ServiceInstance, error) {
	parameters := smclient.Parameters{
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

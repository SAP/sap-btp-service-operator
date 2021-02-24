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

	"github.com/SAP/sap-btp-service-operator/client/sm"

	smTypes "github.com/Peripli/service-manager/pkg/types"
	"github.com/Peripli/service-manager/pkg/web"
	"github.com/SAP/sap-btp-service-operator/api/v1alpha1"
	smclientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const bindingFinalizerName string = "services.cloud.sap.com/binding-finalizer"

// ServiceBindingReconciler reconciles a ServiceBinding object
type ServiceBindingReconciler struct {
	*BaseReconciler
}

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=servicebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances,verbs=get;list
// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=serviceinstances/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *ServiceBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO optimize log - use withValue where possible
	log := r.Log.WithValues("servicebinding", req.NamespacedName)

	serviceBinding := &v1alpha1.ServiceBinding{}
	if err := r.Get(ctx, req.NamespacedName, serviceBinding); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch ServiceBinding")
		}
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	serviceBinding = serviceBinding.DeepCopy()

	smClient, err := r.getSMClient(ctx, log, serviceBinding)
	if err != nil {
		setFailureConditions(smTypes.CREATE, err.Error(), serviceBinding)
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	if len(serviceBinding.Status.OperationURL) > 0 {
		// ongoing operation - poll status from SM
		return r.poll(ctx, smClient, serviceBinding, log)
	}

	if isDelete(serviceBinding.ObjectMeta) {
		return r.delete(ctx, smClient, serviceBinding, log)
	}
	// The object is not being deleted, so if it does not have our finalizer,
	// then lets init it
	if !controllerutil.ContainsFinalizer(serviceBinding, bindingFinalizerName) {
		if err := r.init(ctx, bindingFinalizerName, log, serviceBinding); err != nil {
			return ctrl.Result{}, err
		}
	}

	if serviceBinding.GetObservedGeneration() > 0 && !isInProgress(serviceBinding) {
		log.Info("Binding already created")
		return ctrl.Result{}, nil
	}

	log.Info(fmt.Sprintf("Current generation is %v and observed is %v", serviceBinding.Generation, serviceBinding.GetObservedGeneration()))
	serviceBinding.SetObservedGeneration(serviceBinding.Generation)

	log.Info("service instance name " + serviceBinding.Spec.ServiceInstanceName + " binding namespace " + serviceBinding.Namespace)
	serviceInstance, err := r.getServiceInstanceForBinding(ctx, serviceBinding)
	if err != nil || serviceNotUsable(serviceInstance) {
		var instanceErr error
		if err != nil {
			instanceErr = fmt.Errorf("couldn't find the service instance '%s'. Error: %v", serviceBinding.Spec.ServiceInstanceName, err.Error())
		} else {
			instanceErr = fmt.Errorf("service instance '%s' is not usable", serviceBinding.Spec.ServiceInstanceName)
		}

		setBlockedCondition(instanceErr.Error(), serviceBinding)
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, instanceErr
	}

	if isInProgress(serviceInstance) {
		log.Info(fmt.Sprintf("Service instance with k8s name %s is not ready for binding yet", serviceInstance.Name))

		setInProgressCondition(smTypes.CREATE, fmt.Sprintf("creation in progress, waiting for service instance '%s' to be ready", serviceBinding.Spec.ServiceInstanceName),
			serviceBinding)
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}

	if serviceBinding.Status.BindingID == "" {
		err := r.validateSecretNameIsAvailable(ctx, serviceBinding)
		if err != nil {
			setBlockedCondition(err.Error(), serviceBinding)
			return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceBinding, log)
		}

		binding, err := r.getBindingForRecovery(smClient, serviceBinding, log)
		if binding == nil {
			log.Info("getBindingForRecovery returned nil")
		}
		if err != nil {
			log.Error(err, "failed to list bindings from SM")
		}
		if binding != nil {
			// Recovery - restore binding from SM
			log.Info(fmt.Sprintf("found existing smBinding in SM with id %s, updating status", binding.ID))
			if err := r.SetOwner(ctx, serviceInstance, serviceBinding, log); err != nil {
				return ctrl.Result{}, err
			}

			if binding.LastOperation.Type != smTypes.CREATE || binding.LastOperation.State == smTypes.SUCCEEDED {
				// store secret unless binding is still being created or failed during creation
				if err := r.storeBindingSecret(ctx, serviceBinding, binding, log); err != nil {
					setFailureConditions(binding.LastOperation.Type, err.Error(), serviceBinding)
					if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, err
				}
			}
			r.resyncBindingStatus(serviceBinding, binding, serviceInstance.Status.InstanceID)

			return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceBinding, log)
		}
		return r.createBinding(ctx, smClient, serviceInstance, serviceBinding, log)
	}

	log.Error(fmt.Errorf("update binding is not allowed, this line should not be reached"), "")
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) createBinding(ctx context.Context, smClient sm.Client, serviceInstance *v1alpha1.ServiceInstance, serviceBinding *v1alpha1.ServiceBinding, log logr.Logger) (ctrl.Result, error) {
	log.Info("Creating smBinding in SM")
	serviceBinding.Status.InstanceID = serviceInstance.Status.InstanceID
	bindingParameters, err := getParameters(serviceBinding)
	if err != nil {
		log.Error(err, "failed to parse smBinding parameters")
		return r.markAsNonTransientError(ctx, smTypes.CREATE, err.Error(), serviceBinding, log)
	}

	if err := r.SetOwner(ctx, serviceInstance, serviceBinding, log); err != nil {
		setFailureConditions(smTypes.CREATE, err.Error(), serviceBinding)
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	smBinding, operationURL, bindErr := smClient.Bind(&smclientTypes.ServiceBinding{
		Name: serviceBinding.Spec.ExternalName,
		Labels: smTypes.Labels{
			namespaceLabel: []string{serviceInstance.Namespace},
			k8sNameLabel:   []string{serviceBinding.Name},
			clusterIDLabel: []string{r.Config.ClusterID},
		},
		ServiceInstanceID: serviceInstance.Status.InstanceID,
		Parameters:        bindingParameters,
	}, nil)

	if bindErr != nil {
		log.Error(err, "failed to create service binding", "serviceInstanceID", serviceInstance.Status.InstanceID)
		if isTransientError(bindErr, log) {
			return r.markAsTransientError(ctx, smTypes.CREATE, bindErr.Error(), serviceBinding, log)
		}
		return r.markAsNonTransientError(ctx, smTypes.CREATE, bindErr.Error(), serviceBinding, log)
	}

	if operationURL != "" {
		var bindingID string
		if bindingID = sm.ExtractBindingID(operationURL); len(bindingID) == 0 {
			return r.markAsNonTransientError(ctx, smTypes.CREATE, fmt.Sprintf("failed to extract smBinding ID from operation URL %s", operationURL), serviceBinding, log)
		}
		serviceBinding.Status.BindingID = bindingID

		log.Info("Create smBinding request is async")
		serviceBinding.Status.OperationURL = operationURL
		serviceBinding.Status.OperationType = smTypes.CREATE
		setInProgressCondition(smTypes.CREATE, "", serviceBinding)
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			log.Error(err, "unable to update ServiceBinding status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	}

	log.Info("Binding created successfully")

	if err := r.storeBindingSecret(ctx, serviceBinding, smBinding, log); err != nil {
		log.Error(err, "failed to create secret")
		setFailureConditions(smTypes.CREATE, err.Error(), serviceBinding)
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			log.Error(err, "unable to update ServiceBinding status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	setSuccessConditions(smTypes.CREATE, serviceBinding)
	serviceBinding.Status.BindingID = smBinding.ID
	log.Info("Updating binding", "bindingID", smBinding.ID)

	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceBinding, log)
}

func (r *ServiceBindingReconciler) delete(ctx context.Context, smClient sm.Client, serviceBinding *v1alpha1.ServiceBinding, log logr.Logger) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(serviceBinding, bindingFinalizerName) {
		if len(serviceBinding.Status.BindingID) == 0 {
			log.Info("No binding id found validating binding does not exists in SM before removing finalizer")
			smBinding, err := r.getBindingForRecovery(smClient, serviceBinding, log)
			if err != nil {
				return ctrl.Result{}, err
			}
			if smBinding != nil {
				log.Info("binding exists in SM continue with deletion")
				serviceBinding.Status.BindingID = smBinding.ID
				setInProgressCondition(smTypes.DELETE, "delete after recovery", serviceBinding)
				return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceBinding, log)
			}

			// make sure there's no secret stored for the binding
			if err := r.deleteBindingSecret(ctx, serviceBinding, log); err != nil {
				return ctrl.Result{}, err
			}

			log.Info("Binding does not exists in SM, removing finalizer")
			if err := r.removeFinalizer(ctx, serviceBinding, bindingFinalizerName, log); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}

		log.Info(fmt.Sprintf("Deleting binding with id %v from SM", serviceBinding.Status.BindingID))
		operationURL, unbindErr := smClient.Unbind(serviceBinding.Status.BindingID, nil)
		if unbindErr != nil {
			log.Error(unbindErr, "failed to delete binding")
			if isTransientError(unbindErr, log) {
				return r.markAsTransientError(ctx, smTypes.DELETE, unbindErr.Error(), serviceBinding, log)
			}
			// if fail to delete the binding in SM, return with error
			// so that it can be retried
			setFailureConditions(smTypes.DELETE, unbindErr.Error(), serviceBinding)
			if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, unbindErr
		}

		if operationURL != "" {
			log.Info("Deleting binding async")
			serviceBinding.Status.OperationURL = operationURL
			serviceBinding.Status.OperationType = smTypes.DELETE
			setInProgressCondition(smTypes.DELETE, "", serviceBinding)
			if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
		}

		log.Info("Binding was deleted successfully")
		return r.removeBindingFromKubernetes(ctx, serviceBinding, log)
	}
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) poll(ctx context.Context, smClient sm.Client, serviceBinding *v1alpha1.ServiceBinding, log logr.Logger) (ctrl.Result, error) {
	log.Info(fmt.Sprintf("resource is in progress, found operation url %s", serviceBinding.Status.OperationURL))
	status, statusErr := smClient.Status(serviceBinding.Status.OperationURL, nil)
	if statusErr != nil {
		log.Info(fmt.Sprintf("failed to fetch operation, got error from SM: %s", statusErr.Error()), "operationURL", serviceBinding.Status.OperationURL)
		setFailureConditions(serviceBinding.Status.OperationType, statusErr.Error(), serviceBinding)
		freshStatus := v1alpha1.ServiceBindingStatus{
			Conditions: serviceBinding.GetConditions(),
		}
		if isDelete(serviceBinding.ObjectMeta) {
			freshStatus.BindingID = serviceBinding.Status.BindingID
		}
		serviceBinding.Status = freshStatus
		if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
			log.Error(err, "failed to update status during polling")
		}
		return ctrl.Result{}, statusErr
	}

	if status == nil {
		setFailureConditions(serviceBinding.Status.OperationType, "failed to get last operation status", serviceBinding)
		return ctrl.Result{Requeue: true}, r.updateStatusWithRetries(ctx, serviceBinding, log)
	}
	switch status.State {
	case string(smTypes.IN_PROGRESS):
		fallthrough
	case string(smTypes.PENDING):
		return ctrl.Result{Requeue: true, RequeueAfter: r.Config.PollInterval}, nil
	case string(smTypes.FAILED):
		//non transient error - should not retry
		setFailureConditions(smTypes.OperationCategory(status.Type), status.Description, serviceBinding)
		if serviceBinding.Status.OperationType == smTypes.DELETE {
			serviceBinding.Status.OperationURL = ""
			serviceBinding.Status.OperationType = ""
			if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
				log.Error(err, "unable to update ServiceBinding status")
				return ctrl.Result{}, err
			}
			errMsg := "Async unbind operation failed"
			if status.Errors != nil {
				errMsg = fmt.Sprintf("Async unbind operation failed, errors: %s", string(status.Errors))
			}
			return ctrl.Result{}, fmt.Errorf(errMsg)
		}
	case string(smTypes.SUCCEEDED):
		setSuccessConditions(smTypes.OperationCategory(status.Type), serviceBinding)
		switch serviceBinding.Status.OperationType {
		case smTypes.CREATE:
			smBinding, err := smClient.GetBindingByID(serviceBinding.Status.BindingID, nil)
			if err != nil {
				log.Error(err, fmt.Sprintf("binding %s succeeded but could not fetch it from SM", serviceBinding.Status.BindingID))
				return ctrl.Result{}, err
			}

			if err := r.storeBindingSecret(ctx, serviceBinding, smBinding, log); err != nil {
				log.Error(err, fmt.Sprintf("binding %s succeeded but could not store secret", serviceBinding.Status.BindingID))
				return ctrl.Result{}, err
			}
		case smTypes.DELETE:
			return r.removeBindingFromKubernetes(ctx, serviceBinding, log)
		}
	}

	serviceBinding.Status.OperationURL = ""
	serviceBinding.Status.OperationType = ""

	return ctrl.Result{}, r.updateStatusWithRetries(ctx, serviceBinding, log)
}

func (r *ServiceBindingReconciler) SetOwner(ctx context.Context, serviceInstance *v1alpha1.ServiceInstance, serviceBinding *v1alpha1.ServiceBinding, log logr.Logger) error {
	if bindingAlreadyOwnedByInstance(serviceInstance, serviceBinding) {
		log.Info("Binding already owned by instance", "bindingName", serviceBinding.Name, "instanceName", serviceInstance.Name)
		return nil
	}

	log.Info("Binding instance as owner of binding", "bindingName", serviceBinding.Name, "instanceName", serviceInstance.Name)
	if err := controllerutil.SetControllerReference(serviceInstance, serviceBinding, r.Scheme); err != nil {
		log.Error(err, fmt.Sprintf("Could not update the smBinding %s owner instance reference", serviceBinding.Name))
		return err
	}
	if err := r.Update(ctx, serviceBinding); err != nil {
		log.Error(err, "Failed to set controller reference", "bindingName", serviceBinding.Name)
		return err
	}
	return nil
}

func bindingAlreadyOwnedByInstance(instance *v1alpha1.ServiceInstance, binding *v1alpha1.ServiceBinding) bool {
	if existing := metav1.GetControllerOf(binding); existing != nil {
		aGV, err := schema.ParseGroupVersion(existing.APIVersion)
		if err != nil {
			return false
		}

		bGV, err := schema.ParseGroupVersion(instance.APIVersion)
		if err != nil {
			return false
		}

		return aGV.Group == bGV.Group && existing.Kind == instance.Kind && existing.Name == instance.Name
	}

	return false
}

func serviceNotUsable(instance *v1alpha1.ServiceInstance) bool {
	return isDelete(instance.ObjectMeta) || instance.Status.Conditions[0].Reason == getConditionReason(smTypes.CREATE, smTypes.FAILED)
}

func (r *ServiceBindingReconciler) getServiceInstanceForBinding(ctx context.Context, binding *v1alpha1.ServiceBinding) (*v1alpha1.ServiceInstance, error) {
	serviceInstance := &v1alpha1.ServiceInstance{}
	if err := r.Get(ctx, types.NamespacedName{Name: binding.Spec.ServiceInstanceName, Namespace: binding.Namespace}, serviceInstance); err != nil {
		return nil, err
	}

	return serviceInstance.DeepCopy(), nil
}

func (r *ServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ServiceBinding{}).
		Complete(r)
}

func (r *ServiceBindingReconciler) resyncBindingStatus(k8sBinding *v1alpha1.ServiceBinding, smBinding *smclientTypes.ServiceBinding, serviceInstanceID string) {
	k8sBinding.Status.ObservedGeneration = k8sBinding.Generation
	k8sBinding.Status.BindingID = smBinding.ID
	k8sBinding.Status.InstanceID = serviceInstanceID
	k8sBinding.Status.OperationURL = ""
	k8sBinding.Status.OperationType = ""
	switch smBinding.LastOperation.State {
	case smTypes.PENDING:
		fallthrough
	case smTypes.IN_PROGRESS:
		k8sBinding.Status.OperationURL = sm.BuildOperationURL(smBinding.LastOperation.ID, smBinding.ID, web.ServiceBindingsURL)
		k8sBinding.Status.OperationType = smBinding.LastOperation.Type
		setInProgressCondition(smBinding.LastOperation.Type, smBinding.LastOperation.Description, k8sBinding)
	case smTypes.SUCCEEDED:
		setSuccessConditions(smBinding.LastOperation.Type, k8sBinding)
	case smTypes.FAILED:
		setFailureConditions(smBinding.LastOperation.Type, smBinding.LastOperation.Description, k8sBinding)
	}
}

func (r *ServiceBindingReconciler) storeBindingSecret(ctx context.Context, k8sBinding *v1alpha1.ServiceBinding, smBinding *smclientTypes.ServiceBinding, log logr.Logger) error {
	logger := log.WithValues("bindingName", k8sBinding.Name, "secretName", k8sBinding.Spec.SecretName)

	var credentialsMap map[string][]byte
	if len(smBinding.Credentials) == 0 {
		log.Info("Binding credentials are empty")
		credentialsMap = make(map[string][]byte)
	} else {
		var err error
		credentialsMap, err = normalizeCredentials(smBinding.Credentials)
		if err != nil {
			logger.Error(err, "Failed to store binding secret")
			return fmt.Errorf("failed to create secret. Error: %v", err.Error())
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sBinding.Spec.SecretName,
			Labels:    map[string]string{"binding": k8sBinding.Name},
			Namespace: k8sBinding.Namespace,
		},
		Data: credentialsMap,
	}
	if err := controllerutil.SetControllerReference(k8sBinding, secret, r.Scheme); err != nil {
		logger.Error(err, "Failed to set secret owner")
		return err
	}

	log.Info("Creating binding secret")
	if err := r.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (r *ServiceBindingReconciler) deleteBindingSecret(ctx context.Context, binding *v1alpha1.ServiceBinding, log logr.Logger) error {
	log.Info("Deleting binding secret")
	bindingSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
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

	if err := r.Delete(ctx, bindingSecret); err != nil {
		log.Error(err, "Failed to delete binding secret")
		return err
	}

	log.Info("secret was deleted successfully")
	return nil
}

func (r *ServiceBindingReconciler) getBindingForRecovery(smClient sm.Client, serviceBinding *v1alpha1.ServiceBinding, log logr.Logger) (*smclientTypes.ServiceBinding, error) {
	parameters := sm.Parameters{
		FieldQuery: []string{
			fmt.Sprintf("name eq '%s'", serviceBinding.Spec.ExternalName),
			fmt.Sprintf("context/clusterid eq '%s'", r.Config.ClusterID),
			fmt.Sprintf("context/namespace eq '%s'", serviceBinding.Namespace)},
		LabelQuery: []string{
			fmt.Sprintf("%s eq '%s'", k8sNameLabel, serviceBinding.Name)},
		GeneralParams: []string{"attach_last_operations=true"},
	}

	bindings, err := smClient.ListBindings(&parameters)
	if err != nil {
		log.Error(err, "failed to list bindings in SM")
		return nil, err
	}
	if bindings != nil && len(bindings.ServiceBindings) == 1 {
		return &bindings.ServiceBindings[0], nil
	}

	return nil, nil
}

func (r *ServiceBindingReconciler) removeBindingFromKubernetes(ctx context.Context, serviceBinding *v1alpha1.ServiceBinding, log logr.Logger) (ctrl.Result, error) {
	serviceBinding.Status.BindingID = ""
	setSuccessConditions(smTypes.DELETE, serviceBinding)
	if err := r.updateStatusWithRetries(ctx, serviceBinding, log); err != nil {
		return ctrl.Result{}, err
	}

	// delete binding secret if exist
	if err := r.deleteBindingSecret(ctx, serviceBinding, log); err != nil {
		return ctrl.Result{}, err
	}

	// remove our finalizer from the list and update it.
	if err := r.removeFinalizer(ctx, serviceBinding, bindingFinalizerName, log); err != nil {
		return ctrl.Result{}, err
	}

	// Stop reconciliation as the item is deleted
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) getSecret(ctx context.Context, namespace string, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, secret)
	return secret, err
}

func (r *ServiceBindingReconciler) validateSecretNameIsAvailable(ctx context.Context, binding *v1alpha1.ServiceBinding) error {
	currentSecret, err := r.getSecret(ctx, binding.Namespace, binding.Spec.SecretName)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	if otherBindingName, ok := currentSecret.Labels["binding"]; otherBindingName != binding.Name || !ok {
		if len(otherBindingName) > 0 {
			return fmt.Errorf("secret %s belongs to another binding %s, choose a differnet name", binding.Spec.SecretName, otherBindingName)
		}
		return fmt.Errorf("the specified secret name '%s' is already taken. Choose another name and try again", binding.Spec.SecretName)
	}
	return nil
}

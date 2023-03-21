/*
Copyright 2023.

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
	"github.com/SAP/sap-btp-service-operator/api"
	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	smClientTypes "github.com/SAP/sap-btp-service-operator/client/sm/types"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// SharedServiceInstanceReconciler reconciles a SharedServiceInstance object
type SharedServiceInstanceReconciler struct {
	*BaseReconciler
}

//+kubebuilder:rbac:groups=services.cloud.sap.com,resources=sharedserviceinstances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=services.cloud.sap.com,resources=sharedserviceinstances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=services.cloud.sap.com,resources=sharedserviceinstances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SharedServiceInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *SharedServiceInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	fmt.Println("SharedServiceInstanceReconciler reconciler")
	log := r.Log.WithValues("sharedserviceinstance", req.NamespacedName).WithValues("correlation_id", uuid.New().String())
	ctx = context.WithValue(ctx, LogKey{}, log)

	sharedServiceInstance := &servicesv1.SharedServiceInstance{}
	if err := r.Get(ctx, req.NamespacedName, sharedServiceInstance); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch SharedServiceInstance")
		}
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	sharedServiceInstance = sharedServiceInstance.DeepCopy()
	fmt.Println("Got the shared instance")

	if len(sharedServiceInstance.GetConditions()) == 0 {
		fmt.Println("condition size is 0")
		if err := r.init(ctx, sharedServiceInstance); err != nil {
			return ctrl.Result{}, err
		}
		sharedServiceInstance.Status.Shared = false
		if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
			return ctrl.Result{}, err
		}
	}
	fmt.Println("Passed init")

	smClient, err := r.getSMClient(ctx, sharedServiceInstance)
	if err != nil {
		return r.markAsTransientError(ctx, Unknown, err, sharedServiceInstance)
	}
	fmt.Println("Got sm client")

	if isDelete(sharedServiceInstance.ObjectMeta) {
		return r.delete(ctx, smClient, sharedServiceInstance)
	}
	fmt.Println("Not delete")

	log.Info(fmt.Sprintf("Current generation is %v and observed is %v", sharedServiceInstance.Generation, sharedServiceInstance.GetObservedGeneration()))
	sharedServiceInstance.SetObservedGeneration(sharedServiceInstance.Generation)

	fmt.Println("Trying to get service instance")
	serviceInstance, err := r.getServiceInstanceForShareServiceInstance(ctx, sharedServiceInstance)
	if err != nil || serviceNotUsable(serviceInstance) {
		var shareInstanceErr error
		if err != nil {
			shareInstanceErr = fmt.Errorf("couldn't find the service instance '%s'. Error: %v", sharedServiceInstance.Spec.ServiceInstanceName, err.Error())
		} else {
			shareInstanceErr = fmt.Errorf("service instance '%s' is not usable", sharedServiceInstance.Spec.ServiceInstanceName)
		}

		setBlockedCondition(shareInstanceErr.Error(), sharedServiceInstance)
		if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, shareInstanceErr
	}
	fmt.Println("got - ", serviceInstance.Status.InstanceID)

	if !sharedServiceInstance.Status.Shared {
		fmt.Println("Not shared")
		if sharedServiceInstance.Status.Ready != metav1.ConditionTrue {
			fmt.Println("Ready")
			return r.createShareServiceInstance(ctx, smClient, serviceInstance, sharedServiceInstance)
		} else {
			fmt.Println("not Ready")
			return r.handleShareInstanceChange(ctx, smClient, sharedServiceInstance)
		}
	}
	fmt.Println("shared")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SharedServiceInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicesv1.SharedServiceInstance{}).
		WithOptions(controller.Options{RateLimiter: workqueue.NewItemExponentialFailureRateLimiter(r.Config.RetryBaseDelay, r.Config.RetryMaxDelay)}).
		Complete(r)
}

func (r *SharedServiceInstanceReconciler) handleShareInstanceChange(ctx context.Context, smClient sm.Client, sharedServiceInstance *servicesv1.SharedServiceInstance) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info(fmt.Sprintf("Got instance share change request"))
	shared := false

	fmt.Println("handling sharing instance shared=", shared)

	if sharedServiceInstance.Status.Shared {
		setShareInProgressConditions("Got unshare request", sharedServiceInstance)
		shared = true
	} else {
		setShareInProgressConditions("Got share request", sharedServiceInstance)
	}

	err := smClient.ShareInstance(shared, sharedServiceInstance.Status.InstanceID)
	if err != nil {
		fmt.Println("Got err = ", err.Error())
		return ctrl.Result{}, err
	}

	sharedServiceInstance.Status.Shared = !sharedServiceInstance.Status.Shared
	fmt.Println(sharedServiceInstance.Status.Shared)
	sharedServiceInstance.Status.Ready = metav1.ConditionTrue
	setSuccessShareCondition(shared, sharedServiceInstance)

	fmt.Println("Done handling sharing")
	if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
		fmt.Println("ohohoh, ", err.Error())
		return ctrl.Result{}, err
	}
	fmt.Println("finally done")
	return ctrl.Result{}, nil
}

func (r *SharedServiceInstanceReconciler) delete(ctx context.Context, smClient sm.Client, sharedServiceInstance *servicesv1.SharedServiceInstance) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info("Trying to delete shared service instance object")

	setInProgressConditions(smClientTypes.DELETE, "deleting shared service instance", sharedServiceInstance)
	if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
		return ctrl.Result{}, err
	}

	if sharedServiceInstance.Status.Shared == false {
		setSuccessConditions(smClientTypes.DELETE, sharedServiceInstance)
		if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return r.handleShareInstanceChange(ctx, smClient, sharedServiceInstance)
}

func (r *SharedServiceInstanceReconciler) getServiceInstanceForShareServiceInstance(ctx context.Context, instance *servicesv1.SharedServiceInstance) (*servicesv1.ServiceInstance, error) {
	serviceInstance := &servicesv1.ServiceInstance{}
	if err := r.Get(ctx, types.NamespacedName{Name: instance.Spec.ServiceInstanceName, Namespace: instance.Namespace}, serviceInstance); err != nil {
		return nil, err
	}

	return serviceInstance.DeepCopy(), nil
}

func (r *SharedServiceInstanceReconciler) createShareServiceInstance(ctx context.Context, smClient sm.Client, serviceInstance *servicesv1.ServiceInstance, sharedServiceInstance *servicesv1.SharedServiceInstance) (ctrl.Result, error) {
	log := GetLogger(ctx)
	log.Info("Creating shared service instance in SM")
	sharedServiceInstance.Status.InstanceID = serviceInstance.Status.InstanceID
	return r.handleShareInstanceChange(ctx, smClient, sharedServiceInstance)
}

func setShareInProgressConditions(message string, object api.SAPBTPResource) {
	conditions := object.GetConditions()
	shareCondition := metav1.Condition{
		Type:               api.ConditionShareInProgress,
		Status:             metav1.ConditionTrue,
		Reason:             getConditionReason(api.ConditionShareInProgress, smClientTypes.INPROGRESS),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, shareCondition)
	object.SetConditions(conditions)
}

func setSuccessShareCondition(shared bool, object api.SAPBTPResource) {
	message := "Instance got shared successfully"
	if shared {
		message = "Instance got unshared successfully"
	}
	conditions := object.GetConditions()
	readyCondition := metav1.Condition{
		Type:               api.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             getConditionReason(api.ConditionReady, smClientTypes.SUCCEEDED),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}

	successCondition := metav1.Condition{
		Type:               api.ConditionSucceeded,
		Status:             metav1.ConditionTrue,
		Reason:             getConditionReason(api.ConditionSucceeded, smClientTypes.SUCCEEDED),
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}

	meta.RemoveStatusCondition(&conditions, api.ConditionShareInProgress)
	meta.RemoveStatusCondition(&conditions, api.ConditionSucceeded)
	meta.SetStatusCondition(&conditions, successCondition)
	meta.SetStatusCondition(&conditions, readyCondition)

	object.SetConditions(conditions)
}

func newSharedInstanceObject(name, namespace string) *servicesv1.SharedServiceInstance {
	return &servicesv1.SharedServiceInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: servicesv1.GroupVersion.String(),
			Kind:       "ShareServiceInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

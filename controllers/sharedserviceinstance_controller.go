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

	if len(sharedServiceInstance.GetConditions()) == 0 {
		sharedServiceInstance.Status.Shared = false
		if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.init(ctx, sharedServiceInstance); err != nil {
			return ctrl.Result{}, err
		}
	}

	smClient, err := r.getSMClient(ctx, sharedServiceInstance)
	if err != nil {
		return r.markAsTransientError(ctx, Unknown, err, sharedServiceInstance)
	}

	if isDelete(sharedServiceInstance.ObjectMeta) {
		return r.delete(ctx, smClient, sharedServiceInstance)
	}

	log.Info(fmt.Sprintf("Current generation is %v and observed is %v", sharedServiceInstance.Generation, sharedServiceInstance.GetObservedGeneration()))
	sharedServiceInstance.SetObservedGeneration(sharedServiceInstance.Generation)

	if !sharedServiceInstance.Status.Shared {
		return r.handleShareInstanceChange(ctx, smClient, sharedServiceInstance)
	}

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

	if sharedServiceInstance.Status.Shared {
		setShareInProgressConditions("Got un-share request", sharedServiceInstance)
		shared = true
	} else {
		setShareInProgressConditions("Got share request", sharedServiceInstance)
	}

	err := smClient.ShareInstance(shared, sharedServiceInstance.Status.InstanceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	setSuccessShareCondition(shared, sharedServiceInstance)

	sharedServiceInstance.Status.Shared = !sharedServiceInstance.Status.Shared
	if err := r.updateStatus(ctx, sharedServiceInstance); err != nil {
		return ctrl.Result{}, err
	}

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

func setShareInProgressConditions(message string, object api.SAPBTPResource) {
	conditions := object.GetConditions()
	shareCondition := metav1.Condition{
		Type:               api.ConditionShareInProgress,
		Status:             metav1.ConditionTrue,
		Reason:             message,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.SetStatusCondition(&conditions, shareCondition)
	object.SetConditions(conditions)
}

func setSuccessShareCondition(shared bool, object api.SAPBTPResource) {
	message := "Instance got shared successfully"
	if !shared {
		message = "Instance got un-shared successfully"
	}
	conditions := object.GetConditions()
	successShareCondition := metav1.Condition{
		Type:               api.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             message,
		Message:            message,
		ObservedGeneration: object.GetGeneration(),
	}
	meta.RemoveStatusCondition(&conditions, api.ConditionShareInProgress)
	meta.SetStatusCondition(&conditions, successShareCondition)
	object.SetConditions(conditions)
}

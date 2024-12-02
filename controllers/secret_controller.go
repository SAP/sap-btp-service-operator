package controllers

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/SAP/sap-btp-service-operator/api/common"
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/internal/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// +kubebuilder:rbac:groups=services.cloud.sap.com,resources=secret,verbs=update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=update

func (r *SecretReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.Log.WithValues("secret", req.NamespacedName).WithValues("correlation_id", uuid.New().String())
	ctx = context.WithValue(ctx, utils.LogKey{}, log)
	log.Info(fmt.Sprintf("reconciling secret %s", req.NamespacedName))
	// Fetch the Secret
	var secret corev1.Secret
	if err := r.Get(ctx, req.NamespacedName, &secret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch ServiceInstance")
		}
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var instances v1.ServiceInstanceList
	labelSelector := client.MatchingLabels{common.InstanceSecretLabel + common.Separator + string(secret.GetUID()): secret.Name}
	if err := r.Client.List(ctx, &instances, labelSelector); err != nil {
		log.Error(err, "failed to list service instances")
		return ctrl.Result{}, err
	}
	for _, instance := range instances.Items {
		log.Info(fmt.Sprintf("waking up instance %s", instance.Name))
		instance.Status.SecretChange = true
		err := utils.UpdateStatus(ctx, r.Client, &instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	labelSelector := labels.SelectorFromSet(map[string]string{common.WatchSecretLabel: "true"})
	labelPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return labelSelector.Matches(labels.Set(e.ObjectNew.GetLabels())) && isSecretDataChanged(e)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return labelSelector.Matches(labels.Set(e.Object.GetLabels()))
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(labelPredicate).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
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

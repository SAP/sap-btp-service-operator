package controllers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/SAP/sap-btp-service-operator/internal/utils/logutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
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

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;create;update

func (r *SecretReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.Log.WithValues("secret", req.NamespacedName).WithValues("correlation_id", uuid.New().String())
	ctx = context.WithValue(ctx, logutils.LogKey, log)
	log.Info(fmt.Sprintf("reconciling params secret %s", req.NamespacedName))
	// Fetch the Secret
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, req.NamespacedName, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch Secret")
		}
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	instances := &v1.ServiceInstanceList{}
	labelSelector := client.MatchingLabels{utils.GetLabelKeyForInstanceSecret(secret.Name): secret.Name}
	if err := r.Client.List(ctx, instances, client.InNamespace(secret.Namespace), labelSelector); err != nil {
		log.Error(err, "failed to list service instances")
		return ctrl.Result{}, err
	}

	for _, instance := range instances.Items {
		log.Info(fmt.Sprintf("waking up referencing instance %s", instance.Name))
		instance.Status.ForceReconcile = true
		err := utils.UpdateStatus(ctx, r.Client, &instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	if utils.IsMarkedForDeletion(secret.ObjectMeta) {
		log.Info("secret is marked for deletion, removing finalizer")
		return ctrl.Result{}, utils.RemoveFinalizer(ctx, r.Client, secret, common.FinalizerName)
	}

	log.Info("finished reconciling params secret")
	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	labelSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{common.WatchSecretLabel: "true"},
	}
	selectorPredicate, err := predicate.LabelSelectorPredicate(labelSelector)
	if err != nil {
		return err
	}

	dataChangedPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !reflect.DeepEqual(
				e.ObjectOld.(*corev1.Secret).Data,
				e.ObjectNew.(*corev1.Secret).Data,
			)
		},
		DeleteFunc: func(e event.DeleteEvent) bool { return true },
		CreateFunc: func(e event.CreateEvent) bool { return false },
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(selectorPredicate, dataChangedPredicate)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

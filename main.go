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

package main

import (
	"context"
	"flag"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"k8s.io/apimachinery/pkg/labels"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/SAP/sap-btp-service-operator/internal/utils"

	"k8s.io/client-go/rest"

	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/SAP/sap-btp-service-operator/internal/config"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/SAP/sap-btp-service-operator/api/common"
	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/api/v1/webhooks"
	"github.com/SAP/sap-btp-service-operator/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = servicesv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var loggerUseDevMode bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoints bind to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&loggerUseDevMode, "logger_use_dev_mode", true,
		"Sets the logger to use dev mode, e.g. more friendly printing format")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(loggerUseDevMode)))

	mgrOptions := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: 9443,
		}),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "aa689ecc.cloud.sap.com",
	}

	if config.Get().EnableLimitedCache {
		setupLog.Info("limited cache enabled")
		mgrOptions.Cache = cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&v1.Secret{}:                  {Label: labels.SelectorFromSet(map[string]string{common.ManagedByBTPOperatorLabel: "true"})},
				&v1.ConfigMap{}:               {Label: labels.SelectorFromSet(map[string]string{common.ManagedByBTPOperatorLabel: "true"})},
				&servicesv1.ServiceInstance{}: {},
				&servicesv1.ServiceBinding{}:  {},
			},
		}
	}
	syncPeriod := 10 * time.Hour
	mgrOptions.Cache.SyncPeriod = &syncPeriod

	if !config.Get().AllowClusterAccess {
		allowedNamespaces := config.Get().AllowedNamespaces
		allowedNamespaces = append(allowedNamespaces, config.Get().ReleaseNamespace)
		setupLog.Info(fmt.Sprintf("Allowed namespaces are %v", allowedNamespaces))
		result := make(map[string]cache.Config)
		for _, s := range allowedNamespaces {
			result[s] = cache.Config{}
		}

		if config.Get().EnableLimitedCache {
			mgrOptions.Cache.DefaultNamespaces = result
		} else {
			mgrOptions.NewCache = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
				opts.DefaultNamespaces = result
				return cache.New(config, opts)
			}
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if len(config.Get().InitialClusterID) == 0 {
		setupLog.Info("cluster secret not found, creating it")
		createClusterSecret(mgr.GetClient())
	} else if config.Get().InitialClusterID != config.Get().ClusterID {
		panic(fmt.Sprintf("ClusterID changed, which is not supported. Please redeploy with --set cluster.id=%s", config.Get().InitialClusterID))
	}

	var nonCachedClient client.Client
	if config.Get().EnableLimitedCache {
		var clErr error
		nonCachedClient, clErr = client.New(mgr.GetConfig(), client.Options{Scheme: scheme})
		if clErr != nil {
			setupLog.Error(clErr, "unable to create non cached client")
			os.Exit(1)
		}
	}

	utils.InitializeSecretsClient(mgr.GetClient(), nonCachedClient, config.Get())

	if err = (&controllers.ServiceInstanceReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("ServiceInstance"),
		Scheme:      mgr.GetScheme(),
		Config:      config.Get(),
		Recorder:    mgr.GetEventRecorderFor("ServiceInstance"),
		GetSMClient: utils.GetSMClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceInstance")
		os.Exit(1)
	}
	if err = (&controllers.ServiceBindingReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("ServiceBinding"),
		Scheme:      mgr.GetScheme(),
		Config:      config.Get(),
		Recorder:    mgr.GetEventRecorderFor("ServiceBinding"),
		GetSMClient: utils.GetSMClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceBinding")
		os.Exit(1)
	}
	if err = (&controllers.SecretReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Secret"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Secret")
		os.Exit(1)
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		mgr.GetWebhookServer().Register("/mutate-services-cloud-sap-com-v1-serviceinstance", &webhook.Admission{Handler: &webhooks.ServiceInstanceDefaulter{Decoder: admission.NewDecoder(mgr.GetScheme())}})
		mgr.GetWebhookServer().Register("/mutate-services-cloud-sap-com-v1-servicebinding", &webhook.Admission{Handler: &webhooks.ServiceBindingDefaulter{Decoder: admission.NewDecoder(mgr.GetScheme())}})
		if err = (&servicesv1.ServiceBinding{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ServiceBinding")
			os.Exit(1)
		}
		if err = (&servicesv1.ServiceInstance{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ServiceInstance")
			os.Exit(1)
		}
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}

func createClusterSecret(client client.Client) {
	clusterSecret := &v1.Secret{}
	clusterSecret.Name = "sap-btp-operator-clusterid"
	clusterSecret.Namespace = config.Get().ReleaseNamespace
	clusterSecret.Labels = map[string]string{common.ManagedByBTPOperatorLabel: "true", common.ClusterSecretLabel: "true"}
	clusterSecret.StringData = map[string]string{"INITIAL_CLUSTER_ID": config.Get().ClusterID}
	clusterSecret.Labels = map[string]string{common.ManagedByBTPOperatorLabel: "true"}
	if err := client.Create(context.Background(), clusterSecret); err != nil {
		setupLog.Error(err, "failed to create cluster secret")
	}
}

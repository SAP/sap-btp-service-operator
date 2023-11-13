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
	"flag"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/client-go/rest"

	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/SAP/sap-btp-service-operator/api/v1/webhooks"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/SAP/sap-btp-service-operator/internal/secrets"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/SAP/sap-btp-service-operator/internal/config"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	servicesv1 "github.com/SAP/sap-btp-service-operator/api/v1"
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
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoints bind to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "aa689ecc.cloud.sap.com",
	}

	if !config.Get().AllowClusterAccess {
		allowedNamespaces := config.Get().AllowedNamespaces
		allowedNamespaces = append(allowedNamespaces, config.Get().ReleaseNamespace)
		setupLog.Info(fmt.Sprintf("Allowed namespaces are %v", allowedNamespaces))
		mgrOptions.NewCache = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			opts.Namespaces = allowedNamespaces
			return cache.New(config, opts)
		}
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	secretResolver := &secrets.SecretResolver{
		ManagementNamespace:    config.Get().ManagementNamespace,
		ReleaseNamespace:       config.Get().ReleaseNamespace,
		EnableNamespaceSecrets: config.Get().EnableNamespaceSecrets,
		Client:                 mgr.GetClient(),
		Log:                    logf.Log.WithName("secret-resolver"),
	}

	if err = (&controllers.ServiceInstanceReconciler{
		BaseReconciler: &controllers.BaseReconciler{
			Client:         mgr.GetClient(),
			Log:            ctrl.Log.WithName("controllers").WithName("ServiceInstance"),
			Scheme:         mgr.GetScheme(),
			Config:         config.Get(),
			SecretResolver: secretResolver,
			Recorder:       mgr.GetEventRecorderFor("ServiceInstance"),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceInstance")
		os.Exit(1)
	}
	if err = (&controllers.ServiceBindingReconciler{
		BaseReconciler: &controllers.BaseReconciler{
			Client:         mgr.GetClient(),
			Log:            ctrl.Log.WithName("controllers").WithName("ServiceBinding"),
			Scheme:         mgr.GetScheme(),
			Config:         config.Get(),
			SecretResolver: secretResolver,
			Recorder:       mgr.GetEventRecorderFor("ServiceBinding"),
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceBinding")
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

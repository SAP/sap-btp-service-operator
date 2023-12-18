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

// +kubebuilder:scaffold:imports
import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	ginkgo_config "github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"

	"github.com/SAP/sap-btp-service-operator/api"
	v1 "github.com/SAP/sap-btp-service-operator/api/v1"
	"github.com/SAP/sap-btp-service-operator/api/v1/webhooks"
	"github.com/SAP/sap-btp-service-operator/client/sm"
	"github.com/SAP/sap-btp-service-operator/client/sm/smfakes"
	"github.com/SAP/sap-btp-service-operator/internal/config"

	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	timeout                   = time.Second * 20
	interval                  = time.Millisecond * 50
	syncPeriod                = time.Millisecond * 250
	pollInterval              = time.Millisecond * 250
	ignoreNonTransientTimeout = time.Second * 10

	fakeBindingID        = "fake-binding-id"
	bindingTestNamespace = "test-namespace"
)

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	fakeClient *smfakes.FakeClient
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	ginkgo_config.DefaultReporterConfig.Verbose = false
	RunSpecs(t, "Controllers Suite")
}

var _ = BeforeSuite(func(done Done) {
	printSection("Starting BeforeSuite")

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(false)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "config", "webhook")},
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = v1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	webhookInstallOptions := &testEnv.WebhookInstallOptions

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).ToNot(HaveOccurred())

	fakeClient = &smfakes.FakeClient{}
	testConfig := config.Get()
	testConfig.SyncPeriod = syncPeriod
	testConfig.PollInterval = pollInterval
	testConfig.IgnoreNonTransientTimeout = ignoreNonTransientTimeout

	By("registering webhooks")
	k8sManager.GetWebhookServer().Register("/mutate-services-cloud-sap-com-v1-serviceinstance", &webhook.Admission{Handler: &webhooks.ServiceInstanceDefaulter{Decoder: admission.NewDecoder(k8sManager.GetScheme())}})
	k8sManager.GetWebhookServer().Register("/mutate-services-cloud-sap-com-v1-servicebinding", &webhook.Admission{Handler: &webhooks.ServiceBindingDefaulter{Decoder: admission.NewDecoder(k8sManager.GetScheme())}})

	err = (&v1.ServiceBinding{}).SetupWebhookWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&v1.ServiceInstance{}).SetupWebhookWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	By("registering controllers")
	err = (&ServiceInstanceReconciler{
		BaseReconciler: &BaseReconciler{
			Client:   k8sManager.GetClient(),
			Scheme:   k8sManager.GetScheme(),
			Log:      ctrl.Log.WithName("controllers").WithName("ServiceInstance"),
			SMClient: func() sm.Client { return fakeClient },
			Config:   testConfig,
			Recorder: k8sManager.GetEventRecorderFor("ServiceInstance"),
		},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ServiceBindingReconciler{
		BaseReconciler: &BaseReconciler{
			Client:   k8sManager.GetClient(),
			Scheme:   k8sManager.GetScheme(),
			Log:      ctrl.Log.WithName("controllers").WithName("ServiceBinding"),
			SMClient: func() sm.Client { return fakeClient },
			Config:   testConfig,
			Recorder: k8sManager.GetEventRecorderFor("ServiceBinding"),
		},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	// +kubebuilder:scaffold:webhook

	By("starting the k8s manager")
	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	By("waiting for the webhook server to get ready")
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		_ = conn.Close()
		return nil
	}, timeout, interval).Should(Succeed())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	By("creating namespace " + testNamespace)
	nsSpec := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
	err = k8sClient.Create(context.Background(), nsSpec)
	Expect(err).ToNot(HaveOccurred())

	By("creating namespace " + bindingTestNamespace)
	nsSpec = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: bindingTestNamespace}}
	err = k8sClient.Create(context.Background(), nsSpec)
	Expect(err).ToNot(HaveOccurred())

	printSection("Finished BeforeSuite")
	close(done)
}, 60)

var _ = AfterSuite(func() {
	printSection("Starting AfterSuite")

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())

	printSection("Finished AfterSuite")
})

func isResourceReady(resource api.SAPBTPResource) bool {
	return resource.GetObservedGeneration() == resource.GetGeneration() &&
		meta.IsStatusConditionPresentAndEqual(resource.GetConditions(), api.ConditionReady, metav1.ConditionTrue)
}

func waitForResourceToBeReady(ctx context.Context, resource api.SAPBTPResource) {
	waitForResourceCondition(ctx, resource, api.ConditionReady, metav1.ConditionTrue, "", "")
}

func waitForResourceCondition(ctx context.Context, resource api.SAPBTPResource, conditionType string, status metav1.ConditionStatus, reason, message string) {
	key := getResourceNamespacedName(resource)
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, resource); err != nil {
			return false
		}

		if resource.GetObservedGeneration() != resource.GetGeneration() {
			return false
		}

		cond := meta.FindStatusCondition(resource.GetConditions(), conditionType)
		if cond == nil {
			return false
		}

		if cond.Status != status {
			return false
		}

		if len(reason) > 0 && cond.Reason != reason {
			return false
		}

		if len(message) > 0 && !strings.Contains(cond.Message, message) {
			return false
		}

		return true
	}, timeout*2, interval).Should(BeTrue(),
		eventuallyMsgForResource(
			fmt.Sprintf("expected condition: {type: %s, status: %s, reason: %s, message: %s} was not met", conditionType, status, reason, message),
			key,
			resource),
	)
}
func waitForResourceAnnotationRemove(ctx context.Context, resource api.SAPBTPResource, annotationsKey ...string) {
	key := getResourceNamespacedName(resource)
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, resource); err != nil {
			return false
		}
		for _, annotationKey := range annotationsKey {
			_, ok := resource.GetAnnotations()[annotationKey]
			if ok {
				return false
			}
		}
		return true
	}, timeout*2, interval).Should(BeTrue(),
		eventuallyMsgForResource(
			fmt.Sprintf("annotation %s was not removed", annotationsKey),
			key,
			resource),
	)
}

func getResourceNamespacedName(resource client.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()}
}

func deleteAndWait(ctx context.Context, key types.NamespacedName, resource client.Object) {
	wait := true
	Eventually(func() bool {
		if err := k8sClient.Get(ctx, key, resource); err != nil {
			if apierrors.IsNotFound(err) {
				wait = false
				return true
			}
			return false
		}

		if err := k8sClient.Delete(ctx, resource); err != nil {
			if apierrors.IsNotFound(err) {
				wait = false
				return true
			}
			return false
		}

		return true
	}, timeout, interval).Should(BeTrue(), eventuallyMsgForResource("failed to mark for deletion", key, resource))

	if wait {
		waitForResourceToBeDeleted(ctx, key, resource)
	}
}

func waitForResourceToBeDeleted(ctx context.Context, key types.NamespacedName, resource client.Object) {
	Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(ctx, key, resource))
	}, timeout, interval).Should(BeTrue(), eventuallyMsgForResource("resource is not deleted", key, resource))
}

func createParamsSecret(namespace string) {
	credentialsMap := make(map[string][]byte)
	credentialsMap["secret-parameter"] = []byte("{\"secret-key\":\"secret-value\"}")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "param-secret",
			Namespace: namespace,
		},
		Data: credentialsMap,
	}

	Expect(k8sClient.Create(context.Background(), secret)).ToNot(HaveOccurred())
}

func printSection(str string) {
	ul := strings.Builder{}
	for i := 1; i <= len(str); i++ {
		ul.WriteString("=")
	}
	fmt.Println(ul.String())
	fmt.Println(str)
	fmt.Println(ul.String())

	fmt.Println()
}

func getNonTransientBrokerError(errMessage string) error {
	return &sm.ServiceManagerError{
		StatusCode:  http.StatusBadRequest,
		Description: "smErrMessage",
		BrokerError: &api.HTTPStatusCodeError{
			StatusCode:   400,
			ErrorMessage: &errMessage,
		}}
}

func getTransientBrokerError(errorMessage string) error {
	return &sm.ServiceManagerError{
		StatusCode:  http.StatusBadGateway,
		Description: "smErrMessage",
		BrokerError: &api.HTTPStatusCodeError{
			StatusCode:   http.StatusTooManyRequests,
			ErrorMessage: &errorMessage,
		},
	}
}

func eventuallyMsgForResource(message string, key types.NamespacedName, resource client.Object) string {
	gvk, _ := apiutil.GVKForObject(resource, scheme.Scheme)
	return fmt.Sprintf("eventaully failure for %s %s. message: %s", gvk.Kind, key.String(), message)
}

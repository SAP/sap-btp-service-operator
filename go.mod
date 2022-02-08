module github.com/SAP/sap-btp-service-operator

go 1.15

require (
	github.com/Peripli/service-manager v0.19.0
	github.com/go-logr/logr v0.3.0
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/google/uuid v1.1.2
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/onrik/logrus v0.8.0 // indirect
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.3
	golang.org/x/oauth2 v0.0.0-20201208152858-08078c50e5b5
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v0.20.1
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/yaml v1.2.0
)

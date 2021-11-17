// Your header goes here...
//

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	servicescloudsapcomv1alpha1 "github.com/SAP/sap-btp-service-operator/api/services.cloud.sap.com/v1alpha1"
	versioned "github.com/SAP/sap-btp-service-operator/client/clientset/versioned"
	internalinterfaces "github.com/SAP/sap-btp-service-operator/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/SAP/sap-btp-service-operator/client/listers/services.cloud.sap.com/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// ServiceInstanceInformer provides access to a shared informer and lister for
// ServiceInstances.
type ServiceInstanceInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.ServiceInstanceLister
}

type serviceInstanceInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewServiceInstanceInformer constructs a new informer for ServiceInstance type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewServiceInstanceInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredServiceInstanceInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredServiceInstanceInformer constructs a new informer for ServiceInstance type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredServiceInstanceInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ServicesV1alpha1().ServiceInstances(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ServicesV1alpha1().ServiceInstances(namespace).Watch(context.TODO(), options)
			},
		},
		&servicescloudsapcomv1alpha1.ServiceInstance{},
		resyncPeriod,
		indexers,
	)
}

func (f *serviceInstanceInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredServiceInstanceInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *serviceInstanceInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&servicescloudsapcomv1alpha1.ServiceInstance{}, f.defaultInformer)
}

func (f *serviceInstanceInformer) Lister() v1alpha1.ServiceInstanceLister {
	return v1alpha1.NewServiceInstanceLister(f.Informer().GetIndexer())
}

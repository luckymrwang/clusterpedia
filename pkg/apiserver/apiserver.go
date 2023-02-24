package apiserver

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/api/errors"
	metainternal "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	clientrest "k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	internal "github.com/clusterpedia-io/api/clusterpedia"
	"github.com/clusterpedia-io/api/clusterpedia/install"
	"github.com/clusterpedia-io/clusterpedia/pkg/apiserver/registry/clusterpedia/resources"
	"github.com/clusterpedia-io/clusterpedia/pkg/client/clientset/versioned"
	"github.com/clusterpedia-io/clusterpedia/pkg/informers"
	"github.com/clusterpedia-io/clusterpedia/pkg/kubeapiserver"
	"github.com/clusterpedia-io/clusterpedia/pkg/utils/filters"
)

var (
	// Scheme defines methods for serializing and deserializing API objects.
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific
	// versions and content types.
	Codecs = serializer.NewCodecFactory(Scheme)

	// ParameterCodec handles versioning of objects that are converted to query parameters.
	ParameterCodec = runtime.NewParameterCodec(Scheme)
)

func init() {
	install.Install(Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})
	_ = metainternal.AddToScheme(Scheme)

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

// Config defines the config for the apiserver
type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
}

type ClusterPediaServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig

	ClientConfig *clientrest.Config
}

// CompletedConfig embeds a private pointer that cannot be instantiated outside of this package.
type CompletedConfig struct {
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		cfg.GenericConfig.ClientConfig,
	}

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedConfig{&c}
}

func (config completedConfig) New() (*ClusterPediaServer, error) {
	if config.ClientConfig == nil {
		return nil, fmt.Errorf("CompletedConfig.New() called with config.ClientConfig == nil")
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config.ClientConfig)
	if err != nil {
		return nil, err
	}
	initialAPIGroupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}

	kubernetesClient, err := kubernetes.NewForConfig(config.ClientConfig)
	if err != nil {
		return nil, err
	}
	aggregationClient, err := versioned.NewForConfig(config.ClientConfig)
	if err != nil {
		return nil, err
	}
	informerFactory := informers.NewInformerFactories(kubernetesClient, aggregationClient)

	resourceServerConfig := kubeapiserver.NewDefaultConfig()
	resourceServerConfig.GenericConfig.ExternalAddress = config.GenericConfig.ExternalAddress
	resourceServerConfig.GenericConfig.LoopbackClientConfig = config.GenericConfig.LoopbackClientConfig
	resourceServerConfig.ExtraConfig = kubeapiserver.ExtraConfig{
		InitialAPIGroupResources: initialAPIGroupResources,
		InformerFactory:          informerFactory,
	}
	kubeResourceAPIServer, err := resourceServerConfig.Complete().New(genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	handlerChainFunc := config.GenericConfig.BuildHandlerChainFunc
	config.GenericConfig.BuildHandlerChainFunc = func(apiHandler http.Handler, c *genericapiserver.Config) http.Handler {
		handler := handlerChainFunc(apiHandler, c)
		handler = filters.WithRequestQuery(handler)
		handler = filters.WithAcceptHeader(handler)
		return handler
	}

	genericServer, err := config.GenericConfig.New("clusterpedia", hooksDelegate{kubeResourceAPIServer})
	if err != nil {
		return nil, err
	}

	v1beta1storage := map[string]rest.Storage{}
	v1beta1storage["resources"] = resources.NewREST(kubeResourceAPIServer.Handler.NonGoRestfulMux)

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(internal.GroupName, Scheme, ParameterCodec, Codecs)
	apiGroupInfo.VersionedResourcesStorageMap["v1beta1"] = v1beta1storage
	if err := genericServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	genericServer.AddPostStartHookOrDie("start-clusterpedia-informers", func(context genericapiserver.PostStartHookContext) error {
		return waitForResourceSync(context.StopCh, informerFactory, kubernetesClient)
	})

	return &ClusterPediaServer{
		GenericAPIServer: genericServer,
	}, nil
}

func (server *ClusterPediaServer) Run(ctx context.Context) error {
	return server.GenericAPIServer.PrepareRun().Run(ctx.Done())
}

type hooksDelegate struct {
	genericapiserver.DelegationTarget
}

func (s hooksDelegate) UnprotectedHandler() http.Handler {
	return nil
}

func (s hooksDelegate) HealthzChecks() []healthz.HealthChecker {
	return []healthz.HealthChecker{}
}

func (s hooksDelegate) ListedPaths() []string {
	return []string{}
}

func (s hooksDelegate) NextDelegate() genericapiserver.DelegationTarget {
	return nil
}

func waitForResourceSync(stopCh <-chan struct{}, informerFactory informers.InformerFactory, kubernetesClient *kubernetes.Clientset) error {
	klog.V(0).Info("Start cache objects")

	// resources we have to create informer first
	k8sGVRs := map[schema.GroupVersion][]string{
		{Group: "", Version: "v1"}: {
			"namespaces",
			"nodes",
			"resourcequotas",
			"pods",
			"services",
			"persistentvolumeclaims",
			"persistentvolumes",
			"secrets",
			"configmaps",
			"serviceaccounts",
		},
		{Group: "apps", Version: "v1"}: {
			"deployments",
			"daemonsets",
			"replicasets",
			"statefulsets",
			"controllerrevisions",
		},
		{Group: "storage.k8s.io", Version: "v1"}: {
			"storageclasses",
		},
		{Group: "batch", Version: "v1"}: {
			"jobs",
		},
		{Group: "batch", Version: "v1beta1"}: {
			"cronjobs",
		},
		{Group: "networking.k8s.io", Version: "v1"}: {
			"ingresses",
			"networkpolicies",
		},
		{Group: "autoscaling", Version: "v2beta2"}: {
			"horizontalpodautoscalers",
		},
	}

	if err := waitForCacheSync(kubernetesClient.Discovery(),
		informerFactory.KubernetesSharedInformerFactory(),
		func(resource schema.GroupVersionResource) (interface{}, error) {
			return informerFactory.KubernetesSharedInformerFactory().ForResource(resource)
		},
		k8sGVRs, stopCh); err != nil {
		return err
	}

	klog.V(0).Info("Finished caching objects")
	return nil

}

func waitForCacheSync(discoveryClient discovery.DiscoveryInterface, sharedInformerFactory informers.GenericInformerFactory, informerForResourceFunc informerForResourceFunc, GVRs map[schema.GroupVersion][]string, stopCh <-chan struct{}) error {
	for groupVersion, resourceNames := range GVRs {
		var apiResourceList *metav1.APIResourceList
		var err error
		err = retry.OnError(retry.DefaultRetry, func(err error) bool {
			return !errors.IsNotFound(err)
		}, func() error {
			apiResourceList, err = discoveryClient.ServerResourcesForGroupVersion(groupVersion.String())
			return err
		})
		if err != nil {
			return fmt.Errorf("failed to fetch group version resources %s: %s", groupVersion, err)
		}
		for _, resourceName := range resourceNames {
			groupVersionResource := groupVersion.WithResource(resourceName)
			if !isResourceExists(apiResourceList.APIResources, groupVersionResource) {
				klog.Warningf("resource %s not exists in the cluster", groupVersionResource)
			} else {
				// reflect.ValueOf(sharedInformerFactory).MethodByName("ForResource").Call([]reflect.Value{reflect.ValueOf(groupVersionResource)})
				if _, err = informerForResourceFunc(groupVersionResource); err != nil {
					return fmt.Errorf("failed to create informer for %s: %s", groupVersionResource, err)
				}
			}
		}
	}
	sharedInformerFactory.Start(stopCh)
	sharedInformerFactory.WaitForCacheSync(stopCh)
	return nil
}

func isResourceExists(apiResources []metav1.APIResource, resource schema.GroupVersionResource) bool {
	for _, apiResource := range apiResources {
		if apiResource.Name == resource.Resource {
			return true
		}
	}
	return false
}

type informerForResourceFunc func(resource schema.GroupVersionResource) (interface{}, error)

package kubeapiserver

import (
	"strings"
	"sync"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/restmapper"
	apicore "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/apis/rbac"
	apisstorage "k8s.io/kubernetes/pkg/apis/storage"
	printersinternal "k8s.io/kubernetes/pkg/printers/internalversion"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/clusterpedia-io/clusterpedia/pkg/kubeapiserver/printers"
	"github.com/clusterpedia-io/clusterpedia/pkg/kubeapiserver/resourcerest"
	"github.com/clusterpedia-io/clusterpedia/pkg/scheme"
	"github.com/clusterpedia-io/clusterpedia/pkg/storage"
	"github.com/clusterpedia-io/clusterpedia/pkg/storageconfig"
)

type RESTManager struct {
	serializer                 runtime.NegotiatedSerializer
	storageFactory             storage.StorageFactory
	resourcetSorageConfig      *storageconfig.StorageConfigFactory
	equivalentResourceRegistry runtime.EquivalentResourceMapper

	lock      sync.Mutex
	groups    atomic.Value // map[string]metav1.APIGroup
	resources atomic.Value // map[schema.GroupResource]metav1.APIResource

	restResourceInfos atomic.Value // map[schema.GroupVersionResource]RESTResourceInfo

	requestVerbs metav1.Verbs
}

func NewRESTManager(serializer runtime.NegotiatedSerializer, storageMediaType string, storageFactory storage.StorageFactory, initialAPIGroupResources []*restmapper.APIGroupResources) *RESTManager {
	requestVerbs := storageFactory.GetSupportedRequestVerbs()

	apiresources := make(map[schema.GroupResource]metav1.APIResource)
	for _, groupresources := range initialAPIGroupResources {
		group := groupresources.Group
		for _, version := range group.Versions {
			resources, ok := groupresources.VersionedResources[version.Version]
			if !ok {
				continue
			}

			for _, resource := range resources {
				if strings.Contains(resource.Name, "/") {
					// skip subresources
					continue
				}

				gr := schema.GroupResource{Group: group.Name, Resource: resource.Name}
				if _, ok := apiresources[gr]; ok {
					continue
				}

				gk := schema.GroupKind{Group: group.Name, Kind: resource.Kind}
				if gvs := scheme.LegacyResourceScheme.VersionsForGroupKind(gk); len(gvs) == 0 {
					// skip custom resource
					continue
				}

				resource.Verbs = requestVerbs
				apiresources[gr] = resource
			}
		}
	}

	manager := &RESTManager{
		serializer:                 serializer,
		storageFactory:             storageFactory,
		resourcetSorageConfig:      storageconfig.NewStorageConfigFactory(),
		equivalentResourceRegistry: runtime.NewEquivalentResourceRegistry(),
		requestVerbs:               requestVerbs,
	}

	manager.resources.Store(apiresources)
	manager.groups.Store(map[string]metav1.APIGroup{})
	manager.restResourceInfos.Store(make(map[schema.GroupVersionResource]RESTResourceInfo))
	return manager
}

func (m *RESTManager) GetAPIGroups() map[string]metav1.APIGroup {
	return m.groups.Load().(map[string]metav1.APIGroup)
}

func (m *RESTManager) GetRESTResourceInfo(gvr schema.GroupVersionResource) RESTResourceInfo {
	infos := m.restResourceInfos.Load().(map[schema.GroupVersionResource]RESTResourceInfo)
	return infos[gvr]
}

type RESTResourceInfo struct {
	APIResource  metav1.APIResource
	RequestScope *handlers.RequestScope
	Storage      *resourcerest.RESTStorage
}

func (info RESTResourceInfo) Empty() bool {
	return info.APIResource.Name == "" || info.RequestScope == nil || info.Storage == nil
}

var legacyResourcesWithDefaultTableConvertor = map[schema.GroupResource]struct{}{
	apicore.Resource("limitranges"):              {},
	rbac.Resource("roles"):                       {},
	rbac.Resource("clusterroles"):                {},
	apisstorage.Resource("csistoragecapacities"): {},
}

func GetTableConvertor(gr schema.GroupResource) rest.TableConvertor {
	if !scheme.LegacyResourceScheme.IsGroupRegistered(gr.Group) {
		return printers.NewDefaultTableConvertor(gr)
	}

	if _, ok := legacyResourcesWithDefaultTableConvertor[gr]; ok {
		return printers.NewDefaultTableConvertor(gr)
	}

	return printerstorage.TableConvertor{TableGenerator: printers.NewClusterTableGenerator().With(printersinternal.AddHandlers)}
}

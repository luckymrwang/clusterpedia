package kubeapiserver

import (
	"sync"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers"
)

type RESTManager struct {
	serializer                 runtime.NegotiatedSerializer
	equivalentResourceRegistry runtime.EquivalentResourceMapper

	lock      sync.Mutex
	groups    atomic.Value // map[string]metav1.APIGroup
	resources atomic.Value // map[schema.GroupResource]metav1.APIResource

	restResourceInfos atomic.Value // map[schema.GroupVersionResource]RESTResourceInfo

	requestVerbs metav1.Verbs
}

func (m *RESTManager) GetRESTResourceInfo(gvr schema.GroupVersionResource) RESTResourceInfo {
	infos := m.restResourceInfos.Load().(map[schema.GroupVersionResource]RESTResourceInfo)
	return infos[gvr]
}

type RESTResourceInfo struct {
	APIResource  metav1.APIResource
	RequestScope *handlers.RequestScope
}

func (info RESTResourceInfo) Empty() bool {
	return info.APIResource.Name == "" || info.RequestScope == nil
}

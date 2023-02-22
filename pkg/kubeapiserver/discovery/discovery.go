package discovery

import (
	"net/http"
	"sync"
	"sync/atomic"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/negotiation"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"

	"github.com/clusterpedia-io/clusterpedia/pkg/utils/request"
)

type APIGroupSource interface {
	GetAPIGroups() map[string]metav1.APIGroup
}

type ResourceDiscoveryAPI struct {
	Group    string
	Resource metav1.APIResource
	Versions map[schema.GroupVersion]struct{}
}

// DiscoveryManager  管理集群的 discovery api，并处理 /api 和 /apis 的请求
type DiscoveryManager struct {
	// groupSource 用来保证所有集群的 API Group 保持一致
	groupSource APIGroupSource

	serializer                       runtime.NegotiatedSerializer
	stripVersionNegotiatedSerializer stripVersionNegotiatedSerializer

	discoveryLock  sync.Mutex
	groupHandler   *clusterGroupDiscoveryHandler
	versionHandler *clusterVersionDiscoveryHandler

	// groups 保存了所有集群支持的 API Group
	// clusterGroups 保存了每个集群的 API Group
	apigroups        atomic.Value // type: []metav1.APIGroup
	clusterAPIGroups atomic.Value // type: map[string][]metav1.APIGroup

	delegate http.Handler
}

// NewDiscoveryManager return a new instance of DiscoveryManager
func NewDiscoveryManager(serializer runtime.NegotiatedSerializer, groupSource APIGroupSource, delegate http.Handler) *DiscoveryManager {
	stripVersionNegotiatedSerializer := stripVersionNegotiatedSerializer{serializer}

	manager := &DiscoveryManager{
		serializer:                       serializer,
		stripVersionNegotiatedSerializer: stripVersionNegotiatedSerializer,

		groupSource: groupSource,
		delegate:    delegate,

		groupHandler: &clusterGroupDiscoveryHandler{
			serializer:                       serializer,
			stripVersionNegotiatedSerializer: stripVersionNegotiatedSerializer,
			delegate:                         delegate,
		},
		versionHandler: &clusterVersionDiscoveryHandler{
			serializer:                       serializer,
			stripVersionNegotiatedSerializer: stripVersionNegotiatedSerializer,
			delegate:                         delegate,
		},
	}

	manager.apigroups.Store([]metav1.APIGroup{})
	manager.clusterAPIGroups.Store(make(map[string][]metav1.APIGroup))

	// init group handler
	manager.groupHandler.handlers.Store(make(map[string]*groupDiscoveryHandler))
	manager.groupHandler.rebuildGlobalDiscoveryAPI(map[string]metav1.APIGroup{})

	// init version handler
	manager.versionHandler.handlers.Store(make(map[string]*versionDiscoveryHandler))
	manager.versionHandler.rebuildGlobalDiscoveryAPI()

	return manager
}

func (m *DiscoveryManager) ResourceEnabled(cluster string, gvr schema.GroupVersionResource) bool {
	var handler *versionDiscoveryHandler
	if cluster == "" {
		handler = m.versionHandler.global.Load().(*versionDiscoveryHandler)
	} else {
		handlers := m.versionHandler.handlers.Load().(map[string]*versionDiscoveryHandler)
		handler = handlers[cluster]
	}

	if handler == nil {
		return false
	}
	return handler.gvrs.Has(gvr.String())
}

func (m *DiscoveryManager) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	pathParts := splitPath(req.URL.Path)
	if len(pathParts) == 0 || len(pathParts) > 3 {
		m.delegate.ServeHTTP(w, req)
		return
	}

	prefix := pathParts[0]
	if prefix == "api" {
		m.handleLegacyAPI(pathParts, w, req)
		return
	}
	if prefix != "apis" {
		m.delegate.ServeHTTP(w, req)
		return
	}

	// match /apis
	if len(pathParts) == 1 {
		var apigroups []metav1.APIGroup
		if cluster := request.ClusterNameValue(req.Context()); cluster != "" {
			var ok bool
			clusterAPIGroups := m.clusterAPIGroups.Load().(map[string][]metav1.APIGroup)
			if apigroups, ok = clusterAPIGroups[cluster]; !ok {
				m.delegate.ServeHTTP(w, req)
				return
			}
		} else {
			apigroups = m.apigroups.Load().([]metav1.APIGroup)
		}

		responsewriters.WriteObjectNegotiated(m.serializer, negotiation.DefaultEndpointRestrictions, schema.GroupVersion{}, w, req, http.StatusOK, &metav1.APIGroupList{Groups: apigroups})
		return
	}

	// match /apis/<group>
	if len(pathParts) == 2 {
		m.groupHandler.ServeHTTP(w, req)
		return
	}

	// match /apis/<group>/<version>
	m.versionHandler.ServeHTTP(w, req)
}

func (m *DiscoveryManager) handleLegacyAPI(pathParts []string, w http.ResponseWriter, req *http.Request) {
	if len(pathParts) > 2 {
		m.delegate.ServeHTTP(w, req)
		return
	}

	// match /api
	if len(pathParts) == 1 {
		apiVersions := &metav1.APIVersions{
			Versions: []string{"v1"},
		}

		responsewriters.WriteObjectNegotiated(m.stripVersionNegotiatedSerializer, negotiation.DefaultEndpointRestrictions, schema.GroupVersion{}, w, req, http.StatusOK, apiVersions)
		return
	}

	// match /api/v1
	m.versionHandler.ServeHTTP(w, req)
}

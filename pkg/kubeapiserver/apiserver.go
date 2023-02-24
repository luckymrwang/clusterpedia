package kubeapiserver

import (
	"bytes"
	"fmt"
	"k8s.io/klog/v2"
	"net/http"
	rt "runtime"
	"time"

	"github.com/emicklei/go-restful/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	urlruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	genericrequest "k8s.io/apiserver/pkg/endpoints/request"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericfilters "k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/client-go/restmapper"

	"github.com/clusterpedia-io/clusterpedia/pkg/informers"
	resourcev1alpha3 "github.com/clusterpedia-io/clusterpedia/pkg/kapis/resources/v1alpha3"
	"github.com/clusterpedia-io/clusterpedia/pkg/utils/filters"
	"github.com/clusterpedia-io/clusterpedia/pkg/utils/iputil"
	"github.com/clusterpedia-io/clusterpedia/pkg/version"
)

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Group: "", Version: "v1"})
	Scheme.AddUnversionedTypes(schema.GroupVersion{Group: "", Version: "v1"},
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
		&metav1.WatchEvent{},
	)
}

func NewDefaultConfig() *Config {
	genericConfig := genericapiserver.NewRecommendedConfig(Codecs)

	genericConfig.APIServerID = ""
	genericConfig.EnableIndex = false
	genericConfig.EnableDiscovery = false
	genericConfig.EnableProfiling = false
	genericConfig.EnableMetrics = false
	genericConfig.BuildHandlerChainFunc = BuildHandlerChain
	genericConfig.HealthzChecks = []healthz.HealthChecker{healthz.PingHealthz}
	genericConfig.ReadyzChecks = []healthz.HealthChecker{healthz.PingHealthz}
	genericConfig.LivezChecks = []healthz.HealthChecker{healthz.PingHealthz}

	// disable genericapiserver's default post start hooks
	const maxInFlightFilterHookName = "max-in-flight-filter"
	genericConfig.DisabledPostStartHooks.Insert(maxInFlightFilterHookName)

	return &Config{GenericConfig: genericConfig}
}

type ExtraConfig struct {
	InitialAPIGroupResources []*restmapper.APIGroupResources

	InformerFactory informers.InformerFactory
}

type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig

	ExtraConfig ExtraConfig
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig

	ExtraConfig *ExtraConfig
}

type CompletedConfig struct {
	*completedConfig
}

func (c *Config) Complete() CompletedConfig {
	completed := &completedConfig{
		GenericConfig: c.GenericConfig.Complete(),
		ExtraConfig:   &c.ExtraConfig,
	}

	if c.GenericConfig.Version == nil {
		version := version.GetKubeVersion()
		c.GenericConfig.Version = &version
	}
	c.GenericConfig.RequestInfoResolver = wrapRequestInfoResolverForNamespace{
		c.GenericConfig.RequestInfoResolver,
	}
	return CompletedConfig{completed}
}

func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*genericapiserver.GenericAPIServer, error) {
	genericserver, err := c.GenericConfig.New("generic-apiserver", delegationTarget)
	if err != nil {
		return nil, err
	}

	delegate := delegationTarget.UnprotectedHandler()
	if delegate == nil {
		delegate = http.NotFoundHandler()
	}

	container := restful.NewContainer()
	container.Filter(logRequestAndResponse)
	container.Router(restful.CurlyRouter{})
	container.RecoverHandler(func(panicReason interface{}, httpWriter http.ResponseWriter) {
		logStackOnRecover(panicReason, httpWriter)
	})
	urlruntime.Must(resourcev1alpha3.AddToContainer(container, c.ExtraConfig.InformerFactory))

	//resourceHandler := &ResourceHandler{}
	genericserver.Handler.NonGoRestfulMux.HandlePrefix("/kapis/", container)

	return genericserver, nil
}

func BuildHandlerChain(apiHandler http.Handler, c *genericapiserver.Config) http.Handler {
	handler := genericapifilters.WithRequestInfo(apiHandler, c.RequestInfoResolver)
	handler = genericfilters.WithPanicRecovery(handler, c.RequestInfoResolver)

	// https://github.com/clusterpedia-io/clusterpedia/issues/54
	handler = filters.RemoveFieldSelectorFromRequest(handler)

	/* used for debugging
	handler = genericapifilters.WithWarningRecorder(handler)
	handler = WithClusterName(handler, "cluster-1")
	*/
	return handler
}

/* used for debugging
func WithClusterName(handler http.Handler, cluster string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		req = req.WithContext(request.WithClusterName(req.Context(), cluster))
		handler.ServeHTTP(w, req)
	})
}
*/

type wrapRequestInfoResolverForNamespace struct {
	genericrequest.RequestInfoResolver
}

func (r wrapRequestInfoResolverForNamespace) NewRequestInfo(req *http.Request) (*genericrequest.RequestInfo, error) {
	info, err := r.RequestInfoResolver.NewRequestInfo(req)
	if err != nil {
		return nil, err
	}

	if info.Resource == "namespaces" {
		info.Namespace = ""
	}
	return info, nil
}

func logStackOnRecover(panicReason interface{}, w http.ResponseWriter) {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("recover from panic situation: - %v\r\n", panicReason))
	for i := 2; ; i += 1 {
		_, file, line, ok := rt.Caller(i)
		if !ok {
			break
		}
		buffer.WriteString(fmt.Sprintf("    %s:%d\r\n", file, line))
	}
	klog.Errorln(buffer.String())

	headers := http.Header{}
	if ct := w.Header().Get("Content-Type"); len(ct) > 0 {
		headers.Set("Accept", ct)
	}

	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Internal server error"))
}

func logRequestAndResponse(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	start := time.Now()
	chain.ProcessFilter(req, resp)

	// Always log error response
	logWithVerbose := klog.V(4)
	if resp.StatusCode() > http.StatusBadRequest {
		logWithVerbose = klog.V(0)
	}

	logWithVerbose.Infof("%s - \"%s %s %s\" %d %d %dms",
		iputil.RemoteIp(req.Request),
		req.Request.Method,
		req.Request.URL,
		req.Request.Proto,
		resp.StatusCode(),
		resp.ContentLength(),
		time.Since(start)/time.Millisecond,
	)
}

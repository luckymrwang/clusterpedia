package kubeapiserver

import (
	"fmt"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	genericrequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/klog/v2"

	"github.com/clusterpedia-io/clusterpedia/pkg/utils/request"
)

type ResourceHandler struct {
	minRequestTimeout time.Duration
	delegate          http.Handler

	rest *RESTManager
}

func (r *ResourceHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	requestInfo, ok := genericrequest.RequestInfoFrom(req.Context())
	if !ok {
		responsewriters.ErrorNegotiated(
			apierrors.NewInternalError(fmt.Errorf("no RequestInfo found in the context")),
			Codecs, schema.GroupVersion{}, w, req,
		)
		return
	}

	gvr := schema.GroupVersionResource{Group: requestInfo.APIGroup, Version: requestInfo.APIVersion, Resource: requestInfo.Resource}
	// When clusterName not empty, first check cluster whether exist
	clusterName := request.ClusterNameValue(req.Context())

	info := r.rest.GetRESTResourceInfo(gvr)
	if info.Empty() {
		err := fmt.Errorf("not found request scope or resource storage")
		klog.ErrorS(err, "Failed to handle resource request", "resource", gvr)
		responsewriters.ErrorNegotiated(
			apierrors.NewInternalError(err),
			Codecs, gvr.GroupVersion(), w, req,
		)
		return
	}

	resource := info.APIResource
	if requestInfo.Namespace != "" && !resource.Namespaced {
		r.delegate.ServeHTTP(w, req)
		return
	}

	var handler http.Handler
	switch requestInfo.Verb {
	case "get":
		if clusterName == "" {
			responsewriters.ErrorNegotiated(
				apierrors.NewBadRequest("please specify the cluster name when using the resource name to get a specific resource."),
				Codecs, gvr.GroupVersion(), w, req,
			)
			return
		}

		//handler = handlers.GetResource(storage, reqScope)
	case "list":
		//handler = handlers.ListResource(storage, nil, reqScope, false, r.minRequestTimeout)
	case "watch":
		//handler = handlers.ListResource(storage, storage, reqScope, true, r.minRequestTimeout)
	default:
		responsewriters.ErrorNegotiated(
			apierrors.NewMethodNotSupported(gvr.GroupResource(), requestInfo.Verb),
			Codecs, gvr.GroupVersion(), w, req,
		)
	}

	if handler != nil {
		handler.ServeHTTP(w, req)
	}
}

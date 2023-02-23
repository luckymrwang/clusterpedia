/*
Copyright 2020 KubeAggregation Authors

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

package v1alpha3

import (
	"strings"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/klog"

	"github.com/clusterpedia-io/clusterpedia/pkg/api"
	"github.com/clusterpedia-io/clusterpedia/pkg/apiserver/params"
	"github.com/clusterpedia-io/clusterpedia/pkg/apiserver/query"
	"github.com/clusterpedia-io/clusterpedia/pkg/models/resources/v1alpha2"
	resourcev1alpha2 "github.com/clusterpedia-io/clusterpedia/pkg/models/resources/v1alpha2/resource"
	resourcev1alpha3 "github.com/clusterpedia-io/clusterpedia/pkg/models/resources/v1alpha3/resource"
)

type Handler struct {
	resourceGetterV1alpha3  *resourcev1alpha3.ResourceGetter
	resourcesGetterV1alpha2 *resourcev1alpha2.ResourceGetter
}

func New(resourceGetterV1alpha3 *resourcev1alpha3.ResourceGetter, resourcesGetterV1alpha2 *resourcev1alpha2.ResourceGetter) *Handler {
	return &Handler{
		resourceGetterV1alpha3:  resourceGetterV1alpha3,
		resourcesGetterV1alpha2: resourcesGetterV1alpha2,
	}
}

func (h *Handler) handleGetResources(request *restful.Request, response *restful.Response) {
	namespace := request.PathParameter("namespace")
	resourceType := request.PathParameter("resources")
	name := request.PathParameter("name")

	// use informers to retrieve resources
	result, err := h.resourceGetterV1alpha3.Get(resourceType, namespace, name)
	if err == nil {
		response.WriteEntity(result)
		return
	}

	if err != resourcev1alpha3.ErrResourceNotSupported {
		klog.Error(err, resourceType)
		api.HandleInternalError(response, nil, err)
		return
	}

	// fallback to v1alpha2
	resultV1alpha2, err := h.resourcesGetterV1alpha2.GetResource(namespace, resourceType, name)
	if err != nil {
		if err == resourcev1alpha2.ErrResourceNotSupported {
			api.HandleNotFound(response, request, err)
			return
		}
		klog.Error(err)
		api.HandleError(response, request, err)
		return
	}

	response.WriteEntity(resultV1alpha2)

}

// handleListResources retrieves resources
func (h *Handler) handleListResources(request *restful.Request, response *restful.Response) {
	query := query.ParseQueryParameter(request)
	resourceType := request.PathParameter("resources")
	namespace := request.PathParameter("namespace")

	result, err := h.resourceGetterV1alpha3.List(resourceType, namespace, query)
	if err == nil {
		response.WriteEntity(result)
		return
	}

	if err != resourcev1alpha3.ErrResourceNotSupported {
		klog.Error(err, resourceType)
		api.HandleInternalError(response, request, err)
		return
	}

	// fallback to v1alpha2
	result, err = h.fallback(resourceType, namespace, query)
	if err != nil {
		if err == resourcev1alpha2.ErrResourceNotSupported {
			api.HandleNotFound(response, request, err)
			return
		}
		klog.Error(err)
		api.HandleError(response, request, err)
		return
	}
	response.WriteEntity(result)
}

func (h *Handler) fallback(resourceType string, namespace string, q *query.Query) (*api.ListResult, error) {
	orderBy := string(q.SortBy)
	limit, offset := q.Pagination.Limit, q.Pagination.Offset
	reverse := !q.Ascending
	conditions := &params.Conditions{Match: make(map[string]string, 0), Fuzzy: make(map[string]string, 0)}
	for field, value := range q.Filters {
		switch field {
		case query.FieldName:
			conditions.Fuzzy[v1alpha2.Name] = string(value)
			break
		case query.FieldNames:
			conditions.Match[v1alpha2.Name] = string(value)
			break
		case query.FieldCreationTimeStamp:
			conditions.Match[v1alpha2.CreateTime] = string(value)
			break
		case query.FieldLastUpdateTimestamp:
			conditions.Match[v1alpha2.UpdateTime] = string(value)
			break
		case query.FieldLabel:
			values := strings.SplitN(string(value), ":", 2)
			if len(values) == 2 {
				conditions.Match[values[0]] = values[1]
			} else {
				conditions.Match[v1alpha2.Label] = values[0]
			}
			break
		case query.FieldAnnotation:
			values := strings.SplitN(string(value), ":", 2)
			if len(values) == 2 {
				conditions.Match[v1alpha2.Annotation] = values[1]
			} else {
				conditions.Match[v1alpha2.Annotation] = values[0]
			}
			break
		case query.FieldStatus:
			conditions.Match[v1alpha2.Status] = string(value)
			break
		case query.FieldOwnerReference:
			conditions.Match[v1alpha2.Owner] = string(value)
			break
		default:
			conditions.Match[string(field)] = string(value)
			break
		}
	}

	result, err := h.resourcesGetterV1alpha2.ListResources(namespace, resourceType, conditions, orderBy, reverse, limit, offset)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return &api.ListResult{
		Items:      result.Items,
		TotalItems: result.TotalCount,
	}, nil
}

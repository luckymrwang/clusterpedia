/*
Copyright 2019 The KubeAggregation Authors.

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
	"net/http"

	restfulspec "github.com/emicklei/go-restful-openapi"
	"github.com/emicklei/go-restful/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/clusterpedia-io/clusterpedia/pkg/api"
	"github.com/clusterpedia-io/clusterpedia/pkg/apiserver/query"
	"github.com/clusterpedia-io/clusterpedia/pkg/apiserver/runtime"
	"github.com/clusterpedia-io/clusterpedia/pkg/informers"
	resourcev1alpha2 "github.com/clusterpedia-io/clusterpedia/pkg/models/resources/v1alpha2/resource"
	resourcev1alpha3 "github.com/clusterpedia-io/clusterpedia/pkg/models/resources/v1alpha3/resource"
)

const (
	GroupName = ""

	tagClusteredResource  = "Clustered Resource"
	tagNamespacedResource = "Namespaced Resource"

	ok = "OK"
)

var GroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

func AddToContainer(c *restful.Container, informerFactory informers.InformerFactory) error {
	webservice := runtime.NewWebService(GroupVersion)
	handler := New(resourcev1alpha3.NewResourceGetter(informerFactory), resourcev1alpha2.NewResourceGetter(informerFactory))

	webservice.Route(webservice.GET("/{resources}").
		To(handler.handleListResources).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagClusteredResource}).
		Doc("Cluster level resources").
		Param(webservice.PathParameter("resources", "cluster level resource type, e.g. pods,jobs,configmaps,services.")).
		Param(webservice.QueryParameter(query.ParameterName, "name used to do filtering").Required(false)).
		Param(webservice.QueryParameter(query.ParameterPage, "page").Required(false).DataFormat("page=%d").DefaultValue("page=1")).
		Param(webservice.QueryParameter(query.ParameterLimit, "limit").Required(false)).
		Param(webservice.QueryParameter(query.ParameterAscending, "sort parameters, e.g. reverse=true").Required(false).DefaultValue("ascending=false")).
		Param(webservice.QueryParameter(query.ParameterOrderBy, "sort parameters, e.g. orderBy=createTime")).
		Returns(http.StatusOK, ok, api.ListResult{}))

	webservice.Route(webservice.GET("/{resources}/{name}").
		To(handler.handleGetResources).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagClusteredResource}).
		Doc("Cluster level resource").
		Param(webservice.PathParameter("resources", "cluster level resource type, e.g. pods,jobs,configmaps,services.")).
		Param(webservice.PathParameter("name", "the name of the clustered resources")).
		Returns(http.StatusOK, api.StatusOK, nil))

	webservice.Route(webservice.GET("/namespaces/{namespace}/{resources}").
		To(handler.handleListResources).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagNamespacedResource}).
		Doc("Namespace level resource query").
		Param(webservice.PathParameter("namespace", "the name of the project")).
		Param(webservice.PathParameter("resources", "namespace level resource type, e.g. pods,jobs,configmaps,services.")).
		Param(webservice.QueryParameter(query.ParameterName, "name used to do filtering").Required(false)).
		Param(webservice.QueryParameter(query.ParameterPage, "page").Required(false).DataFormat("page=%d").DefaultValue("page=1")).
		Param(webservice.QueryParameter(query.ParameterLimit, "limit").Required(false)).
		Param(webservice.QueryParameter(query.ParameterAscending, "sort parameters, e.g. reverse=true").Required(false).DefaultValue("ascending=false")).
		Param(webservice.QueryParameter(query.ParameterOrderBy, "sort parameters, e.g. orderBy=createTime")).
		Returns(http.StatusOK, ok, api.ListResult{}))

	webservice.Route(webservice.GET("/namespaces/{namespace}/{resources}/{name}").
		To(handler.handleGetResources).
		Metadata(restfulspec.KeyOpenAPITags, []string{tagNamespacedResource}).
		Doc("Namespace level get resource query").
		Param(webservice.PathParameter("namespace", "the name of the project")).
		Param(webservice.PathParameter("resources", "namespace level resource type, e.g. pods,jobs,configmaps,services.")).
		Param(webservice.PathParameter("name", "the name of resource")).
		Returns(http.StatusOK, ok, api.ListResult{}))

	c.Add(webservice)

	return nil
}

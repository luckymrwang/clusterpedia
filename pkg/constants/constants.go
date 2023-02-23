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

package constants

const (
	APIVersion = "v1alpha1"

	KubeSystemNamespace           = "kube-system"
	KubeSphereMonitoringNamespace = "kubesphere-monitoring-system"
	KubeSphereLoggingNamespace    = "kubesphere-logging-system"
	KubeSphereNamespace           = "kubesphere-system"
	PorterNamespace               = "porter-system"
	AdminUserName                 = "admin"
	KubeSphereConfigMapDataKey    = "kubesphere.yaml"

	WorkspaceLabelKey        = "kubesphere.io/workspace"
	NamespaceLabelKey        = "kubesphere.io/namespace"
	DisplayNameAnnotationKey = "kubesphere.io/alias-name"
	CreatorAnnotationKey     = "kubesphere.io/creator"

	UserNameHeader = "X-Token-Username"

	AuthenticationTag = "Authentication"
	UserTag           = "User"

	NamespaceResourcesTag = "Namespace Resources"
	ClusterResourcesTag   = "Cluster Resources"

	LogQueryTag = "Log Query"
)

var (
	SystemNamespaces = []string{KubeSphereNamespace, KubeSphereLoggingNamespace, KubeSphereMonitoringNamespace, KubeSystemNamespace, PorterNamespace}
)

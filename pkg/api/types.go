/*
Copyright 2020 The KubeAggregation Authors.

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

package api

import (
	corev1 "k8s.io/api/core/v1"
)

type ListResult struct {
	Items      []interface{} `json:"items"`
	TotalItems int           `json:"totalItems"`
}

type ResourceQuota struct {
	Namespace string                     `json:"namespace" description:"namespace"`
	Data      corev1.ResourceQuotaStatus `json:"data" description:"resource quota status"`
}

type NamespacedResourceQuota struct {
	Namespace string `json:"namespace,omitempty"`

	Data struct {
		corev1.ResourceQuotaStatus

		// quota left status, do the math on the side, cause it's
		// a lot easier with go-client library
		Left corev1.ResourceList `json:"left,omitempty"`
	} `json:"data,omitempty"`
}

const (
	StatusOK = "ok"
)

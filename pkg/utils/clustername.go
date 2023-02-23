package utils

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	internal "github.com/clusterpedia-io/api/clusterpedia"
)

func ExtractClusterName(obj runtime.Object) string {
	if m, err := meta.Accessor(obj); err == nil {
		if annotations := m.GetAnnotations(); annotations != nil {
			return annotations[internal.ShadowAnnotationClusterName]
		}
	}
	return ""
}

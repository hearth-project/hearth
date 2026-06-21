/*
Copyright 2026 The Hearth Authors.

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

package backend

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
)

const (
	serviceMonitorAPIVersion = "monitoring.coreos.com/v1"
	serviceMonitorKind       = "ServiceMonitor"
)

// BuildServiceMonitor renders a Prometheus Operator ServiceMonitor that scrapes both
// the gateway (queue depth, cold starts, request codes) and the backend vLLM
// (num_requests_waiting, gpu_cache_usage_perc, TTFT) over their shared "http" port at
// /metrics. Rendered as unstructured so we take no Prometheus Operator dependency.
func BuildServiceMonitor(svc *servingv1alpha1.LLMService) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(serviceMonitorAPIVersion)
	obj.SetKind(serviceMonitorKind)
	obj.SetName(svc.Name)
	obj.SetNamespace(svc.Namespace)
	obj.SetLabels(SelectorLabels(svc))
	obj.Object["spec"] = map[string]any{
		"selector": map[string]any{
			"matchLabels": map[string]any{llmServiceLabel: svc.Name},
		},
		"endpoints": []any{
			map[string]any{"port": portNameHTTP, "path": "/metrics", "interval": "15s"},
		},
	}
	return obj
}

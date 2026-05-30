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
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
)

const (
	kedaAPIVersion         = "keda.sh/v1alpha1"
	kedaKind               = "ScaledObject"
	defaultPollingInterval = 5 // seconds; the gateway hold masks this poll latency
	defaultMaxReplicas     = 1
	defaultTarget          = 10
)

// ScaledObjectName is the KEDA ScaledObject managing an LLMService's backend.
func ScaledObjectName(svc *servingv1alpha1.LLMService) string { return svc.Name }

// BuildScaledObject renders a KEDA ScaledObject that scales the backend Deployment
// (0..N, including scale-to-zero) on the gateway's pending-request count via the
// built-in metrics-api scaler. The always-on gateway is what KEDA polls.
//
// NOTE: this uses metrics-api (poll-based). A future optimization is a custom gRPC
// external scaler with StreamIsActive (push) to shave the polling interval off the
// first cold start; the gateway's request hold makes that latency invisible for now.
func BuildScaledObject(svc *servingv1alpha1.LLMService) *unstructured.Unstructured {
	target := svc.Spec.Scaling.Target
	if target <= 0 {
		target = defaultTarget
	}
	maxReplicas := svc.Spec.Scaling.Max
	if maxReplicas <= 0 {
		maxReplicas = defaultMaxReplicas
	}
	queueURL := fmt.Sprintf("http://%s.%s.svc/hearth/queue", GatewayServiceName(svc), svc.Namespace)

	spec := map[string]any{
		"scaleTargetRef":  map[string]any{"name": svc.Name},
		"minReplicaCount": int64(svc.Spec.Scaling.Min),
		"maxReplicaCount": int64(maxReplicas),
		"pollingInterval": int64(defaultPollingInterval),
		"triggers": []any{
			map[string]any{
				"type": "metrics-api",
				"metadata": map[string]any{
					"url":                   queueURL,
					"valueLocation":         "pending",
					"targetValue":           strconv.Itoa(int(target)),
					"activationTargetValue": "0", // any pending request wakes from zero
				},
			},
		},
	}
	if cooldown := int64(svc.Spec.Scaling.ScaleDownStabilization.Seconds()); cooldown > 0 {
		spec["cooldownPeriod"] = cooldown
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(kedaAPIVersion)
	obj.SetKind(kedaKind)
	obj.SetName(ScaledObjectName(svc))
	obj.SetNamespace(svc.Namespace)
	obj.SetLabels(SelectorLabels(svc))
	obj.Object["spec"] = spec
	return obj
}

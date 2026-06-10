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

// Package model resolves an LLMService model spec into a runtime-loadable model.
package model

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
)

// Resolve maps a model spec to a repo id plus the env needed to fetch it. v0 lets the
// runtime download weights at startup; local caching (PVC/hostPath) arrives in a later
// milestone, and catalogRef resolution along with it.
func Resolve(model servingv1alpha1.ModelSpec) (backend.ResolvedModel, error) {
	if model.Source == nil || model.Source.URI == "" {
		if model.CatalogRef != "" {
			return backend.ResolvedModel{}, fmt.Errorf("model.catalogRef resolution is not implemented yet; set model.source.uri")
		}
		return backend.ResolvedModel{}, fmt.Errorf("model.source.uri is required")
	}

	if model.Source.SecretRef != nil {
		return backend.ResolvedModel{}, fmt.Errorf("model.source.secretRef is not supported yet; use a public model source or remove it")
	}

	scheme, ref, ok := strings.Cut(model.Source.URI, "://")
	if !ok || ref == "" {
		return backend.ResolvedModel{}, fmt.Errorf("invalid model uri %q: expected scheme://reference", model.Source.URI)
	}

	switch scheme {
	case "hf", "huggingface":
		return backend.ResolvedModel{Path: ref, Source: "hf"}, nil
	case "modelscope":
		return backend.ResolvedModel{
			Path:   ref,
			Source: "modelscope",
			Env:    []corev1.EnvVar{{Name: "VLLM_USE_MODELSCOPE", Value: "true"}},
		}, nil
	default:
		return backend.ResolvedModel{}, fmt.Errorf("model uri scheme %q is not supported yet (use hf:// or modelscope://)", scheme)
	}
}

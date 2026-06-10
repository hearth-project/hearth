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

// Package backend defines the vendor-neutral abstraction that lets one LLMService
// run on any vLLM runtime (NVIDIA / Ascend / MLU). Adapters only do K8s-layer
// adaptation — scheduling, health, model loading, metrics — never chip kernels.
package backend

import (
	"bytes"
	"fmt"
	"text/template"

	corev1 "k8s.io/api/core/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
)

type ResolvedModel struct {
	Path string
	// Source ("hf" | "modelscope") selects the prewarm download command.
	Source string
	Env    []corev1.EnvVar
}

type AcceleratorRequest struct {
	Resources     corev1.ResourceList
	NodeSelector  map[string]string
	Tolerations   []corev1.Toleration
	SchedulerName string
	// Queue is the Volcano queue, rendered as a pod annotation (see BuildDeployment).
	Queue string
}

type MetricsSource struct {
	Path        string
	PortName    string
	QueueDepth  string
	KVCacheUtil string
}

// BackendAdapter renders the K8s artifacts for one vendor runtime.
type BackendAdapter interface {
	Vendor() string
	PodSpec(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime, m ResolvedModel) (corev1.PodSpec, error)
	Accelerator(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime) (AcceleratorRequest, error)
	MetricsSource(rt *servingv1alpha1.InferenceRuntime) MetricsSource
}

// Registry maps a vendor key to its adapter; new chips are new entries.
type Registry struct {
	adapters map[string]BackendAdapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: map[string]BackendAdapter{}}
}

func (r *Registry) Register(a BackendAdapter) {
	r.adapters[a.Vendor()] = a
}

func (r *Registry) Get(vendor string) (BackendAdapter, bool) {
	a, ok := r.adapters[vendor]
	return a, ok
}

// TemplateData is the context available to InferenceRuntime arg/env templates.
type TemplateData struct {
	Model       ModelData
	Service     ServiceData
	Accelerator AcceleratorData
}

type ModelData struct{ Path string }
type ServiceData struct{ Name, Namespace string }
type AcceleratorData struct{ Index string }

// Render expands a Go-template string against data, failing on unknown fields.
func Render(tmpl string, data TemplateData) (string, error) {
	t, err := template.New("tmpl").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", tmpl, err)
	}
	return buf.String(), nil
}

func RenderAll(tmpls []string, data TemplateData) ([]string, error) {
	out := make([]string, 0, len(tmpls))
	for _, s := range tmpls {
		r, err := Render(s, data)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

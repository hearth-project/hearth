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

// Package moorethreads is the backend adapter for Moore Threads (MUSA) GPUs running
// vLLM-MUSA (github.com/MooreThreads/vllm-musa).
//
// Like the nvidia adapter it is thin: the MT Container Toolkit (or HAMi) injects the
// MUSA driver libraries into the container, so no host driver mounts are needed — the
// adapter only requests whole devices and delegates rendering to the shared vLLM helper.
// It does K8s-layer adaptation only; chip kernels belong to vLLM-MUSA.
package moorethreads

import (
	corev1 "k8s.io/api/core/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
)

const Vendor = "moorethreads"

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

var _ backend.BackendAdapter = (*Adapter)(nil)

func (a *Adapter) Vendor() string { return Vendor }

func (a *Adapter) PodSpec(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime, m backend.ResolvedModel) (corev1.PodSpec, error) {
	return backend.RenderVLLMPodSpec(svc, rt, m)
}

func (a *Adapter) Accelerator(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime) (backend.AcceleratorRequest, error) {
	return backend.WholeDeviceAccelerator(svc, rt)
}

func (a *Adapter) MetricsSource(rt *servingv1alpha1.InferenceRuntime) backend.MetricsSource {
	return backend.MetricsFromRuntime(rt)
}

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

// Package ascend is the backend adapter for Huawei Ascend NPUs running vLLM-Ascend.
//
// Status (v0.2.0-rc.1): experimental preview. vLLM-Ascend has been run on a real 910B and the
// rendered manifests confirmed correct for it; the device-plugin scheduling e2e is still pending
// (the v1 "supported" milestone). The adapter does only K8s-layer adaptation (resource request +
// driver projection); chip kernels belong to vLLM-Ascend.
package ascend

import (
	corev1 "k8s.io/api/core/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
)

const Vendor = "ascend"

// hostDriverMounts are the standard Huawei CANN/driver paths the vLLM-Ascend container
// needs when the host driver is used directly. Drop these when the cluster runs the
// ascend-docker-runtime, which injects them automatically. These paths were confirmed present
// on a real 910B (CANN 9.0.0 / driver 26.0.rc1).
var hostDriverMounts = []backend.HostMount{
	{Name: "ascend-driver", Path: "/usr/local/Ascend/driver"},
	{Name: "npu-smi", Path: "/usr/local/bin/npu-smi"},
	{Name: "dcmi", Path: "/usr/local/dcmi"},
	{Name: "ascend-install-info", Path: "/etc/ascend_install.info"},
}

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

var _ backend.BackendAdapter = (*Adapter)(nil)

func (a *Adapter) Vendor() string { return Vendor }

func (a *Adapter) PodSpec(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime, m backend.ResolvedModel) (corev1.PodSpec, error) {
	pod, err := backend.RenderVLLMPodSpec(svc, rt, m)
	if err != nil {
		return corev1.PodSpec{}, err
	}
	backend.AddHostMounts(&pod, hostDriverMounts)
	return pod, nil
}

func (a *Adapter) Accelerator(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime) (backend.AcceleratorRequest, error) {
	return backend.WholeDeviceAccelerator(svc, rt)
}

func (a *Adapter) MetricsSource(rt *servingv1alpha1.InferenceRuntime) backend.MetricsSource {
	return backend.MetricsFromRuntime(rt)
}

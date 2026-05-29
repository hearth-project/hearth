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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InferenceRuntimeSpec defines a pluggable inference backend (e.g. NVIDIA-vLLM,
// vLLM-Ascend, vLLM-MLU). It adapts a vendor runtime to Kubernetes — scheduling,
// health, model loading and metrics — without ever touching chip kernels.
type InferenceRuntimeSpec struct {
	// family is the engine family this runtime belongs to (e.g. "vllm").
	// +kubebuilder:default=vllm
	Family string `json:"family"`

	// vendor identifies the accelerator vendor this runtime targets.
	// +kubebuilder:validation:Enum=nvidia;ascend;cambricon;hygon
	Vendor string `json:"vendor"`

	// priority breaks ties when several runtimes match an LLMService selector
	// (higher wins).
	// +kubebuilder:default=0
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// container is the serving container; it must expose the OpenAI API and /metrics.
	Container RuntimeContainer `json:"container"`

	// accelerator maps this runtime onto a device-plugin resource and scheduling.
	Accelerator AcceleratorSpec `json:"accelerator"`

	// health configures model-load-aware probes.
	// +optional
	Health RuntimeHealth `json:"health,omitempty"`

	// lifecycle controls graceful drain of in-flight streams on scale-down.
	// +optional
	Lifecycle RuntimeLifecycle `json:"lifecycle,omitempty"`

	// metrics tells the Hearth Scaler where the LLM-aware signals live.
	Metrics RuntimeMetrics `json:"metrics"`
}

// RuntimeContainer describes the serving container image and its single port.
type RuntimeContainer struct {
	// image is the serving container image.
	Image string `json:"image"`

	// args are Go-templated and rendered by the operator with model, service and
	// accelerator context before being applied to the pod.
	// +optional
	Args []string `json:"args,omitempty"`

	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// port is the single port serving both the OpenAI API and /metrics.
	Port RuntimePort `json:"port"`
}

// RuntimePort is the named container port for API + metrics traffic.
type RuntimePort struct {
	// +kubebuilder:default=http
	Name string `json:"name"`

	// +kubebuilder:default=8000
	ContainerPort int32 `json:"containerPort"`
}

// AcceleratorSpec adapts the abstract accelerator request to a concrete vendor
// device-plugin resource and scheduling, reusing (never replacing) HAMi/Volcano.
type AcceleratorSpec struct {
	// resourceName is the device-plugin resource key (e.g. nvidia.com/gpu,
	// huawei.com/Ascend910). Configurable because it varies by vendor and version.
	ResourceName string `json:"resourceName"`

	// sharing declares whether this runtime supports fractional devices
	// (e.g. NVIDIA via HAMi).
	// +optional
	Sharing AcceleratorSharing `json:"sharing,omitempty"`

	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// scheduler routes pods to an existing scheduler (e.g. Volcano).
	// +optional
	Scheduler RuntimeScheduler `json:"scheduler,omitempty"`
}

// AcceleratorSharing declares fractional-device support.
type AcceleratorSharing struct {
	// +optional
	Supported bool `json:"supported,omitempty"`
}

// RuntimeScheduler selects an existing scheduler and queue.
type RuntimeScheduler struct {
	// name is the scheduler name; empty uses the default scheduler.
	// +optional
	Name string `json:"name,omitempty"`

	// +optional
	Queue string `json:"queue,omitempty"`
}

// RuntimeHealth holds model-load-aware probes. startup gives slow weight loads
// minutes of headroom before liveness can kill the pod.
type RuntimeHealth struct {
	// +optional
	Readiness *corev1.Probe `json:"readiness,omitempty"`

	// +optional
	Liveness *corev1.Probe `json:"liveness,omitempty"`

	// +optional
	Startup *corev1.Probe `json:"startup,omitempty"`
}

// RuntimeLifecycle controls graceful termination of in-flight streams.
type RuntimeLifecycle struct {
	// terminationGracePeriodSeconds must cover the longest in-flight stream.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// preStopDrain stops routing new requests and waits for in-flight streams
	// to finish before the pod is torn down.
	// +optional
	PreStopDrain bool `json:"preStopDrain,omitempty"`
}

// RuntimeMetrics maps logical scaling signals to the runtime's Prometheus metric
// names. vLLM serves /metrics on the same port as the API.
type RuntimeMetrics struct {
	// +kubebuilder:default=/metrics
	Path string `json:"path"`

	// port is the metrics port name.
	// +kubebuilder:default=http
	Port string `json:"port"`

	// queueDepth is the pending-requests metric used for scale-to-zero and scaling.
	QueueDepth string `json:"queueDepth"`

	// +optional
	KVCacheUtil string `json:"kvCacheUtil,omitempty"`

	// +optional
	Running string `json:"running,omitempty"`

	// +optional
	TTFT string `json:"ttft,omitempty"`
}

// InferenceRuntimeStatus defines the observed state of InferenceRuntime.
type InferenceRuntimeStatus struct {
	// conditions represent the current state of the InferenceRuntime resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Family",type=string,JSONPath=".spec.family"
// +kubebuilder:printcolumn:name="Vendor",type=string,JSONPath=".spec.vendor"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=".spec.container.image",priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// InferenceRuntime is a cluster-scoped, reusable backend driver for one vendor runtime.
type InferenceRuntime struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of InferenceRuntime
	// +required
	Spec InferenceRuntimeSpec `json:"spec"`

	// status defines the observed state of InferenceRuntime
	// +optional
	Status InferenceRuntimeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// InferenceRuntimeList contains a list of InferenceRuntime
type InferenceRuntimeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []InferenceRuntime `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InferenceRuntime{}, &InferenceRuntimeList{})
}

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LLMServiceSpec is the declarative, one-click description of a served model:
// what to run, on which backend, how to scale (including to zero), and how to cache.
type LLMServiceSpec struct {
	// model selects the model to serve.
	Model ModelSpec `json:"model"`

	// runtime selects the backend — pinned by name or auto-picked by vendor preference.
	// +optional
	Runtime RuntimeSelection `json:"runtime,omitempty"`

	// resources declares the accelerator / CPU / memory request.
	// +optional
	Resources ResourceSpec `json:"resources,omitempty"`

	// scaling configures scale-to-zero and queue-driven autoscaling.
	// +optional
	Scaling ScalingSpec `json:"scaling,omitempty"`

	// cache configures model-weight caching — the key to usable scale-to-zero.
	// +optional
	Cache CacheSpec `json:"cache,omitempty"`

	// endpoint configures the served API and cold-start client behavior.
	// +optional
	Endpoint EndpointSpec `json:"endpoint,omitempty"`
}

// ModelSpec resolves a model either from a catalog entry or an explicit source.
type ModelSpec struct {
	// catalogRef resolves source and defaults from a model catalog.
	// +optional
	CatalogRef string `json:"catalogRef,omitempty"`

	// source specifies the model location explicitly.
	// +optional
	Source *ModelSource `json:"source,omitempty"`
}

// ModelSource points at model weights via a scheme-prefixed URI.
type ModelSource struct {
	// uri is the model location: hf:// | modelscope:// | oci:// | s3:// | pvc://
	URI string `json:"uri"`

	// secretRef holds credentials for private sources (e.g. a ModelScope token).
	// +optional
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// RuntimeSelection chooses the backend for an LLMService.
type RuntimeSelection struct {
	// name pins a specific InferenceRuntime.
	// +optional
	Name string `json:"name,omitempty"`

	// selector auto-picks among runtimes by vendor preference order.
	// +optional
	Selector *RuntimeSelector `json:"selector,omitempty"`

	// argsOverride appends to / overrides the runtime's serving args.
	// +optional
	ArgsOverride []string `json:"argsOverride,omitempty"`
}

// RuntimeSelector constrains automatic backend selection.
type RuntimeSelector struct {
	// vendor lists acceptable vendors in preference order.
	// +optional
	Vendor []string `json:"vendor,omitempty"`
}

// ResourceSpec is the abstract accelerator request; it maps onto the runtime's
// accelerator definition at reconcile time.
type ResourceSpec struct {
	// accelerators is the number of whole devices to request.
	// +kubebuilder:default=1
	// +optional
	Accelerators int32 `json:"accelerators,omitempty"`

	// fraction requests a sub-device slice; valid only when the runtime supports sharing.
	// +optional
	Fraction *AcceleratorFraction `json:"fraction,omitempty"`

	// +optional
	CPU *resource.Quantity `json:"cpu,omitempty"`

	// +optional
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// AcceleratorFraction requests a fraction of a device (e.g. NVIDIA via HAMi).
type AcceleratorFraction struct {
	// +optional
	Memory *resource.Quantity `json:"memory,omitempty"`

	// +optional
	Cores int32 `json:"cores,omitempty"`
}

// ScalingSpec configures KEDA-driven autoscaling. Scaling is intentionally limited
// to LLM-aware signals: CPU and raw RPS are not supported.
type ScalingSpec struct {
	// min replicas; 0 enables scale-to-zero.
	// +kubebuilder:default=0
	// +optional
	Min int32 `json:"min,omitempty"`

	// +kubebuilder:default=1
	// +optional
	Max int32 `json:"max,omitempty"`

	// metric drives autoscaling.
	// +kubebuilder:validation:Enum=queueDepth;kvCacheUtil
	// +kubebuilder:default=queueDepth
	// +optional
	Metric string `json:"metric,omitempty"`

	// target is the desired metric value per replica.
	// +kubebuilder:default=10
	// +optional
	Target int32 `json:"target,omitempty"`

	// activationTimeout is how long the gateway buffers a request during cold start.
	// +kubebuilder:default="5m"
	// +optional
	ActivationTimeout metav1.Duration `json:"activationTimeout,omitempty"`

	// +kubebuilder:default="5m"
	// +optional
	ScaleDownStabilization metav1.Duration `json:"scaleDownStabilization,omitempty"`

	// drainTimeout must be <= the runtime's terminationGracePeriodSeconds.
	// +kubebuilder:default="2m"
	// +optional
	DrainTimeout metav1.Duration `json:"drainTimeout,omitempty"`
}

// CacheSpec selects how model weights are cached so cold pods load from local disk
// instead of re-downloading on every scale-from-zero.
type CacheSpec struct {
	// +kubebuilder:validation:Enum=NodeLocalPVC;HostPath;SharedPVC;BakedImage;None
	// +kubebuilder:default=NodeLocalPVC
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// +optional
	Size *resource.Quantity `json:"size,omitempty"`

	// prewarm hydrates weights into the cache before first traffic.
	// +optional
	Prewarm bool `json:"prewarm,omitempty"`
}

// EndpointSpec configures the served API surface and cold-start client behavior.
type EndpointSpec struct {
	// +kubebuilder:default=true
	// +optional
	OpenAICompatible bool `json:"openAICompatible,omitempty"`

	// +optional
	ColdStart ColdStartSpec `json:"coldStart,omitempty"`
}

// ColdStartSpec controls what a client experiences while a model scales from zero.
type ColdStartSpec struct {
	// mode is how cold requests are handled: keepalive (SSE heartbeats hold the
	// connection) or reject (fast 503 + Retry-After for the client to retry).
	// +kubebuilder:validation:Enum=keepalive;reject
	// +kubebuilder:default=keepalive
	// +optional
	Mode string `json:"mode,omitempty"`

	// +kubebuilder:default="10s"
	// +optional
	HeartbeatInterval metav1.Duration `json:"heartbeatInterval,omitempty"`
}

// LLMServicePhase is a high-level summary of the service lifecycle.
// +kubebuilder:validation:Enum=Pending;Loading;Ready;ScaledToZero;Degraded
type LLMServicePhase string

const (
	PhasePending      LLMServicePhase = "Pending"
	PhaseLoading      LLMServicePhase = "Loading"
	PhaseReady        LLMServicePhase = "Ready"
	PhaseScaledToZero LLMServicePhase = "ScaledToZero"
	PhaseDegraded     LLMServicePhase = "Degraded"
)

// LLMServiceStatus defines the observed state of LLMService.
type LLMServiceStatus struct {
	// phase is a high-level summary of the service state.
	// +optional
	Phase LLMServicePhase `json:"phase,omitempty"`

	// resolvedRuntime is the InferenceRuntime actually selected.
	// +optional
	ResolvedRuntime string `json:"resolvedRuntime,omitempty"`

	// replicas is the current number of serving pods.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// endpointURL is the OpenAI-compatible base URL clients should call.
	// +optional
	EndpointURL string `json:"endpointURL,omitempty"`

	// conditions represent the current state of the LLMService resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Runtime",type=string,JSONPath=".status.resolvedRuntime"
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// LLMService is the Schema for the llmservices API
type LLMService struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of LLMService
	// +required
	Spec LLMServiceSpec `json:"spec"`

	// status defines the observed state of LLMService
	// +optional
	Status LLMServiceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LLMServiceList contains a list of LLMService
type LLMServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []LLMService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LLMService{}, &LLMServiceList{})
}

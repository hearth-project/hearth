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
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
)

// ServingContainerName is the name of the vLLM serving container.
const ServingContainerName = "serving"

const (
	managedByLabel  = "app.kubernetes.io/managed-by"
	managedByValue  = "hearth"
	nameLabel       = "app.kubernetes.io/name"
	llmServiceLabel = "serving.hearth.dev/llmservice"
	runtimeLabel    = "serving.hearth.dev/runtime"
)

// SelectorLabels are the immutable labels identifying one LLMService's pods.
func SelectorLabels(svc *servingv1alpha1.LLMService) map[string]string {
	return map[string]string{
		nameLabel:       svc.Name,
		managedByLabel:  managedByValue,
		llmServiceLabel: svc.Name,
	}
}

func podLabels(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime) map[string]string {
	l := SelectorLabels(svc)
	l[runtimeLabel] = rt.Name
	return l
}

// DesiredReplicas is the replica count the operator manages directly. Scale-to-zero
// (min=0) is owned by KEDA in a later milestone; until then a single manual replica
// keeps the service reachable.
func DesiredReplicas(svc *servingv1alpha1.LLMService) int32 {
	if svc.Spec.Scaling.Min > 0 {
		return svc.Spec.Scaling.Min
	}
	return 1
}

// BuildDeployment assembles the vLLM Deployment for an LLMService, using the vendor
// adapter for the pod spec and accelerator request.
func BuildDeployment(a BackendAdapter, svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime, m ResolvedModel) (*appsv1.Deployment, error) {
	pod, err := a.PodSpec(svc, rt, m)
	if err != nil {
		return nil, err
	}
	accel, err := a.Accelerator(svc, rt)
	if err != nil {
		return nil, err
	}
	applyAccelerator(&pod, accel)

	art, err := planCache(svc)
	if err != nil {
		return nil, err
	}
	applyCache(&pod, art)

	replicas := DesiredReplicas(svc)
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Labels:    podLabels(svc, rt),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: SelectorLabels(svc)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels(svc, rt)},
				Spec:       pod,
			},
		},
	}
	return dep, nil
}

// applyAccelerator merges the accelerator request into the serving container and pod.
// Extended (device-plugin) resources are set as limits only; Kubernetes mirrors them
// into requests automatically.
func applyAccelerator(pod *corev1.PodSpec, accel AcceleratorRequest) {
	if len(accel.NodeSelector) > 0 {
		if pod.NodeSelector == nil {
			pod.NodeSelector = map[string]string{}
		}
		maps.Copy(pod.NodeSelector, accel.NodeSelector)
	}
	pod.Tolerations = append(pod.Tolerations, accel.Tolerations...)
	if accel.SchedulerName != "" {
		pod.SchedulerName = accel.SchedulerName
	}
	for i := range pod.Containers {
		if pod.Containers[i].Name != ServingContainerName {
			continue
		}
		if pod.Containers[i].Resources.Limits == nil {
			pod.Containers[i].Resources.Limits = corev1.ResourceList{}
		}
		maps.Copy(pod.Containers[i].Resources.Limits, accel.Resources)
	}
}

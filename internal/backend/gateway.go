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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/gateway"
)

const (
	gatewayPort = 8080
	// defaultGatewayReplicas is 1 for crisp scale-from-zero: KEDA's metrics-api polls a
	// single gateway replica's pending count, so >1 replica (each counting its own
	// pending) softens activation until an aggregating scaler lands. Accepts SPOF for v0.
	defaultGatewayReplicas = 1
	gatewayLabel           = "serving.hearth.dev/gateway"
	backendSvcSuffix       = "-backend"
)

func BackendServiceName(svc *servingv1alpha1.LLMService) string {
	return svc.Name + backendSvcSuffix
}

func GatewayServiceName(svc *servingv1alpha1.LLMService) string { return svc.Name }

func gatewaySelectorLabels(svc *servingv1alpha1.LLMService) map[string]string {
	return map[string]string{
		nameLabel:      svc.Name + "-gateway",
		managedByLabel: managedByValue,
		gatewayLabel:   svc.Name,
	}
}

// gatewayServiceLabels are the gateway Service's metadata labels: the selector labels
// plus the shared llmservice label so one ServiceMonitor selects gateway + backend.
func gatewayServiceLabels(svc *servingv1alpha1.LLMService) map[string]string {
	l := gatewaySelectorLabels(svc)
	l[llmServiceLabel] = svc.Name
	return l
}

func BuildBackendService(svc *servingv1alpha1.LLMService, rt *servingv1alpha1.InferenceRuntime) *corev1.Service {
	return &corev1.Service{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{Name: BackendServiceName(svc), Namespace: svc.Namespace, Labels: SelectorLabels(svc)},
		Spec: corev1.ServiceSpec{
			Selector: SelectorLabels(svc),
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString(rt.Spec.Container.Port.Name),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func BuildGatewayService(svc *servingv1alpha1.LLMService) *corev1.Service {
	return &corev1.Service{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{Name: GatewayServiceName(svc), Namespace: svc.Namespace, Labels: gatewayServiceLabels(svc)},
		Spec: corev1.ServiceSpec{
			Selector: gatewaySelectorLabels(svc),
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt(gatewayPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func BuildGatewayDeployment(svc *servingv1alpha1.LLMService, image string, replicas int32) *appsv1.Deployment {
	if replicas <= 0 {
		replicas = defaultGatewayReplicas
	}
	backendURL := fmt.Sprintf("http://%s.%s.svc:80", BackendServiceName(svc), svc.Namespace)
	labels := gatewaySelectorLabels(svc)

	env := []corev1.EnvVar{
		{Name: gateway.EnvBackendURL, Value: backendURL},
		{Name: gateway.EnvListenAddr, Value: fmt.Sprintf(":%d", gatewayPort)},
	}
	if at := svc.Spec.Scaling.ActivationTimeout.Duration; at > 0 {
		env = append(env, corev1.EnvVar{Name: gateway.EnvActivationTimeout, Value: at.String()})
	}
	if cs := svc.Spec.Endpoint.ColdStart; cs.Mode != "" {
		env = append(env, corev1.EnvVar{Name: gateway.EnvColdStartMode, Value: cs.Mode})
		if hb := cs.HeartbeatInterval.Duration; hb > 0 {
			env = append(env, corev1.EnvVar{Name: gateway.EnvHeartbeatInterval, Value: hb.String()})
		}
	}

	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(gatewayPort)},
	}}

	return &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: svc.Name + "-gateway", Namespace: svc.Namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:           "gateway",
						Image:          image,
						Env:            env,
						Ports:          []corev1.ContainerPort{{Name: "http", ContainerPort: gatewayPort, Protocol: corev1.ProtocolTCP}},
						ReadinessProbe: probe,
						LivenessProbe:  probe.DeepCopy(),
					}},
				},
			},
		},
	}
}

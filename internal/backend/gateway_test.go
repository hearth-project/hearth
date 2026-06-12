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

package backend_test

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
)

func gatewaySvc() *servingv1alpha1.LLMService {
	return &servingv1alpha1.LLMService{ObjectMeta: metav1.ObjectMeta{Name: "qwen3-8b", Namespace: "ai"}}
}

func TestGatewayReplicasDefaultAndOverride(t *testing.T) {
	g := NewWithT(t)

	// non-positive falls back to the default (1, for crisp scale-from-zero)
	dep := backend.BuildGatewayDeployment(gatewaySvc(), "img", 0)
	g.Expect(dep.Spec.Replicas).NotTo(BeNil())
	g.Expect(*dep.Spec.Replicas).To(Equal(int32(1)))

	// an explicit value is respected (HA opt-in)
	dep = backend.BuildGatewayDeployment(gatewaySvc(), "img", 3)
	g.Expect(*dep.Spec.Replicas).To(Equal(int32(3)))
}

func TestGatewayCarriesImagePullSecrets(t *testing.T) {
	g := NewWithT(t)
	svc := gatewaySvc()
	svc.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
	dep := backend.BuildGatewayDeployment(svc, "img", 1)
	g.Expect(dep.Spec.Template.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "regcred"}))
}

func TestGatewayPointsAtBackendService(t *testing.T) {
	g := NewWithT(t)
	dep := backend.BuildGatewayDeployment(gatewaySvc(), "img", 1)
	env := dep.Spec.Template.Spec.Containers[0].Env
	var backendURL string
	for _, e := range env {
		if e.Name == "HEARTH_BACKEND_URL" {
			backendURL = e.Value
		}
	}
	g.Expect(backendURL).To(Equal("http://qwen3-8b-backend.ai.svc:80"))
}

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
)

func TestWholeDeviceAcceleratorRejectsFraction(t *testing.T) {
	g := NewWithT(t)
	mem := resource.MustParse("12Gi")
	svc := &servingv1alpha1.LLMService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen3-8b", Namespace: "ai"},
		Spec: servingv1alpha1.LLMServiceSpec{
			Resources: servingv1alpha1.ResourceSpec{
				Fraction: &servingv1alpha1.AcceleratorFraction{Memory: &mem, Cores: 50},
			},
		},
	}
	rt := &servingv1alpha1.InferenceRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "vllm-nvidia"},
		Spec: servingv1alpha1.InferenceRuntimeSpec{
			Accelerator: servingv1alpha1.AcceleratorSpec{ResourceName: "nvidia.com/gpu"},
		},
	}
	_, err := backend.WholeDeviceAccelerator(svc, rt)
	g.Expect(err).To(MatchError(ContainSubstring("fraction")))
}

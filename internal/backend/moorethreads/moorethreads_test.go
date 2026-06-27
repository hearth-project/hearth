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

package moorethreads_test

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
	"github.com/hearth-project/hearth/internal/backend/moorethreads"
)

func musaRuntime() *servingv1alpha1.InferenceRuntime {
	return &servingv1alpha1.InferenceRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "vllm-musa"},
		Spec: servingv1alpha1.InferenceRuntimeSpec{
			Family: "vllm",
			Vendor: "moorethreads",
			Container: servingv1alpha1.RuntimeContainer{
				Image: "mthreads/vllm-musa:v0.22.0",
				Args:  []string{"--model={{ .Model.Path }}", "--served-model-name={{ .Service.Name }}"},
				Port:  servingv1alpha1.RuntimePort{Name: "http", ContainerPort: 8000},
			},
			Accelerator: servingv1alpha1.AcceleratorSpec{
				ResourceName: "mthreads.com/vgpu",
				NodeSelector: map[string]string{"accelerator": "mtt-s5000"},
			},
		},
	}
}

func musaService() *servingv1alpha1.LLMService {
	return &servingv1alpha1.LLMService{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen3-8b", Namespace: "ai"},
		Spec: servingv1alpha1.LLMServiceSpec{
			Model:   servingv1alpha1.ModelSpec{Source: &servingv1alpha1.ModelSource{URI: "modelscope://Qwen/Qwen3-8B-Instruct"}},
			Runtime: servingv1alpha1.RuntimeSelection{Name: "vllm-musa"},
		},
	}
}

func resolved() backend.ResolvedModel {
	return backend.ResolvedModel{Path: "Qwen/Qwen3-8B-Instruct"}
}

// TestSameFrameworkRendersMooreThreads is the multi-backend proof: the same CRD + builder
// renders a correct MUSA pod requesting mthreads.com/vgpu, with zero Moore Threads hardware.
func TestSameFrameworkRendersMooreThreads(t *testing.T) {
	g := NewWithT(t)
	dep, err := backend.BuildDeployment(moorethreads.New(), musaService(), musaRuntime(), resolved())
	g.Expect(err).NotTo(HaveOccurred())

	c := dep.Spec.Template.Spec.Containers[0]
	g.Expect(c.Image).To(Equal("mthreads/vllm-musa:v0.22.0"))
	g.Expect(c.Args).To(ContainElement("--model=Qwen/Qwen3-8B-Instruct"))

	// the MUSA resource comes from the runtime, not adapter code
	g.Expect(c.Resources.Limits).To(HaveKey(corev1.ResourceName("mthreads.com/vgpu")))
	g.Expect(dep.Spec.Template.Spec.NodeSelector).To(HaveKeyWithValue("accelerator", "mtt-s5000"))
}

// TestNoHostDriverMounts confirms the MUSA adapter relies on the MT Container Toolkit /
// HAMi for driver injection: only the shared /dev/shm volume, none of the host driver
// mounts the Ascend adapter projects.
func TestNoHostDriverMounts(t *testing.T) {
	g := NewWithT(t)
	pod, err := moorethreads.New().PodSpec(musaService(), musaRuntime(), resolved())
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(pod.Volumes).To(HaveLen(1))
	g.Expect(pod.Volumes[0].Name).To(Equal("dshm"))
}

func TestVendor(t *testing.T) {
	g := NewWithT(t)
	g.Expect(moorethreads.New().Vendor()).To(Equal("moorethreads"))
}

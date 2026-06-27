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

package controller

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
)

func rt(name, vendor string, priority int32) servingv1alpha1.InferenceRuntime {
	return servingv1alpha1.InferenceRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       servingv1alpha1.InferenceRuntimeSpec{Vendor: vendor, Priority: priority},
	}
}

func TestPickByVendorPreferenceOrderBeatsPriority(t *testing.T) {
	g := NewWithT(t)
	items := []servingv1alpha1.InferenceRuntime{
		rt("vllm-nvidia", "nvidia", 100),
		rt("vllm-ascend", "ascend", 10),
	}
	got, err := pickByVendor(items, []string{"ascend", "nvidia"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Name).To(Equal("vllm-ascend"))
}

func TestPickByVendorPriorityTieBreak(t *testing.T) {
	g := NewWithT(t)
	items := []servingv1alpha1.InferenceRuntime{
		rt("vllm-ascend-alt", "ascend", 90),
		rt("vllm-ascend", "ascend", 100),
	}
	got, err := pickByVendor(items, []string{"ascend"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Name).To(Equal("vllm-ascend"))
}

func TestPickByVendorFallsThroughToNextVendor(t *testing.T) {
	g := NewWithT(t)
	items := []servingv1alpha1.InferenceRuntime{
		rt("vllm-nvidia", "nvidia", 0),
	}
	got, err := pickByVendor(items, []string{"cambricon", "nvidia"})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got.Name).To(Equal("vllm-nvidia"))
}

func TestPickByVendorNoMatch(t *testing.T) {
	g := NewWithT(t)
	items := []servingv1alpha1.InferenceRuntime{
		rt("vllm-nvidia", "nvidia", 0),
	}
	_, err := pickByVendor(items, []string{"ascend"})
	g.Expect(err).To(MatchError(ContainSubstring("ascend")))
}

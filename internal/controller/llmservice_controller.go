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
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	servingv1alpha1 "github.com/hearth-project/hearth/api/v1alpha1"
	"github.com/hearth-project/hearth/internal/backend"
	"github.com/hearth-project/hearth/internal/backend/registry"
	"github.com/hearth-project/hearth/internal/model"
)

const fieldOwner = client.FieldOwner("hearth-operator")

// LLMServiceReconciler reconciles a LLMService object
type LLMServiceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Backends *backend.Registry
	// GatewayImage is the data-plane proxy image the operator deploys per LLMService.
	GatewayImage string
}

// +kubebuilder:rbac:groups=serving.hearth.dev,resources=llmservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.hearth.dev,resources=llmservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.hearth.dev,resources=llmservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.hearth.dev,resources=inferenceruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile renders the vLLM Deployment and Service for an LLMService from its
// selected InferenceRuntime, then reflects the result in status.
func (r *LLMServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var svc servingv1alpha1.LLMService
	if err := r.Get(ctx, req.NamespacedName, &svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	rt, err := r.resolveRuntime(ctx, &svc)
	if err != nil {
		return r.fail(ctx, &svc, "RuntimeResolution", err)
	}

	adapter, ok := r.Backends.Get(rt.Spec.Vendor)
	if !ok {
		return r.fail(ctx, &svc, "UnsupportedVendor", fmt.Errorf("no backend adapter registered for vendor %q", rt.Spec.Vendor))
	}

	resolved, err := model.Resolve(svc.Spec.Model)
	if err != nil {
		return r.fail(ctx, &svc, "ModelResolution", err)
	}

	dep, err := backend.BuildDeployment(adapter, &svc, rt, resolved)
	if err != nil {
		return r.fail(ctx, &svc, "Render", err)
	}
	if err := r.apply(ctx, &svc, dep); err != nil {
		return r.fail(ctx, &svc, "ApplyDeployment", err)
	}
	if err := r.apply(ctx, &svc, backend.BuildBackendService(&svc, rt)); err != nil {
		return r.fail(ctx, &svc, "ApplyBackendService", err)
	}
	if err := r.apply(ctx, &svc, backend.BuildGatewayDeployment(&svc, r.GatewayImage)); err != nil {
		return r.fail(ctx, &svc, "ApplyGateway", err)
	}
	if err := r.apply(ctx, &svc, backend.BuildGatewayService(&svc)); err != nil {
		return r.fail(ctx, &svc, "ApplyGatewayService", err)
	}

	pvc, err := backend.BuildCachePVC(&svc)
	if err != nil {
		return r.fail(ctx, &svc, "Cache", err)
	}
	if pvc != nil {
		if err := r.ensureCreated(ctx, &svc, pvc); err != nil {
			return r.fail(ctx, &svc, "ApplyCachePVC", err)
		}
	}

	job, err := backend.BuildPrewarmJob(&svc, rt, resolved)
	if err != nil {
		return r.fail(ctx, &svc, "Prewarm", err)
	}
	if job != nil {
		if err := r.ensureCreated(ctx, &svc, job); err != nil {
			return r.fail(ctx, &svc, "ApplyPrewarmJob", err)
		}
	}

	var live appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKeyFromObject(dep), &live); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled LLMService", "runtime", rt.Name, "readyReplicas", live.Status.ReadyReplicas)
	return r.updateStatus(ctx, &svc, rt.Name, &live)
}

// resolveRuntime selects the backend: a pinned name, else the highest-priority runtime
// matching the vendor preference order.
func (r *LLMServiceReconciler) resolveRuntime(ctx context.Context, svc *servingv1alpha1.LLMService) (*servingv1alpha1.InferenceRuntime, error) {
	sel := svc.Spec.Runtime
	if sel.Name != "" {
		var rt servingv1alpha1.InferenceRuntime
		if err := r.Get(ctx, client.ObjectKey{Name: sel.Name}, &rt); err != nil {
			return nil, fmt.Errorf("get InferenceRuntime %q: %w", sel.Name, err)
		}
		return &rt, nil
	}
	if sel.Selector != nil && len(sel.Selector.Vendor) > 0 {
		var list servingv1alpha1.InferenceRuntimeList
		if err := r.List(ctx, &list); err != nil {
			return nil, err
		}
		return pickByVendor(list.Items, sel.Selector.Vendor)
	}
	return nil, fmt.Errorf("spec.runtime: set either name or selector.vendor")
}

func pickByVendor(items []servingv1alpha1.InferenceRuntime, vendors []string) (*servingv1alpha1.InferenceRuntime, error) {
	for _, v := range vendors {
		var best *servingv1alpha1.InferenceRuntime
		for i := range items {
			if items[i].Spec.Vendor != v {
				continue
			}
			if best == nil || items[i].Spec.Priority > best.Spec.Priority {
				best = &items[i]
			}
		}
		if best != nil {
			return best, nil
		}
	}
	return nil, fmt.Errorf("no InferenceRuntime matches vendors %v", vendors)
}

// apply server-side applies an owned object so the API server keeps ownership of fields
// Hearth does not set (defaults, clusterIP, etc.), giving idempotent reconciliation.
func (r *LLMServiceReconciler) apply(ctx context.Context, owner *servingv1alpha1.LLMService, obj client.Object) error {
	if err := controllerutil.SetControllerReference(owner, obj, r.Scheme); err != nil {
		return err
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	ac := client.ApplyConfigurationFromUnstructured(&unstructured.Unstructured{Object: u})
	return r.Apply(ctx, ac, fieldOwner, client.ForceOwnership)
}

// ensureCreated creates an owned object once and ignores AlreadyExists. Used for the
// cache PVC and prewarm Job, which have immutable fields and are never updated in place.
func (r *LLMServiceReconciler) ensureCreated(ctx context.Context, owner *servingv1alpha1.LLMService, obj client.Object) error {
	if err := controllerutil.SetControllerReference(owner, obj, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, obj); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *LLMServiceReconciler) updateStatus(ctx context.Context, svc *servingv1alpha1.LLMService, runtimeName string, dep *appsv1.Deployment) (ctrl.Result, error) {
	phase := servingv1alpha1.PhasePending
	switch {
	case dep.Status.ReadyReplicas > 0:
		phase = servingv1alpha1.PhaseReady
	case dep.Status.Replicas > 0:
		phase = servingv1alpha1.PhaseLoading
	}

	svc.Status.Phase = phase
	svc.Status.ResolvedRuntime = runtimeName
	svc.Status.Replicas = dep.Status.ReadyReplicas
	svc.Status.EndpointURL = fmt.Sprintf("http://%s.%s.svc/v1", svc.Name, svc.Namespace)

	cond := metav1.Condition{Type: "Ready", ObservedGeneration: svc.Generation}
	if phase == servingv1alpha1.PhaseReady {
		cond.Status, cond.Reason, cond.Message = metav1.ConditionTrue, "Available", "Serving pods are ready"
	} else {
		cond.Status, cond.Reason, cond.Message = metav1.ConditionFalse, "Deploying", "Waiting for serving pods to become ready"
	}
	meta.SetStatusCondition(&svc.Status.Conditions, cond)

	if err := r.Status().Update(ctx, svc); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *LLMServiceReconciler) fail(ctx context.Context, svc *servingv1alpha1.LLMService, reason string, err error) (ctrl.Result, error) {
	logf.FromContext(ctx).Error(err, "Failed to reconcile LLMService", "reason", reason)
	svc.Status.Phase = servingv1alpha1.PhaseDegraded
	meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		ObservedGeneration: svc.Generation,
	})
	if uerr := r.Status().Update(ctx, svc); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *LLMServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Backends == nil {
		r.Backends = registry.New()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1alpha1.LLMService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		Named("llmservice").
		Complete(r)
}

# Hearth

**Declarative, scale-to-zero serving for domestic open-source LLMs on your own Kubernetes —
vendor-neutral across NVIDIA, Ascend, and more.**

Hearth is a Kubernetes operator that turns "run Qwen / DeepSeek / GLM on my private cluster" into a
single `LLMService` manifest: declarative deploy, queue-driven autoscaling, and **scale-to-zero** —
with NVIDIA-vLLM / vLLM-Ascend / vLLM-MLU as **pluggable backends** behind one API.

> **Status — v0, early.** The NVIDIA backend is implemented and tested; the **Ascend** backend is
> scaffolded and golden-tested (renders correct manifests) but **not yet validated on real NPUs** —
> that's the v1 milestone (pending hardware). Scale-to-zero (gateway + KEDA) is in progress. Not
> production-ready. ⭐ and follow along.

## Why Hearth

The "deploy an LLM on K8s" space is crowded but **NVIDIA-first and English-first**. Hearth's design
center is different: **vendor-neutral, domestic-runtime-first orchestration**, with private/"XinChuang"
delivery as a first-class concern. You write the model and the scaling intent; Hearth picks the
backend, renders the workload, caches the weights, and scales it to zero when idle.

**What Hearth does — and deliberately does not do:**

| Layer | Owner | Hearth |
|---|---|---|
| Inference engine | vLLM (+ `vllm-ascend` / `vllm-mlu`) | **Uses it.** Never re-implements; writes no chip kernels. |
| GPU/NPU scheduling | device plugins, **HAMi**, **Volcano** | **Builds on.** Targets their resources; never replaces them. |
| Datacenter scale-out | **llm-d**, **KServe** | **Out of scope.** Hearth is the few-GPU, scale-to-zero, private end. |
| Declarative lifecycle + scale-to-zero + vendor-neutral packaging | — | **This is Hearth.** |

## A 60-second example

```yaml
apiVersion: serving.hearth.dev/v1alpha1
kind: LLMService
metadata:
  name: qwen3-8b
  namespace: ai
spec:
  model:
    source:
      uri: modelscope://Qwen/Qwen3-8B-Instruct   # hf:// | modelscope:// | oci:// | s3:// | pvc://
  runtime:
    selector: { vendor: [nvidia, ascend] }        # auto-pick a backend, in preference order
  resources:
    accelerators: 1
  scaling:
    min: 0            # scale-to-zero
    max: 3
    metric: queueDepth
    target: 10
```

```console
$ kubectl apply -f qwen3-8b.yaml
$ kubectl get llmservice -n ai
NAME       PHASE          RUNTIME       REPLICAS   AGE
qwen3-8b   ScaledToZero   vllm-nvidia   0          30s
```

The same manifest runs on an Ascend cluster by making `vllm-ascend` the available runtime — no spec
change. That portability is the whole point.

## Multi-backend, by design

Backends are described declaratively in a cluster-scoped `InferenceRuntime` (image, args, accelerator
resource, probes, metrics). Adapter **code** is thin because the differences are data:

| Backend | Engine | Accelerator | v0 status |
|---|---|---|---|
| `vllm-nvidia` | NVIDIA-vLLM | `nvidia.com/gpu` | ✅ implemented + tested |
| `vllm-ascend` | vLLM-Ascend | `huawei.com/Ascend910` | 🧪 scaffolded + golden-tested (HW validation in v1) |
| `vllm-mlu` (Cambricon) | vLLM-MLU | `cambricon.com/mlu` | 🗺️ planned |

Adding a chip is a small adapter, not a rewrite — see [`internal/backend`](internal/backend).

## Install

Hearth needs **KEDA** for scale-to-zero (and optionally the **Prometheus Operator** for the
ServiceMonitor + dashboard — Hearth degrades gracefully without it).

```bash
# KEDA (required for autoscaling / scale-to-zero)
helm repo add kedacore https://kedacore.github.io/charts
helm install keda kedacore/keda -n keda --create-namespace

# Hearth operator (CRDs + RBAC + controller)
helm install hearth ./charts/hearth -n hearth-system --create-namespace
```

Then register a backend and deploy a model:

```bash
kubectl apply -f config/samples/serving_v1alpha1_inferenceruntime.yaml
kubectl apply -f config/samples/serving_v1alpha1_llmservice.yaml
kubectl get llmservice -w
```

## Quickstart (kind, no GPU required to try the control plane)

```bash
# 1. install the CRDs
make install

# 2. run the operator against your current kube context
make run

# 3. register a backend + a service
kubectl create namespace ai
kubectl apply -f config/samples/serving_v1alpha1_inferenceruntime.yaml
kubectl apply -f config/samples/serving_v1alpha1_llmservice.yaml -n ai

# 4. watch it reconcile (pod stays Pending without a GPU — expected)
kubectl get llmservice,deploy,svc -n ai
```

To actually serve tokens you need an NVIDIA GPU node with the device plugin; see `docs/` (coming
soon) for the spot-GPU walkthrough.

## Architecture (short)

`LLMService` (what to serve + how to scale) + `InferenceRuntime` (a pluggable backend) → the operator
renders a vLLM `Deployment` + `Service`, a model cache, and a KEDA `ScaledObject` whose external
scaler is a small Hearth gateway that buffers requests during cold start. Full design, CRD reference,
and the scale-to-zero data flow live in [`docs/`](docs) and the project proposal.

## Roadmap

- **v0** — multi-backend abstraction on NVIDIA; model caching; gateway + KEDA scale-to-zero; Helm + dashboard.
- **v1** — Ascend running on real NPUs; HAMi/Volcano integration; curated domestic-model catalog.
- **v2** — Cambricon/Hygon; LoRA; air-gapped "XinChuang" offline bundle.

## Contributing & License

Early-stage and moving fast — issues and ideas welcome. Licensed under **Apache-2.0**.

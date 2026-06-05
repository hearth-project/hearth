<div align="center">

# 🔥 Hearth

**Declarative, scale-to-zero serving for domestic open-source LLMs on your own Kubernetes —
vendor-neutral across NVIDIA, Ascend, and more.**

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/hearth-project/hearth)](go.mod)
[![Release](https://img.shields.io/github/v/release/hearth-project/hearth?include_prereleases&label=release&sort=semver)](https://github.com/hearth-project/hearth/releases)
[![CI](https://github.com/hearth-project/hearth/actions/workflows/test.yml/badge.svg)](https://github.com/hearth-project/hearth/actions/workflows/test.yml)
[![Status: alpha](https://img.shields.io/badge/status-alpha-orange.svg)](ROADMAP.md)

[**Quickstart**](#quickstart) · [**Architecture**](docs/architecture.md) · [**Observability**](docs/observability.md) · [**Roadmap**](ROADMAP.md) · [**Contributing**](CONTRIBUTING.md)

</div>

Hearth is a Kubernetes operator that turns "run Qwen / DeepSeek / GLM on my private cluster" into a
single `LLMService` manifest: declarative deploy, queue-driven autoscaling, and **scale-to-zero** —
with NVIDIA-vLLM / vLLM-Ascend / vLLM-MLU as **pluggable backends** behind one API.

> **Status — pre-release `v0.1.0` (alpha).** The NVIDIA backend and the full scale-to-zero path
> (gateway + KEDA) are **implemented and verified end-to-end on real NVIDIA GPUs** — cold-start
> keepalive, graceful drain, model caching/prewarm, 1→N autoscaling, and observability. The
> **Ascend** backend is scaffolded and golden-tested (renders correct manifests) but **not yet
> validated on real NPUs** — the v1 milestone (pending hardware). Still `v1alpha1` and **not
> production-ready** (no auth, no multi-tenancy) — see the **[roadmap](ROADMAP.md)**. ⭐ and follow along.

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
change. **That portability is the whole point.**

## Multi-backend, by design

Backends are described declaratively in a cluster-scoped `InferenceRuntime` (image, args, accelerator
resource, probes, metrics). Adapter **code** is thin because the differences are data:

| Backend | Engine | Accelerator | v0 status |
|---|---|---|---|
| `vllm-nvidia` | NVIDIA-vLLM | `nvidia.com/gpu` | ✅ implemented + verified on GPU |
| `vllm-ascend` | vLLM-Ascend | `huawei.com/Ascend910` | 🧪 scaffolded + golden-tested (HW validation in v1) |
| `vllm-mlu` (Cambricon) | vLLM-MLU | `cambricon.com/mlu` | 🗺️ planned |

Adding a chip is a small adapter, not a rewrite — see [`internal/backend`](internal/backend).

## Quickstart

> Try the control plane on **kind — no GPU required**.

```bash
# 1. install the CRDs into your current kube-context
make install

# 2. run the operator against that context
make run

# 3. register a backend + a service
kubectl create namespace ai
kubectl apply -f config/samples/serving_v1alpha1_inferenceruntime.yaml
kubectl apply -f config/samples/serving_v1alpha1_llmservice.yaml -n ai

# 4. watch it reconcile (backend pod stays Pending without a GPU — expected)
kubectl get llmservice,deploy,svc -n ai
```

This exercises the control plane: the operator reconciles an `LLMService` into its child objects. The
gateway and backend pods start once you point the operator at a built gateway image
(`go run ./cmd/main.go --gateway-image=<your-registry>/hearth-gateway:v0.1.0`) and provide a GPU node
with the device plugin. A spot-GPU walkthrough is coming to [`docs/`](docs).

## Install

> **Pre-release (`v0.1.0`):** prebuilt images aren't published yet, so build and push your own and
> pass the tag to the chart. A tagged release will publish `ghcr.io/hearth-project/hearth` and
> `hearth-gateway`, after which the chart installs without the `--set` overrides.

Hearth needs **KEDA** for scale-to-zero (and optionally the **Prometheus Operator** for the
ServiceMonitor + dashboard — Hearth degrades gracefully without it).

```bash
# KEDA (required for autoscaling / scale-to-zero)
helm repo add kedacore https://kedacore.github.io/charts
helm install keda kedacore/keda -n keda --create-namespace

# Build + push the operator and gateway images (until a tagged release publishes them)
make docker-build         docker-push         IMG=<your-registry>/hearth:v0.1.0
make docker-build-gateway docker-push-gateway GATEWAY_IMG=<your-registry>/hearth-gateway:v0.1.0

# Hearth operator (CRDs + RBAC + controller)
helm install hearth ./charts/hearth -n hearth-system --create-namespace \
  --set image.registry=<your-registry> --set image.tag=v0.1.0

# register a backend and deploy a model
kubectl apply -f config/samples/serving_v1alpha1_inferenceruntime.yaml
kubectl apply -f config/samples/serving_v1alpha1_llmservice.yaml
kubectl get llmservice -w
```

## Architecture

`LLMService` (what to serve + how to scale) + `InferenceRuntime` (a pluggable backend) → the operator
renders a vLLM `Deployment` + `Service`, a model cache, and a KEDA `ScaledObject` whose external
scaler is a small Hearth gateway that buffers requests during cold start.

📖 See [`docs/architecture.md`](docs/architecture.md) for the components, CRDs, and the full
scale-to-zero data flow.

See [`docs/observability.md`](docs/observability.md) for the Grafana dashboard import steps and
gateway metric reference.

## Roadmap

See **[ROADMAP.md](ROADMAP.md)** for the prioritized path to production and what v0 is (and isn't) good for.

- **v0 — `v0.1.0` pre-release (now)** — multi-backend abstraction on NVIDIA, **verified end-to-end on
  real GPUs**: model caching/prewarm, gateway + KEDA scale-to-zero, cold-start keepalive, graceful
  drain, 1→N autoscaling, Helm + dashboard.
- **v1** — Ascend running on real NPUs; HAMi/Volcano integration; curated domestic-model catalog.
- **v2** — Cambricon/Hygon; LoRA; air-gapped "XinChuang" offline bundle.

> **Not production-ready yet** — no auth, no multi-tenancy, `v1alpha1` API. It's a strong fit today
> for **internal/dev, latency-tolerant, cost-sensitive** serving (scale-to-zero packs many idle models
> onto few GPUs). See the roadmap's production-readiness section before exposing it to real users.

## Contributing

Hearth is early and moving fast — contributions, issues, and ideas are very welcome, especially
**validating the Ascend backend on real NPUs** and the [roadmap](ROADMAP.md)'s P0/P1 items. Start with
**[CONTRIBUTING.md](CONTRIBUTING.md)** and please follow our [Code of Conduct](CODE_OF_CONDUCT.md).
To report a vulnerability, see [SECURITY.md](SECURITY.md).

## License

Licensed under [**Apache-2.0**](LICENSE).

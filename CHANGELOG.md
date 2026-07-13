# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to follow
[Semantic Versioning](https://semver.org/) once it reaches a stable release.

## [Unreleased]

### Added
- A shared Ascend hardware-validation guide covering required images, validation levels, evidence,
  and separate 910B, Atlas 300I Duo, and Atlas 300I Pro result sets.
- Validation bounds for accelerator counts, scaling values, runtime ports, and termination grace
  periods.

### Changed
- Prewarm Jobs inherit the runtime's node selector, tolerations, scheduler, and Volcano queue so
  node-local model data is prepared where the backend can run.
- `LLMService` resources are reconciled when a matching `InferenceRuntime` changes.

### Fixed
- Reject unsupported `BakedImage` cache requests instead of silently rendering them as uncached.
- Reject invalid PVC claim names and model paths that could escape the mounted model volume.

## [0.2.0-rc.1] - 2026-06-27

Pre-release documenting the first **real-hardware bring-up of the Ascend 910B backend**. vLLM-Ascend
now serves on a physical 910B, the operator's rendered manifests are confirmed correct for the 910
family, and the gateway data-plane is verified against the live NPU. The remaining gap — the operator
scheduling a backend pod onto an NPU via the device plugin (full integrated scale-to-zero e2e) — needs
a schedulable NPU node and stays open. Ascend support is therefore **experimental / technical preview**,
not yet "supported." Still `v1alpha1` and not production-ready.

### Added
- **Ascend 910B validation report + bring-up runbook** ([docs/ascend-910b-validation.md](docs/ascend-910b-validation.md))
  capturing the verified environment (910B2C 64 GB, CANN 9.0.0 / driver 26.0.rc1), the smoke test,
  the operator render dry-run, and the gateway data-plane results.

### Changed
- **Ascend runtime image** pinned to `quay.io/ascend/vllm-ascend:v0.21.0rc1` (was `v0.18.0`) — the
  base Atlas-A2/910B tag, matching the `vllm_ascend 0.21.0rc1` stack verified on real hardware.
- **Ascend status** updated from "scaffolded, not run on hardware" to **experimental / technical
  preview** across README, ROADMAP, and the adapter docs, reflecting what is now verified on a real
  910B (vLLM-Ascend serves; manifests render correctly; gateway data-plane works) versus what is not
  (device-plugin scheduling + full integrated e2e).

### Verified (Ascend 910B, real hardware)
- **vLLM-Ascend serves on the NPU** — Qwen2.5 loaded onto a 910B2C and answered via the OpenAI API
  (CANN 9.0.0 / driver 26.0.rc1, vllm-ascend 0.21.0rc1).
- **Operator renders a correct 910B backend** (kind dry-run) — `huawei.com/Ascend910` request, CANN
  driver host-mounts, ModelScope cache wiring, load-gated probes; vendor selector resolves to `vllm-ascend`.
- **Gateway data-plane on real NPU** — `/healthz`, `/hearth/queue` (incl. demand-linger), `/metrics`,
  OpenAI passthrough (streaming + non-streaming), and cold-start SSE keepalive → activation timeout → `503`.

### Not yet verified
- Operator → Ascend device plugin → backend pod **scheduled and serving on the NPU**, and the full
  integrated `0→1→N→0` loop on a real NPU node (the v1 "supported" milestone).

## [0.1.0] - 2026-06-06

First **release (alpha)**. The core thesis — declarative, scale-to-zero serving of self-hosted
open-source LLMs on Kubernetes — is implemented and verified end-to-end on real NVIDIA GPUs, and
the full loop now runs in CI with no hardware.
Not production-ready (see [ROADMAP.md](ROADMAP.md)).

### Added
- **CRDs** — `LLMService` (namespaced) and `InferenceRuntime` (cluster-scoped, pluggable backend),
  API group `serving.hearth.dev/v1alpha1`.
- **NVIDIA backend** — renders the vLLM serving workload (image, templated args, accelerator
  resource, model-load-aware probes, metrics). **Ascend** adapter scaffolded + golden-tested
  (not yet run on NPUs).
- **Scale-to-zero** — Hearth gateway (buffering reverse proxy) + KEDA `ScaledObject` on gateway
  queue depth; `0→1→N→0`, verified `1→2` across two GPU nodes.
- **Cold-start handling** — SSE keepalive heartbeats, `reject` mode, load-gated readiness, bounded
  queue (`429`) and activation timeout (`503`).
- **Graceful drain** — in-flight streams finish before scale-down.
- **Model caching** — `HostPath` and `NodeLocalPVC` (with pinnable `cache.storageClassName`,
  verified against Alibaba ESSD) + a prewarm Job.
- **Observability** — per-gateway Prometheus metrics, `ServiceMonitor`, and a Grafana dashboard.
- **No-GPU test harness** — a CPU `vllm-stub` and a kind + KEDA e2e that runs the full
  `0→1→N→0` loop, backpressure (`429`/`503`), and graceful drain on every PR, no accelerator
  required, plus a [no-GPU development guide](docs/no-gpu-development.md).
- **Packaging** — Helm chart (operator + RBAC + CRDs) and multi-arch image build/release workflow.
- **Project scaffolding** — README, ROADMAP, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, MAINTAINERS,
  GOVERNANCE, issue/PR templates, and a DCO check.

### Changed
- Operator skips no-op `LLMService` status updates, avoiding optimistic-concurrency churn.

[Unreleased]: https://github.com/hearth-project/hearth/compare/v0.2.0-rc.1...HEAD
[0.2.0-rc.1]: https://github.com/hearth-project/hearth/compare/v0.1.0...v0.2.0-rc.1
[0.1.0]: https://github.com/hearth-project/hearth/releases/tag/v0.1.0

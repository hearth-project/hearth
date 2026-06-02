# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project aims to follow
[Semantic Versioning](https://semver.org/) once it reaches a stable release.

## [Unreleased]

## [0.1.0] - 2026-06-02

First **pre-release (alpha)**. The core thesis — declarative, scale-to-zero serving of self-hosted
open-source LLMs on Kubernetes — is implemented and verified end-to-end on real NVIDIA GPUs.
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
- **Packaging** — Helm chart (operator + RBAC + CRDs) and multi-arch image build/release workflow.
- **Project scaffolding** — README, ROADMAP, CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, MAINTAINERS,
  GOVERNANCE, issue/PR templates, and a DCO check.

[Unreleased]: https://github.com/hearth-project/hearth/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hearth-project/hearth/releases/tag/v0.1.0

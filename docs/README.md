# Hearth documentation

A minimal, composable LLM serving control plane for private Kubernetes clusters.

- [Getting started](started.md) — install a release, select a hardware profile, send a request, upgrade, and clean up.
- [Architecture](architecture.md) — components, the two CRDs, and the scale-to-zero data flow.
- [Hearth and Kthena demo](demo.md) — command-driven hot-model and long-tail serving on one cluster.
- [Ascend hardware validation](ascend/ascend-validation.md) — shared prerequisites, validation levels, and required evidence.
- [Ascend 910B validation](ascend/ascend-910b-validation.md) — verified single-device scale-to-zero result, exact stack, and runbook.
- [Ascend 310P deployment validation](ascend/ascend-310p-validation.md) — verified Atlas 300I Duo report and Atlas 300I Pro physical-validation runbook.
- [NVIDIA A10 validation](nvidia/a10-validation.md) — verified v0.3.0-rc.1 two-device lifecycle, Volcano/Kthena coexistence, exact stack, and runbook.
- [CRD reference](crd-reference.md) — field-by-field `LLMService` spec reference.
- [Observability](observability.md) — optional Prometheus discovery, Grafana import, and gateway metrics.
- [Roadmap](../ROADMAP.md) — what's verified, and the prioritized path to production.
- [Contributing](../CONTRIBUTING.md) — dev setup, the build/test loop, and how to add a backend.
- [Developing without a GPU](no-gpu.md) — the `vllm-stub` and the no-hardware test loop.

For runnable, hardware-specific manifests, see [`examples/`](../examples).

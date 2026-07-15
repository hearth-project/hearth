# Hearth documentation

A minimal, composable LLM serving control plane for private Kubernetes clusters.

> Early and growing. Start here; more guides, including a spot-GPU walkthrough, are on the way.

- [Architecture](architecture.md) — components, the two CRDs, and the scale-to-zero data flow.
- [Ascend hardware validation](ascend-validation.md) — shared prerequisites, validation levels, and required evidence.
- [Ascend 910B validation](ascend-910b-validation.md) — real-hardware results and the remaining scheduling step.
- [Ascend 310P deployment validation](ascend-310p-validation.md) — verified Atlas 300I Duo report and Atlas 300I Pro physical-validation runbook.
- [CRD reference](crd-reference.md) — field-by-field `LLMService` spec reference.
- [Observability](observability.md) — Grafana dashboard import steps and gateway metrics.
- [Roadmap](../ROADMAP.md) — what's verified, and the prioritized path to production.
- [Contributing](../CONTRIBUTING.md) — dev setup, the build/test loop, and how to add a backend.
- [Developing without a GPU](no-gpu-development.md) — the `vllm-stub` and the no-hardware test loop.

For a runnable example, see [`config/samples/`](../config/samples).

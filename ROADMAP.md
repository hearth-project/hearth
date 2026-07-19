# Hearth Roadmap

> **Status: v0 (early / alpha).** The core thesis — declarative, scale-to-zero serving of
> self-hosted OSS LLMs on Kubernetes — is implemented and verified end-to-end on real hardware.
> The API is `v1alpha1` (no stability guarantee). **Not production-ready for shared or
> customer-facing workloads** — see [Production readiness](#production-readiness).
> Current release: **`v0.3.0` (alpha)**.

This roadmap is honest about what works, what it's good for today, and the prioritized path to a
production-grade release. It's a living document.

---

## What v0 does today (verified)

Verified **live on real hardware** (NVIDIA A100 on Alibaba ACK, NVIDIA A10 and Ascend on K3s) and
on kind. The A100 lifecycle evidence used vLLM `v0.22.0`; the checked-in `v0.25.1` profile awaits
focused A100 revalidation:

- **Declarative deploy** — one `LLMService` renders Deployment + Services + KEDA `ScaledObject` +
  optional cache and prewarm resources, with owner-ref cascade.
- **Scale-to-zero** — KEDA holds the backend at 0 when idle; a request wakes it.
- **Cold-start handling** — gateway buffers the request, emits SSE **keepalive heartbeats** so
  clients/ingress don't time out, holds until the model is loaded, then streams real tokens.
  Cold start ≈ **100 s (Qwen3-0.6B)** / **110 s (Qwen3-8B)** from zero, prewarmed.
- **Queue-driven autoscaling** — `0→1→N→0` on gateway queue depth; verified `1→2` across two GPU nodes.
- **Push activation** — an opt-in, co-located KEDA ExternalScaler streams cold-demand transitions;
  the polling path remains the default for compatibility.
- **NVIDIA A10 lifecycle** — two physical A10 GPUs are verified through external-push
  `0→1→2→0`, streaming inference, backpressure, reject mode, metrics, drain, self-heal, Helm
  upgrade, Volcano scheduling, and reboot recovery (see
  [NVIDIA A10 validation](docs/nvidia/a10-validation.md)).
- **Backpressure & limits** — bounded queue → `429`; activation timeout → `503`; `reject` cold-start mode.
- **Model caching + prewarm** — `HostPath` and `NodeLocalPVC` (incl. pinnable `storageClassName`,
  verified against Alibaba ESSD); weights hydrated before first traffic.
- **Graceful drain** — in-flight streams finish before a scale-down SIGTERM.
- **Observability** — gateway and vLLM metrics have stable discovery labels; an independent,
  opt-in `ServiceMonitor` and Grafana dashboard live under `examples/observability/`.
- **Packaging** — Helm chart installs operator + RBAC; verified reconciling under chart RBAC.
- **Multi-backend abstraction** — NVIDIA implemented and run; **Ascend 910B3** is verified through
  the device plugin, gateway, KEDA, cache, drain, and reboot recovery on one physical device. The
  completed topology was `0→1→0`; multi-replica scaling needs a multi-device server
  (see [Ascend 910B validation](docs/ascend/ascend-910b-validation.md)).
- **Ascend 310P lifecycle** — Atlas 300I Duo is verified through the device plugin and Hearth
  gateway, including `0→1→2→0`, backpressure, reject mode, drain, caching, and reboot recovery
  (see [Ascend 310P validation](docs/ascend/ascend-310p-validation.md)). Atlas 300I Pro remains
  rendering-tested only.
- **No-GPU CI loop** — the full `0→1→N→0` scale-to-zero e2e (CPU `vllm-stub` + a fake extended
  resource on kind) runs in CI; contributing needs no accelerator.

## Production readiness

**Use it today for:** internal / dev / staging serving where **GPU cost matters and brief downtime
is tolerable**, **latency-tolerant** traffic (cold start is seconds-to-minutes), and **packing many
mostly-idle models onto few GPUs**. Label deployments as alpha.

**Do not use yet for:** customer-facing low-latency endpoints, shared/multi-tenant clusters, or
anything requiring auth, SLAs, or stability guarantees.

---

## Path to production

### Now — finish domestic hardware coverage

- **Complete the Ascend 910B loop.** Status after the 2026-07-15 physical run with the RC images (see
  [Ascend 910B validation](docs/ascend/ascend-910b-validation.md)):

  - [x] vLLM-Ascend serves on a real 910B (CANN 9.0.0 / driver 26.0.rc1, vllm-ascend 0.21.0rc1).
  - [x] Operator renders correct 910B manifests (`huawei.com/Ascend910`, driver mounts, cache, probes).
  - [x] Gateway data-plane verified on the NPU (queue signal, passthrough, cold-start keepalive).
  - [x] Runtime image pinned to the verified tag (`vllm-ascend:v0.21.0rc1`).
  - [x] **Operator → device plugin → pod scheduled and serving on the NPU**, including cold
        `0→1→0`, prewarm/cache, reject mode, backpressure, metrics, drain, self-heal, Helm upgrade,
        and reboot recovery on a physical single-device 910B3 server.
  - [ ] Validate `1→N` on a server with more than one schedulable Ascend910 device. Do not infer a
        multi-replica result from the single-device run.

- [x] **Atlas 300I Duo.** The physical run passed the integrated `0→1→2→0` lifecycle,
  streaming inference, bounded-queue backpressure, reject mode, graceful drain, cache persistence,
  self-heal, Helm upgrade, and reboot recovery.
- [ ] **Atlas 300I Pro.** Validate it independently; the Duo result is not evidence for Pro. Follow
  the [310P report and runbook](docs/ascend/ascend-310p-validation.md).
- [x] **Volcano live validation** — Volcano `v1.15.0` enforced queue placement and quota on a
  three-node Kind cluster, then scheduled Hearth's external-push `0→1→2→0` path on two physical A10
  GPUs with distinct whole-device allocations. Multi-node accelerator topology, HAMi sharing, and
  gang scheduling remain separate work.
- [x] **Hearth and Kthena coexistence** — a Kthena-managed hot model and a Hearth-managed long-tail
  model served concurrently on the same two-GPU host. Hearth recovered automatically after reboot;
  Kthena required manual Pod replacement after an early device-plugin admission race, so unattended
  combined-stack reboot recovery remains unverified.

### P1 — unblock private / enterprise delivery
- [x] **`imagePullSecrets`** — private-registry support on backend, prewarm, and gateway Pods.
- [x] **`pvc://` model sources** — pre-staged, read-only weights with no download at serve time.
- [ ] **`oci://` model sources** — portable offline model delivery for the air-gapped bundle.
- [ ] **`SharedPVC` (RWX) cache** — node-local cache is per-node today, so each new replica
      re-downloads weights; RWX shared cache fixes multi-node cold starts.
- [ ] **Reliable multi-node scale-out** — a replica on a node without the runtime image cached pays a
      multi-minute image pull, and **KEDA scale-down churn can cancel an in-progress pull** so the
      replica never becomes Ready under bursty load (observed on the 2-node A100 run). Ship guidance +
      support for **image pre-distribution**: VPC/in-region registry endpoints, node image pre-pull
      (DaemonSet / ACK ImageCache), and/or a `scaleDownStabilization` floor that won't cancel pulls.
- [ ] **Helm/CRD install ergonomics** — document the Helm-v4-SSA vs `kubectl apply` CRD-ownership
      conflict; smooth upgrades.

### P2 — production hardening (shared / exposed use)
- [ ] **Minimal gateway auth** — static API keys on the OpenAI endpoint (explicitly *not*
      multi-tenancy yet). Today any in-cluster caller can hit any model.
- [ ] **Gateway HA hardening** — default is 1 replica (SPOF). Add `PodDisruptionBudget` +
      pod anti-affinity, and **aggregate the demand signal across replicas**. External-push enforces
      one gateway replica; polling with more than one replica has an incomplete per-Pod view.
- [ ] **Operator HA** — verify leader-election failover.
- [ ] **API stabilization** → `v1beta1` with validation/conversion webhooks; document compatibility.
- [ ] **Test depth** — soak + failure-injection (node/pod loss, GPU failure) on top of the existing
      no-GPU CI loop.

### Demand-driven backlog (parked, not abandoned — built when a named user asks)
- `ModelCatalog` CRD + curated Qwen/DeepSeek/GLM presets (`catalogRef` is unimplemented today).
- KV-cache / TTFT-SLO autoscaling — richer signals beyond queue depth.
- `BakedImage` cache; LoRA hot-swap; canary / blue-green rollouts.
- Multi-tenant quotas, RBAC/SSO, audit, rate limiting.
- **Xinchuang / air-gapped bundle** — offline images + model packs (lands after the P1 enablers).
- Security review + bilingual docs site.

---

## Ecosystem

Hearth is a **minimal, composable serving control plane** for the small end of the LLM-serving
axis. For fleet-grade serving — multi-model routing, prefill/decode disaggregation, datacenter
scale-out — use
[Kthena](https://github.com/volcano-sh/kthena), [AIBrix](https://github.com/vllm-project/aibrix), or
KServe/llm-d; they're excellent, and Hearth composes with them (hot models on the platform, the long
tail scaled to zero with Hearth). We share operational lessons from Hearth's verified scale-to-zero
path with Kthena's design ([kthena#1019](https://github.com/volcano-sh/kthena/issues/1019)). See the
README's ["Hearth and Kthena"](README.md#hearth-and-kthena) for the full positioning.

---

## Known limitations (v0)

- **Cold start is seconds-to-minutes** — scale-to-zero is for latency-tolerant traffic; set
  `scaling.min: 1` for latency-critical models (forgoes the cost saving).
- **Multi-node image pull dominates Nth-replica readiness** — see P1; pre-distribute images.
- **Node-local cache is per-node** — replicas on fresh nodes re-download weights (until `SharedPVC`).
- **`SharedPVC` / `BakedImage` cache strategies and `catalogRef` are not implemented.**
- **No auth, no multi-tenancy, no quotas.**
- **Ascend claims are stack- and topology-specific** — the 910B3 result verifies the integrated
  single-device `0→1→0` path, not multi-replica scaling or every 910B variant. Atlas 300I Duo is
  verified for its recorded stack; Atlas 300I Pro is manifest-only, and MLU is not implemented.
- **`v1alpha1`** — breaking API changes expected before `v1beta1`.

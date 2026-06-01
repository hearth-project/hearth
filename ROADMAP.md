# Hearth Roadmap

> **Status: v0 (early / alpha).** The core thesis — declarative, scale-to-zero serving of
> self-hosted OSS LLMs on Kubernetes — is implemented and verified end-to-end on real hardware.
> The API is `v1alpha1` (no stability guarantee). **Not production-ready for shared or
> customer-facing workloads** — see [Production readiness](#production-readiness).

This roadmap is honest about what works, what it's good for today, and the prioritized path to a
production-grade release. It's a living document.

---

## What v0 does today (verified)

Verified **live on real hardware** (NVIDIA A100 on Alibaba ACK, single- and multi-node) and on kind:

- **Declarative deploy** — one `LLMService` renders Deployment + Services + KEDA `ScaledObject` +
  `ServiceMonitor` + cache + prewarm Job, with owner-ref cascade.
- **Scale-to-zero** — KEDA holds the backend at 0 when idle; a request wakes it.
- **Cold-start handling** — gateway buffers the request, emits SSE **keepalive heartbeats** so
  clients/ingress don't time out, holds until the model is loaded, then streams real tokens.
  Cold start ≈ **100 s (Qwen3-0.6B)** / **110 s (Qwen3-8B)** from zero, prewarmed.
- **Queue-driven autoscaling** — `0→1→N→0` on gateway queue depth; verified `1→2` across two GPU nodes.
- **Backpressure & limits** — bounded queue → `429`; activation timeout → `503`; `reject` cold-start mode.
- **Model caching + prewarm** — `HostPath` and `NodeLocalPVC` (incl. pinnable `storageClassName`,
  verified against Alibaba ESSD); weights hydrated before first traffic.
- **Graceful drain** — in-flight streams finish before a scale-down SIGTERM.
- **Observability** — Prometheus scrapes gateway + vLLM `/metrics` via `ServiceMonitor`.
- **Packaging** — Helm chart installs operator + RBAC; verified reconciling under chart RBAC.
- **Multi-backend abstraction** — NVIDIA implemented & run; **Ascend** scaffolded + golden-tested
  (renders correct manifests) but **not yet run on real NPUs**.

## Production readiness

**Use it today for:** internal / dev / staging serving where **GPU cost matters and brief downtime
is tolerable**, **latency-tolerant** traffic (cold start is seconds-to-minutes), and **packing many
mostly-idle models onto few GPUs**. Label deployments as alpha.

**Do not use yet for:** customer-facing low-latency endpoints, shared/multi-tenant clusters, or
anything requiring auth, SLAs, or stability guarantees.

---

## Path to production

### P0 — blockers for any shared / exposed use
- [ ] **Gateway authN/authZ** — API keys / token auth on the OpenAI endpoint; per-model access
      control. Today any in-cluster caller can hit any model.
- [ ] **Gateway HA hardening** — default is 1 replica (SPOF). Add `PodDisruptionBudget` +
      pod anti-affinity, and **aggregate the demand signal across replicas** (KEDA currently polls a
      single gateway's pending count, which softens activation at >1 replica).
- [ ] **Reliable multi-node scale-out** — a replica on a node without the runtime image cached pays a
      multi-minute image pull, and **KEDA scale-down churn can cancel an in-progress pull** so the
      replica never becomes Ready under bursty load (observed on the 2-node A100 run). Ship guidance +
      support for **image pre-distribution**: VPC/in-region registry endpoints, node image pre-pull
      (DaemonSet / ACK ImageCache), and/or a `scaleDownStabilization` floor that won't cancel pulls.

### P1 — operability & stability
- [ ] **API stabilization** → `v1beta1` with validation/conversion webhooks; document compatibility.
- [ ] **Operator HA & cleanliness** — verify leader-election failover; stop the no-op status-update
      writes that cause a benign optimistic-concurrency conflict (skip-if-unchanged / `RetryOnConflict`).
- [ ] **`imagePullSecrets`** — private-registry support on backend/prewarm/gateway (Xinchuang/enterprise).
- [ ] **`SharedPVC` (RWX) cache** — node-local cache is per-node today, so each new replica
      re-downloads weights; RWX shared cache fixes multi-node cold starts.
- [ ] **Test depth** — no-GPU e2e harness (fake device-plugin + vLLM-stub) in CI running the full
      `0→1→N→0` loop on every PR; soak + failure-injection (node/pod loss, GPU failure).
- [ ] **Helm/CRD install ergonomics** — document the Helm-v4-SSA vs `kubectl apply` CRD-ownership
      conflict; smooth upgrades.

### P2 — breadth & enterprise (v1 → v2)
- [ ] **Ascend on real NPU** — validate the scaffolded backend on hardware (the headline v1 milestone);
      HAMi/Volcano integration for sharing/gang scheduling.
- [ ] **Model catalog** — `ModelCatalog` CRD + curated Qwen/DeepSeek/GLM presets (currently
      `catalogRef` is unimplemented).
- [ ] **KV-cache / TTFT-SLO autoscaling** — richer signals beyond queue depth.
- [ ] **`BakedImage` cache** — weights baked into the image for air-gapped/small-model cases.
- [ ] **Enterprise ops** — multi-tenant quotas, RBAC/SSO, audit, rate limiting; LoRA hot-swap;
      canary / blue-green rollouts.
- [ ] **Xinchuang / air-gapped bundle** — offline images + model packs, certified domestic runtimes.
- [ ] **Security review** + docs site (bilingual).

---

## Known limitations (v0)

- **Cold start is seconds-to-minutes** — scale-to-zero is for latency-tolerant traffic; set
  `scaling.min: 1` for latency-critical models (forgoes the cost saving).
- **Multi-node image pull dominates Nth-replica readiness** — see P0; pre-distribute images.
- **Node-local cache is per-node** — replicas on fresh nodes re-download weights (until `SharedPVC`).
- **`SharedPVC` / `BakedImage` cache strategies and `catalogRef` are not implemented.**
- **No auth, no multi-tenancy, no quotas.**
- **Ascend/MLU are manifest-only** (golden-tested), not run on hardware.
- **`v1alpha1`** — breaking API changes expected before `v1beta1`.

# Ascend 910B — validation report & bring-up runbook

**Status: experimental / technical preview (`v0.2.0-rc.1`).** On a real Ascend 910B we verified that
vLLM-Ascend serves on the NPU, that Hearth's operator renders correct 910B manifests, and that the
Hearth gateway data-plane works against the live NPU. The one remaining step — the operator scheduling
a backend pod onto an NPU through the device plugin, and the full integrated `0→1→N→0` loop — is **not
yet verified** and is the v1 "supported" milestone.

This is a deliberately honest split: three of the four bridges to "fully supported" are crossed; the
fourth needs a *schedulable* NPU node (see [Why not full e2e](#why-not-the-full-e2e-yet)).

## Verified environment

| Item | Value |
|---|---|
| Accelerator | Ascend **910B2C**, 64 GB HBM, single card |
| Host arch | x86_64 |
| CANN / driver | **9.0.0** / **26.0.rc1** |
| vLLM-Ascend | `vllm 0.21.0`, `vllm_ascend 0.21.0rc1` |
| Runtime image | `quay.io/ascend/vllm-ascend:v0.21.0rc1` (base Atlas-A2/910B tag) |
| Model (smoke) | `Qwen/Qwen2.5-0.5B-Instruct` from ModelScope |

## What was verified

### 1. vLLM-Ascend serves on the NPU
`vllm serve` loaded the model onto the 910B and answered an OpenAI `/v1/chat/completions` request.
HBM rose from ~3.4 GB idle to ~28 GB while bound (weights ≈ 0.93 GB + KV cache); engine init
(profile + KV cache + warmup) ≈ 2.4 s. This retires the hardware-only risk: the pinned vLLM-Ascend
release is compatible with this CANN/driver stack.

> Note: HuggingFace egress was blocked in the test environment; ModelScope was used
> (`VLLM_USE_MODELSCOPE=true`). Hearth wires this automatically for `modelscope://` sources.

### 2. Operator renders a correct 910B backend (kind dry-run)
Applying the `vllm-ascend` `InferenceRuntime` + an `LLMService` with `runtime.selector.vendor: [ascend]`
on a local kind cluster, the operator resolved the runtime via `pickByVendor` and rendered a backend
`Deployment` with:
- accelerator request `huawei.com/Ascend910: 1`;
- CANN driver host-mounts (`/usr/local/Ascend/driver`, `npu-smi`, `dcmi`, `/etc/ascend_install.info`);
- node selector `accelerator: ascend-910`;
- load-gated `/health` readiness/liveness/startup probes;
- ModelScope cache wiring (`VLLM_USE_MODELSCOPE=true`, `MODELSCOPE_CACHE`) + a prewarm Job;
- the full child set (backend + gateway Deploy/Svc, cache PVC, prewarm Job).

The backend pod stays `Pending` ("didn't match node affinity/selector") — correct, since kind has no
NPU node.

### 3. Gateway data-plane on the real NPU
The Hearth `gateway` binary, pointed at the live vLLM-on-910B server, passed every surface:
- `/healthz` → 200; `/hearth/queue` → pending count incl. demand-linger;
- `/metrics` → `hearth_gateway_*` Prometheus series;
- OpenAI passthrough through `/` (streaming + non-streaming) returning real NPU tokens;
- cold-start SSE keepalive against a not-ready backend → `: heartbeat` beats → `event: error /
  backend activation timeout` at the deadline; non-stream cold path → `503 Retry-After`.

## What is NOT yet verified

- The operator scheduling a backend pod onto an NPU via the **Ascend device plugin**
  (`huawei.com/Ascend910` actually satisfied), and the full integrated `0→1→N→0` scale-to-zero loop
  on a real NPU node.

### Why not the full e2e yet
The preview box was an **unprivileged container** (default Docker capability set — no `SYS_ADMIN`,
read-only `/sys/fs/cgroup`, AppArmor `docker-default`). Nested Kubernetes (kind / k3s / Usernetes)
needs cgroup delegation and `SYS_ADMIN`, so it can't run there. Closing this step needs a privileged
container or a real NPU node.

## Bring-up runbook (for a schedulable NPU node)

When an NPU node (or privileged box that can host k3s/kind) is available:

1. **Confirm the device plugin advertises the resource:**
   ```bash
   kubectl describe node <npu-node> | grep -i huawei.com/Ascend910   # expect a non-zero count
   ```
   If absent, install the [Ascend Device Plugin](https://github.com/Ascend/ascend-device-plugin).
2. **Label the node** to match the runtime's `nodeSelector`:
   ```bash
   kubectl label node <npu-node> accelerator=ascend-910
   ```
3. **Verify the image tag** matches the node's CANN/driver. The sample pins `v0.21.0rc1` (CANN 9.0.0).
   For Atlas A3 use the `-a3` tag, for 310P the `-310p` tag, and `-openeuler` variants for openEuler.
4. **Install Hearth + the runtime, then deploy:**
   ```bash
   make install
   kubectl apply -f config/samples/serving_v1alpha1_inferenceruntime_ascend.yaml
   kubectl apply -f config/samples/serving_v1alpha1_llmservice.yaml   # set runtime.selector.vendor: [ascend]
   ```
5. **Watch the integrated path:** backend pod schedules onto the NPU → `Loading` → `Ready`; then drive
   `0→1→N→0` and curl the gateway. Record cold-start numbers. Success here earns the
   *"validated end-to-end on real domestic silicon (Ascend 910B)"* claim and graduates Ascend from
   preview to supported.

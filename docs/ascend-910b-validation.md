# Ascend 910B deployment validation

## Status

| Profile | Validation level | Status |
|---|---|---|
| Ascend 910B3, one 64 GB device | Scale-to-zero verified | Integrated `0 -> 1 -> 0` passed on physical hardware on 2026-07-15; the RC prewarm Job needed the source-equivalent workaround documented below. |
| Ascend 910B2C, one 64 GB device | Runtime-tested | The earlier direct vLLM and gateway result remains valid only for its recorded environment. |

The 910B3 result covers the operator, Ascend Device Plugin, KEDA, gateway, model cache, and
vLLM-Ascend on the exact stack below. It is a hardware-validated technical preview, not a claim
that every 910B server or software combination is supported. The server exposed one device, so
multi-replica `1 -> N` scaling could not be tested.

Use the [shared Ascend validation guide](ascend-validation.md) for evidence terminology and the
[310P report](ascend-310p-validation.md) for Atlas 300I profiles. Results are not transferable
between those products.

## Validated environment

| Item | Observed value |
|---|---|
| Accelerator | Physical Ascend `910B3`, chip version V1, 64 GB HBM, one device |
| Firmware / driver | `7.7.0.1.231` / `26.0.rc1` |
| Host | Arm64 Ubuntu 26.04 LTS; 16-core HiSilicon TaiShan-v110 |
| Kernel | `6.8.0-134-generic`; DKMS module `davinci_ascend/1.0` |
| Kubernetes | K3s `v1.36.2+k3s1`; containerd `v2.3.2-k3s2` |
| Helm / KEDA | Helm `v4.2.3`; KEDA `2.20.1` |
| Ascend Device Plugin | MindCluster `v7.3.0`; image manifest-list digest `sha256:42fada043e2aa486551dea5d7ed889947fdb7c23d5c34eed2ed72c8c34922876` |
| Runtime image | Internal mirror of `vllm-ascend:v0.21.0rc1`; digest `sha256:71e601c0aaf20e7fa600fbdb54ffda9c5bb8f000341b0c73380c216dfa5c0805` |
| Container stack | `vllm 0.21.0`, `vllm_ascend 0.21.0rc1`, CANN `9.0.0` |
| Hearth images | Operator `0.2.0-rc.1` digest `sha256:fe6095550ca35be60795020c5a391b9526a3203632449dae9f254c8027fdce56`; gateway digest `sha256:7d5c4cef1b0029c49fd3b639ff075cf8ff3a5386aa6e09600ba6bf9ca808f70d` |
| Model | `Qwen/Qwen2.5-0.5B-Instruct` from ModelScope |
| Storage | 40 GB system disk; 120 GB ext4 data disk for K3s, images, and the local-path model PVC |

The host initially ran kernel 7.0.0-14. The `26.0.rc1` driver source failed to build against that
kernel on this server and built successfully against Ubuntu's signed 6.8.0-134 kernel. Do not copy
that kernel choice blindly: use a driver-supported kernel for the exact host OS, then prove a reboot
before treating the installation as persistent.

## Functional results

### Scheduling, cache, and cold start

- MindCluster discovered card ID 5 as `910B3`, registered `huawei.com/Ascend910`, and reported
  `Ascend910-5 Healthy`. Node capacity and allocatable were both 1.
- The backend Pod was scheduled through the device plugin with both request and limit
  `huawei.com/Ascend910: 1`. It received the plugin allocation annotations, the CANN driver mounts,
  the cache PVC, and load-gated probes.
- The corrected prewarm Job downloaded 954 MB into the data-disk PVC in 66 seconds without
  requesting an NPU. The model-weight checksum was
  `fdf756fa7fcbe7404d5c60e26bff1a0c8b8aa1f72ced49e7dd0210fe288fb7fe`.
- A cold streaming request drove `0 -> 1`, received SSE heartbeats while the model loaded, returned
  real NPU tokens and `[DONE]`, and completed with HTTP 200 in 230.99 seconds. Gateway metrics
  recorded 184.23 seconds of activation wait.
- Warm JSON and SSE requests completed in 0.245 and 0.293 seconds. KEDA then returned the backend
  to zero and the NPU process disappeared.
- A second cached startup reached Ready in 2 minutes 47 seconds. vLLM reported 1.46 seconds to load
  0.932 GB of weights and 31.83 seconds for engine profiling, KV-cache creation, and warmup; most of
  the remaining cold time was Python/Ascend process initialization.

vLLM reserved about 55.05 GiB for KV cache and raised total HBM use from about 3.2 GiB idle to
60.9 GiB. That is expected allocator behavior for the default memory-utilization setting, not the
size of the model weights.

### Gateway and metrics

- `/healthz`, `/hearth/queue`, `/v1/models`, streaming and non-streaming
  `/v1/chat/completions`, and gateway `/metrics` passed.
- vLLM `/metrics` exposed the configured `vllm:num_requests_waiting`,
  `vllm:num_requests_running`, and time-to-first-token series.
- Reject mode returned HTTP 503 in 7 ms with `Retry-After: 10`; the demand linger still activated
  the backend from zero.
- With a five-second activation timeout, 105 concurrent cold requests produced exactly 100
  activation-timeout 503 responses and five queue-full 429 responses. The matching gateway
  counters were present.
- Prometheus Operator was intentionally absent. Hearth logged that it skipped `ServiceMonitor`
  creation and continued reconciling, so optional-CRD behavior passed; Prometheus Operator
  integration itself was not tested.

### Drain and recovery

A 15-second drain was not sufficient for this stack. The client received HTTP 200 and `[DONE]`,
but vLLM ended the generation with `finish_reason: "abort"` after one token. With
`drainTimeout: 60s`, the repeated deletion test delivered 129 SSE chunks, `[DONE]`, HTTP 200, and
normal `finish_reason: "length"` in 51.80 seconds. The 910B service example therefore uses 60 seconds.

The following recovery paths also passed:

- a no-op apply preserved every owned Deployment, Service, PVC, and ScaledObject UID;
- deleting the gateway Pod produced a new Ready Pod and left the public Service healthy;
- restarting the operator preserved all owned resource UIDs and status;
- a same-values Helm upgrade to revision 2 preserved the operator Deployment and service resources;
- a full host reboot restored the 6.8 kernel, driver modules, healthy NPU, device-plugin resource,
  K3s, KEDA, Hearth, gateway, ScaledObject, and bound PVC; and
- the cache checksum remained unchanged. A post-reboot cold request again completed through the
  gateway with real NPU tokens, `[DONE]`, and HTTP 200 in 256.25 seconds before returning to zero.

Runtime selection by vendor also resolved the Ascend profile. Unsupported `kvCacheUtil` scaling,
fractional devices, private-source `secretRef`, and a missing runtime produced their intended
Degraded reasons. Admission rejected `scaling.min > scaling.max` and an unknown runtime vendor.

## Release-candidate findings

The hardware run found three release-image/example issues. The example corrections work immediately;
the controller corrections require an operator image rebuilt from current source:

1. The old 910B example supplied only `--model=...` flags. The runtime image entrypoint delegates to
   argv, so it needs explicit `vllm serve`. The example now uses that form.
2. The release operator omitted `TORCH_DEVICE_BACKEND_AUTOLOAD=0` from accelerator-free prewarm
   Pods. ModelScope imported PyTorch, auto-loaded `torch_npu`, and failed on `libascend_hal.so`.
   Current source adds the safeguard; the corrected Job completed without an NPU.
3. Changing an `InferenceRuntime` did not requeue its dependent `LLMService` in the release image.
   The backend template changed only after the service was touched. Current source already watches
   runtimes from the service reconciler.

The images also identify their version as `dev`. Release builds should inject the version so logs
can be tied to an artifact.

## Deployment runbook

### 1. Prepare a compatible node

Install a firmware/driver/kernel combination supported by the server vendor. Before Kubernetes
deployment, require all of these to pass:

```bash
uname -r
npu-smi info
cat /usr/local/Ascend/driver/version.info
```

If K3s images and model weights should live on a data disk, configure both `data-dir` and
`default-local-storage-path` in `/etc/rancher/k3s/config.yaml`; do not rely on editing K3s's
generated local-path manifest.

### 2. Install and verify the device plugin

Use MindCluster's standard non-Volcano Ascend 910 manifest unless the cluster deliberately uses a
different scheduler mode. Its node selector and the Hearth runtime example use the same label:

```bash
kubectl label node <npu-node> accelerator=huawei-Ascend910 --overwrite
kubectl get nodes \
  -o custom-columns='NAME:.metadata.name,ASCEND910:.status.allocatable.huawei\.com/Ascend910'
```

Do not continue until the value is non-zero and the plugin log reports a healthy device.

### 3. Install Hearth and apply the validated profile

Install KEDA and Hearth using the normal project instructions, then use a dedicated namespace:

```bash
kubectl create namespace hearth-910b-validation
kubectl apply -k examples/ascend/910b3 -n hearth-910b-validation
kubectl get llmservice,pvc,job,deploy,pod,scaledobject \
  -n hearth-910b-validation -w
```

If the cluster lacks a default StorageClass, set `cache.storageClassName`. Keep `scaling.max: 1`
unless more than one schedulable `huawei.com/Ascend910` resource is actually available.

### 4. Exercise scale-to-zero

Wait for prewarm completion and for KEDA to hold the backend at zero, then expose the gateway:

```bash
kubectl port-forward -n hearth-910b-validation \
  service/qwen-910b-validation 8080:80

curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"qwen-910b-validation","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"Reply with: Hearth 910B validation passed"}]}'
```

Record the zero replica state, KEDA activation, plugin allocation, load-gated readiness, completed
response, metrics, scale-down, and released NPU. A multi-device server should additionally raise
the configured maximum and prove distinct devices before making a multi-replica claim.

## Remaining bounds

- Multi-replica scaling was not testable on the one-device server.
- Volcano, HAMi sharing, and Prometheus Operator were not installed; only their optional boundaries
  or rendered contracts were checked.
- The exact RC image, driver, firmware, kernel, and device-plugin combination is the evidence unit.
  Revalidate after changing any of them.
- Hearth remains `v1alpha1` and has no authentication or multi-tenancy; hardware validation does
  not make it production-ready for shared or public workloads.

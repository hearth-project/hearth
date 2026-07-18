# NVIDIA A10 deployment validation

## Status

| Profile | Validation level | Status |
|---|---|---|
| NVIDIA A10 | Scale-to-zero verified | Full `0 -> 1 -> 2 -> 0` lifecycle passed on two physical 24 GB GPUs on 2026-07-18. |

The result covers the operator, NVIDIA device plugin, KEDA, gateway, model cache, and vLLM on the
recorded stack. It follows the official [NVIDIA Container Toolkit installation guide](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html),
[K3s NVIDIA runtime guidance](https://docs.k3s.io/advanced#nvidia-container-runtime-support),
[NVIDIA device-plugin documentation](https://github.com/NVIDIA/k8s-device-plugin), and
[vLLM OpenAI-compatible server documentation](https://docs.vllm.ai/en/latest/serving/online_serving/openai_compatible_server/).

## Validation baseline

| Item | Required value |
|---|---|
| Device allocation | Whole-GPU mode; no MIG or time-slicing |
| Kubernetes resource | `nvidia.com/gpu` |
| Node selector | `nvidia.com/gpu.product=NVIDIA-A10` |
| Runtime image | `vllm/vllm-openai:v0.25.1` |
| Smoke model | `Qwen/Qwen2.5-7B-Instruct` from ModelScope |
| Context limit | Explicit `--max-model-len=4096` |
| GPU memory target | `--gpu-memory-utilization=0.9` |
| Default replica limit | `scaling.max: 1`; raise only for additional free A10 GPUs |
| Drain timeout | `120s` |

## Physical result — 2026-07-18

| Item | Observed value |
|---|---|
| Accelerator | Two physical NVIDIA A10 GPUs; 23,028 MiB each; compute capability 8.6 |
| Host | x86-64 Ubuntu 24.04.2 LTS; 32 vCPU; 125 GiB RAM; kernel `6.8.0-90-generic` |
| Driver | NVIDIA `570.133.20`; host `nvidia-smi` reported CUDA 12.8 |
| Kubernetes | K3s `v1.36.2+k3s1`; containerd `2.3.2-k3s2` |
| Helm / KEDA | Helm `4.2.3`; KEDA `2.20.1` |
| NVIDIA integration | Container Toolkit `1.19.1-1`; device plugin `0.19.3`; whole-GPU mode |
| vLLM image | Internal mirror `registry-huabei2.crs-internal.ctyun.cn/hearth-dev/vllm-openai:v0.25.1`; digest `sha256:cbebbf65f838251ba7457b4104f53b38dfdbefe54e5b64ad1d0b286ab08a5d82` |
| Hearth images | Operator `0.2.0` (`sha256:233810ad62972e28c247383295cd6d81ea40f4155d81a164da8524470a830402`); gateway `0.2.0` (`sha256:d977fe854a409ecdea06bf27689fddd1b164e9894f90e6c53c92d24c56c8a242`) |
| Model / cache | `Qwen/Qwen2.5-7B-Instruct`; 30 GiB NodeLocalPVC on a dedicated 120 GB ext4 data disk |

The checked-in profile defaults to one replica and the public vLLM image reference. For this
two-device validation, `spec.scaling.max` was raised to `2` and the supplied internal image mirror
was used. The model, resources, probes, lifecycle, cache, and endpoint settings matched the
profile.

Observed functional results:

- The device plugin advertised exactly two `nvidia.com/gpu` resources. A one-GPU PyTorch CUDA
  matrix multiplication passed before Hearth was deployed and again after device-plugin recovery.
- The prewarm Job downloaded about 15 GB without requesting a GPU. Its byte count and file
  fingerprint were unchanged after a full host reboot.
- A cold streaming request drove `0 -> 1`, received 13 gateway heartbeats, and completed with real
  model tokens, `[DONE]`, and HTTP 200 in `140.03s`. Warm JSON and streaming requests completed in
  `0.299s` and `0.297s`.
- Twenty-four concurrent requests drove `1 -> 2`; both Pods became Ready on distinct physical GPU
  UUIDs, and all 24 streams returned HTTP 200 with `[DONE]`.
- KEDA completed `2 -> 1 -> 0`, respected the configured stabilization window, and released both
  GPUs.
- Reject mode returned `503` with `Retry-After`. After the 100-request queue filled, the next five
  requests returned `429` with `Retry-After`.
- A 512-token stream completed with `[DONE]` after its serving Pod was deleted. The Pod honored the
  full `120s` pre-stop drain before termination.
- vLLM exposed `vllm:kv_cache_usage_perc`, queue, running-request, and time-to-first-token metrics.
  The removed `vllm:gpu_cache_usage_perc` metric was absent, as expected for this release.
- A no-op apply, operator replacement, gateway replacement, same-values Helm upgrade, and device
  plugin replacement preserved the expected resources and restored service.
- After a full host reboot, K3s, KEDA, Hearth, the device plugin, GPU capacity, object identities,
  and cached weights recovered. The first post-reboot request returned HTTP 200 with `[DONE]` in
  `211.74s`, including 20 gateway heartbeats.
- The deprecated `--model` form emitted a vLLM removal warning. After the profile was corrected to
  use the positional model argument, a fresh cold request completed in `110.78s` with HTTP 200 and
  `[DONE]`, and the warning was absent from the backend log.

The post-reboot weight load took `90.18s`, compared with `2.72s` while the host page cache was warm.
This was a local-filesystem read, not a model download, and remained within the profile's five-minute
activation timeout.

Prometheus Operator, Grafana, GPU sharing, MIG, multi-node scheduling, and automatic GPU Feature
Discovery labeling were not tested. The focused single-node lab used exact product labels derived
from `nvidia-smi`; production clusters should automate trustworthy hardware labeling.

Two non-blocking log findings remain. The supplied vLLM image injects four unrecognized
`VLLM_BUILD_*` environment variables, which is image-packaging noise rather than a Hearth runtime
failure. During concurrent spec and status changes, the controller also logged three optimistic
update conflicts; each retry converged without resource loss or service interruption.

## 1. Record the environment

Run on each target node and retain the output:

```bash
uname -a
cat /etc/os-release
nvidia-smi
nvidia-container-cli --version
```

From the Kubernetes management node:

```bash
kubectl version
kubectl get nodes -o wide
kubectl get crd scaledobjects.keda.sh
helm list --all-namespaces
```

Record the GPU product, count, memory, UUIDs, compute capability, driver, Container Toolkit,
device-plugin, container-runtime, Kubernetes, KEDA, and image versions.

## 2. Configure the K3s NVIDIA runtime

Install the Container Toolkit using NVIDIA's distribution-specific instructions. K3s discovers the
NVIDIA runtime when its executable is on the service's path. Configure it as the default runtime in
`/etc/rancher/k3s/config.yaml`, then restart K3s:

```yaml
default-runtime: nvidia
```

```bash
systemctl restart k3s
kubectl get node
```

If the runtime cannot find `runc`, point the toolkit at the K3s-bundled executable rather than
installing a second untracked runtime. Verify the rendered K3s containerd configuration and run a
disposable CUDA Pod before continuing.

## 3. Install and verify the device plugin

Install an explicitly pinned official chart in whole-GPU mode:

```bash
helm repo add nvdp https://nvidia.github.io/k8s-device-plugin --force-update
helm upgrade --install nvidia-device-plugin nvdp/nvidia-device-plugin \
  --version 0.19.3 \
  --namespace nvidia-device-plugin \
  --create-namespace \
  --set runtimeClassName=nvidia \
  --set migStrategy=none \
  --set failOnInitError=true
```

Confirm the DaemonSet is Ready and that capacity matches the physical whole GPUs:

```bash
kubectl rollout status daemonset/nvidia-device-plugin -n nvidia-device-plugin
kubectl get nodes \
  -o custom-columns='NAME:.metadata.name,GPUS:.status.allocatable.nvidia\.com/gpu'
```

The profile requires `nvidia.com/gpu.product=NVIDIA-A10`. GPU Feature Discovery or equivalent
platform automation should publish it. In a focused lab without discovery, add it manually only
after confirming the exact product with `nvidia-smi`:

```bash
kubectl label node <a10-node> nvidia.com/gpu.present=true --overwrite
kubectl label node <a10-node> nvidia.com/gpu.product=NVIDIA-A10 --overwrite
kubectl get node <a10-node> -L nvidia.com/gpu.product
```

## 4. Place K3s data on the data disk

Keep container images and model PVCs off a small system disk. For K3s, configure both locations in
`/etc/rancher/k3s/config.yaml` after mounting the data disk persistently:

```yaml
data-dir: /var/lib/hearth-data/k3s
default-local-storage-path: /var/lib/hearth-data/local-path
```

Restart K3s and prove the placement with a disposable PVC. Do not rely only on the live local-path
ConfigMap because K3s regenerates packaged manifests.

## 5. Deploy the A10 profile

Install Hearth and KEDA first, then apply the independent profile to a dedicated namespace:

```bash
kubectl create namespace hearth-a10-validation
kubectl apply -k examples/nvidia/a10 -n hearth-a10-validation
kubectl get inferenceruntime vllm-nvidia-a10
kubectl get llmservice,pvc,job,deploy,pod,scaledobject \
  -n hearth-a10-validation -w
```

On a lab with two otherwise-idle A10 GPUs, raise the maximum only for the scale-out test:

```bash
kubectl patch llmservice qwen2-5-7b-a10 -n hearth-a10-validation \
  --type merge -p '{"spec":{"scaling":{"max":2}}}'
```

Wait for prewarming to complete and the backend to settle at zero. Confirm that the Job did not
request a GPU and the backend template requests one:

```bash
kubectl get job qwen2-5-7b-a10-prewarm -n hearth-a10-validation
kubectl get deployment qwen2-5-7b-a10 -n hearth-a10-validation -o yaml
```

## 6. Exercise inference and scale-to-zero

Expose the stable gateway Service:

```bash
kubectl port-forward -n hearth-a10-validation service/qwen2-5-7b-a10 8080:80
```

Send a streaming request while the backend is at zero:

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"qwen2-5-7b-a10","stream":true,"messages":[{"role":"user","content":"Reply with: Hearth A10 validation passed"}]}'
```

Record gateway heartbeats, `0 -> 1`, load-gated readiness, HTTP 200, real model tokens, `[DONE]`,
and the eventual return to zero. For two-device validation, hold more than ten concurrent requests
and prove that two Ready Pods have different allocated GPU UUIDs before allowing `2 -> 1 -> 0`.

Check the runtime metrics directly while a backend exists:

```bash
kubectl port-forward -n hearth-a10-validation service/qwen2-5-7b-a10-backend 8000:8000
curl -fsS http://127.0.0.1:8000/metrics | \
  grep -E 'vllm:(kv_cache_usage_perc|num_requests_waiting|num_requests_running|time_to_first_token_seconds)'
```

## 7. Recovery and lifecycle checks

For a complete validation, retain object UIDs and cache fingerprints before each test, then verify:

- no-op profile reapply;
- operator and gateway Pod replacement;
- same-values Hearth Helm upgrade;
- device-plugin Pod replacement and a fresh CUDA smoke Pod;
- in-flight streaming completion during backend Pod deletion; and
- full host reboot with persistent data-disk mount, cache identity, GPU capacity, and post-reboot
  inference.

Do not run destructive recovery tests on a shared or production cluster.

## Troubleshooting

### Backend remains Pending

Check the exact product label, allocatable `nvidia.com/gpu`, taints, PVC binding, and competing GPU
workloads:

```bash
kubectl describe pod -n hearth-a10-validation <backend-pod>
kubectl get nodes -L nvidia.com/gpu.product
kubectl describe node <a10-node>
nvidia-smi
```

### Device plugin is Ready but reports no GPUs

Verify the host driver first, then confirm K3s recognized the NVIDIA runtime and the plugin uses the
expected runtime class. A healthy host `nvidia-smi` does not by itself prove that containerd can
inject a GPU.

### Backend never becomes Ready

```bash
BACKEND_POD=$(kubectl get pod -n hearth-a10-validation \
  -l serving.hearth.dev/llmservice=qwen2-5-7b-a10 \
  --field-selector=status.phase=Running \
  -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n hearth-a10-validation "$BACKEND_POD"
kubectl describe pod -n hearth-a10-validation \
  -l serving.hearth.dev/llmservice=qwen2-5-7b-a10
```

Check image/driver compatibility, rendered arguments, cache contents, GPU assignment, and startup
probe history. The A10 profile uses the positional model argument required by current `vllm serve`.

### Gateway activation timeout

Inspect prewarming, scheduling, startup, and readiness before increasing `activationTimeout`. A
longer timeout does not repair missing devices, an incompatible image, or failed model loading.

## Success criteria

The A10 profile passes physical validation only when:

- each target node advertises healthy `nvidia.com/gpu` capacity and matches the exact A10 selector;
- prewarming completes without requesting a GPU;
- the backend receives one whole GPU and `/health` gates readiness correctly;
- cold and warm OpenAI-compatible streaming requests complete through the gateway with `[DONE]`;
- KEDA completes an observed `0 -> 1 -> configured max -> 0` loop and releases the GPUs;
- multiple replicas use distinct physical allocations when `max` is greater than one;
- an in-flight stream completes during backend termination; and
- the driver, toolkit, device plugin, image digests, cache identity, logs, and timings are recorded.

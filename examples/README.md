# Examples

## Device profiles

Each directory is an independently deployable `InferenceRuntime` and `LLMService` pair for one
accelerator model. Apply only the profile that matches the devices advertised by your Kubernetes
cluster. The root `examples/kustomization.yaml` is intentionally empty so that
`kubectl apply -k examples` cannot deploy incompatible workloads across multiple accelerator
families.

| Profile | Device | Validation |
|---|---|---|
| [`ascend/910b3`](ascend/910b3) | Ascend 910B3 (Atlas A2) | Hardware-verified single-device `0→1→0`; see the [report](../docs/ascend/ascend-910b-validation.md) |
| [`ascend/310p-duo`](ascend/310p-duo) | Two Ascend 310P3 devices (Atlas 300I Duo) | Hardware-verified `0→1→2→0`; see the [report](../docs/ascend/ascend-310p-validation.md) |
| [`ascend/310p-pro`](ascend/310p-pro) | Ascend 310P (Atlas 300I Pro) | Manifest and rendering tested; physical validation is still required |
| [`nvidia/a100`](nvidia/a100) | NVIDIA A100 | Lifecycle verified with vLLM `v0.22.0`; current `v0.25.1` profile awaits focused revalidation |
| [`nvidia/a10`](nvidia/a10) | NVIDIA A10 | Hardware-verified with vLLM `v0.25.1`: external-push `0→1→2→0` and Volcano scheduling on two whole GPUs; see the [report](../docs/nvidia/a10-validation.md) |

Install Hearth, KEDA, and the device plugin for the selected accelerator before applying a
profile. For example:

```bash
kubectl create namespace ai
kubectl apply -k examples/ascend/310p-duo -n ai
```

`InferenceRuntime` is cluster-scoped; `LLMService` is created in the namespace selected by `-n`.
All bundled service profiles use `NodeLocalPVC`. Ensure the cluster has a default dynamic
StorageClass, or set `cache.storageClassName` before applying a profile.

The A100 profile does not guess a `nvidia.com/gpu.product` value because the recorded validation
does not identify the exact PCIe/SXM and memory SKU label. Add the exact label reported by the
target cluster before using this profile in a mixed NVIDIA node pool.

The A10 result used whole-device allocation. It does not establish HAMi, fractional GPU, MIG,
multi-node, or gang-scheduling support.

## Optional observability

Hearth exposes metrics but does not create Prometheus or Grafana resources. The independent
[`observability`](observability) package contains an opt-in `ServiceMonitor` and dashboard.

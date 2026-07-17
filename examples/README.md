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
| [`nvidia/a100`](nvidia/a100) | NVIDIA A100 | Hardware-verified end to end |

Install Hearth, KEDA, and the device plugin for the selected accelerator before applying a
profile. For example:

```bash
kubectl create namespace ai
kubectl apply -k examples/ascend/310p-duo -n ai
```

`InferenceRuntime` is cluster-scoped; `LLMService` is created in the namespace selected by `-n`.
All bundled service profiles use `NodeLocalPVC`. Ensure the cluster has a default dynamic
StorageClass, or set `cache.storageClassName` before applying a profile.

## Optional observability

Hearth exposes metrics but does not create Prometheus or Grafana resources. The independent
[`observability`](observability) package contains an opt-in `ServiceMonitor` and dashboard.

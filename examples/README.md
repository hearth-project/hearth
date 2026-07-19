# Examples

## Device profiles

Each directory is an independently deployable `InferenceRuntime` and `LLMService` pair for one
accelerator model. Apply only the profile that matches the devices advertised by your Kubernetes
cluster. The root `examples/kustomization.yaml` is intentionally empty so that
`kubectl apply -k examples` cannot deploy incompatible workloads across multiple accelerator
families.

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

## Optional observability

Hearth exposes metrics but does not create Prometheus or Grafana resources. The independent
[`observability`](observability) package contains an opt-in `ServiceMonitor` and dashboard.

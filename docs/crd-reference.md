# CRD reference

Hearth CRDs use API group `serving.hearth.dev/v1alpha1`. This page documents the
user-facing `LLMService` spec fields from [`api/v1alpha1/llmservice_types.go`](../api/v1alpha1/llmservice_types.go)
and the generated CRD schema.

## LLMService

`LLMService` is a namespaced resource that describes what model to serve, which runtime to use, how
many accelerator resources to request, how to scale, how to cache model weights, and how cold starts
should behave.

Only `spec.model` is required by the CRD schema. Other sections are optional and use the defaults
shown below when a default is defined.

| Field | Type | Default / enum | Description |
|---|---|---|---|
| `spec.model` | object | required | Model identity and source. |
| `spec.model.catalogRef` | string | - | Optional catalog entry name for a model resolved outside the inline source block. 🚧 **Not yet implemented in v0** — use `spec.model.source.uri` instead. See [#33](https://github.com/hearth-project/hearth/issues/33). |
| `spec.model.source` | object | - | Inline model source configuration. |
| `spec.model.source.uri` | string | required when `source` is set | Model location. Supported in v0: `hf://`, `modelscope://`, and `pvc://<claim>[/<subpath>]` (weights pre-staged on an existing PVC, mounted read-only at `/models` — no download; pair with `cache.strategy: None`). `oci://` and `s3://` are not implemented yet. See [#36](https://github.com/hearth-project/hearth/issues/36). |
| `spec.model.source.secretRef` | object | - | Reserved for private model sources; currently rejected during reconciliation. |
| `spec.model.source.secretRef.name` | string | `""` | Name of the Secret holding source credentials. |
| `spec.runtime` | object | - | Backend runtime selection. Pin a runtime by name or provide a vendor preference selector. |
| `spec.runtime.name` | string | - | Exact `InferenceRuntime` name to use, such as `vllm-nvidia`. |
| `spec.runtime.selector` | object | - | Runtime auto-selection criteria. |
| `spec.runtime.selector.vendor` | string array | - | Acceptable vendors in preference order, for example `["nvidia", "ascend"]`. |
| `spec.runtime.argsOverride` | string array | - | Additional or overriding serving arguments appended to the selected runtime's templated args. |
| `spec.resources` | object | - | Abstract accelerator, CPU, and memory request mapped onto the selected runtime at reconcile time. |
| `spec.resources.accelerators` | integer | `1`, minimum `1` | Number of whole accelerator devices to request. |
| `spec.resources.fraction` | object | - | Reserved for sub-device sharing; currently rejected during reconciliation. |
| `spec.resources.fraction.memory` | quantity | - | Memory portion for a fractional accelerator request. |
| `spec.resources.fraction.cores` | integer | - | Core count for a fractional accelerator request. |
| `spec.resources.cpu` | quantity | - | CPU request for the serving workload. |
| `spec.resources.memory` | quantity | - | Memory request for the serving workload. |
| `spec.scaling` | object | - | KEDA-driven autoscaling configuration. Hearth supports LLM-aware signals rather than CPU or raw RPS. |
| `spec.scaling.min` | integer | `0`, minimum `0` | Minimum backend replicas. `0` enables scale-to-zero. |
| `spec.scaling.max` | integer | `1`, minimum `1` | Maximum backend replicas. |
| `spec.scaling.metric` | string | default `queueDepth`; enum `queueDepth`, `kvCacheUtil` | LLM-aware metric used for scaling decisions. |
| `spec.scaling.target` | integer | `10`, minimum `1` | Desired metric value per replica. |
| `spec.scaling.activationTimeout` | duration string | `5m` | How long the gateway can buffer a request while waiting for a cold backend to become ready. |
| `spec.scaling.scaleDownStabilization` | duration string | `5m` | Stabilization window before scaling down after demand drops. |
| `spec.scaling.drainTimeout` | duration string | `2m` | Time allowed for in-flight requests to drain. Must be no greater than the runtime termination grace period. |
| `spec.cache` | object | - | Model-weight cache configuration for reducing cold-start downloads. |
| `spec.cache.strategy` | string | default `NodeLocalPVC`; enum `NodeLocalPVC`, `HostPath`, `SharedPVC`, `BakedImage`, `None` | Cache backend. `SharedPVC` is listed in the API but not yet supported in v0; tracked in [#37](https://github.com/hearth-project/hearth/issues/37). `BakedImage` is also not implemented in v0 yet; tracked in [#33](https://github.com/hearth-project/hearth/issues/33). |
| `spec.cache.size` | quantity | - | Requested cache PVC size for `NodeLocalPVC`. |
| `spec.cache.storageClassName` | string | - | StorageClass for the cache PVC. Empty uses the cluster default. |
| `spec.cache.prewarm` | boolean | - | Whether to hydrate model weights into the persistent cache before first traffic. |
| `spec.endpoint` | object | - | Client-facing endpoint behavior. |
| `spec.endpoint.openAICompatible` | boolean | `true` | Whether the endpoint should expose the OpenAI-compatible API path. |
| `spec.endpoint.coldStart` | object | - | Behavior for requests received while the backend is scaled to zero or still loading. |
| `spec.endpoint.coldStart.mode` | string | default `keepalive`; enum `keepalive`, `reject` | `keepalive` holds streaming requests open with SSE heartbeats; `reject` returns fast `503 + Retry-After`. |
| `spec.endpoint.coldStart.heartbeatInterval` | duration string | `10s` | Interval between keepalive heartbeats while a cold streaming request is waiting. |
| `spec.imagePullSecrets` | object array | - | `LocalObjectReference`s applied to the backend, gateway, and prewarm pods so images from private / air-gapped registries can be pulled. The named Secrets must exist in the LLMService's namespace. |
| `spec.imagePullSecrets[].name` | string | `""` | Name of an image-pull Secret in the LLMService's namespace. |

Kubernetes quantity fields accept standard resource quantity strings such as `8`, `500m`, `32Gi`,
or `60Gi`. Duration fields use Go/Kubernetes duration strings such as `10s`, `2m`, or `5m`.

## InferenceRuntime

`InferenceRuntime` is cluster-scoped configuration consumed by the `LLMService` controller. Its own
controller is passive. Changing a runtime requeues services that pin it or select its vendor.

| Field | Required | Description |
|---|---|---|
| `spec.family` | yes | Runtime family; defaults to `vllm`. |
| `spec.vendor` | yes | Adapter key: `nvidia`, `ascend`, `cambricon`, `hygon`, or `moorethreads`. An adapter must also be registered in code. |
| `spec.priority` | no | Tie-breaker within one vendor; higher values win. Vendor order in an `LLMService` selector wins before priority. |
| `spec.container.image` | yes | Serving image. It must expose an OpenAI-compatible API and the configured metrics endpoint. |
| `spec.container.args` | no | Go templates rendered with `.Model.Path`, `.Service.Name`, and `.Service.Namespace`. Service `argsOverride` values are appended. |
| `spec.container.env` | no | Container environment; literal values may use the same templates. |
| `spec.container.port` | yes | Shared API and metrics port. `containerPort` must be between `1` and `65535`. |
| `spec.accelerator.resourceName` | yes | Extended resource advertised by the installed device plugin. |
| `spec.accelerator.nodeSelector` | no | Labels required on the accelerator node. |
| `spec.accelerator.tolerations` | no | Tolerations copied to the serving Pod. |
| `spec.accelerator.scheduler` | no | Optional scheduler name and Volcano queue. The queue becomes `scheduling.volcano.sh/queue-name`. |
| `spec.health` | no | Readiness, liveness, and startup probes. Hearth supplies readiness and startup defaults when omitted. |
| `spec.lifecycle.terminationGracePeriodSeconds` | no | Pod termination budget; minimum `1`. Hearth widens it when needed for draining. |
| `spec.lifecycle.preStopDrain` | no | Adds a pre-stop wait using `/bin/sh`; the serving image must contain a shell. |
| `spec.metrics` | yes | Metrics path, port name, and logical vLLM metric names. Queue depth is required. |

Runtime definitions describe Kubernetes integration; they do not install drivers, device plugins,
CANN, schedulers, or inference kernels.

# Getting started

This guide installs a released Hearth control plane, deploys one hardware profile, and sends a
request through the scale-to-zero gateway. For contributor workflows, see
[CONTRIBUTING.md](../CONTRIBUTING.md). For a complete loop without an accelerator, see
[Developing without a GPU](no-gpu-development.md).

## Prerequisites

Before deploying a model, provide:

- Kubernetes 1.29 or newer, with `kubectl` pointing to the intended cluster;
- Helm;
- KEDA for autoscaling and scale-to-zero;
- a driver and device plugin compatible with the selected accelerator;
- enough storage for the selected model, or a pre-staged `pvc://` model source; and
- registry and model-source access required by the selected profile.

Hearth does not install hardware drivers, device plugins, KEDA, model-serving engines, or
schedulers. Check the target before continuing:

```bash
kubectl config current-context
kubectl get nodes
```

Use a dedicated development cluster while evaluating Hearth.

## Install Hearth

The release publishes matching operator and gateway images and attaches a packaged Helm chart to
the GitHub release:

```bash
HEARTH_VERSION=0.2.0

helm repo add kedacore https://kedacore.github.io/charts --force-update
helm upgrade --install keda kedacore/keda \
  --version 2.20.1 \
  --namespace keda \
  --create-namespace

helm upgrade --install hearth \
  "https://github.com/hearth-project/hearth/releases/download/v${HEARTH_VERSION}/hearth-${HEARTH_VERSION}.tgz" \
  --namespace hearth-system \
  --create-namespace

kubectl rollout status deployment/hearth-controller-manager -n hearth-system
kubectl get crd inferenceruntimes.serving.hearth.dev llmservices.serving.hearth.dev
```

From a source checkout at the same version, the equivalent chart command is:

```bash
helm upgrade --install hearth ./charts/hearth \
  --namespace hearth-system \
  --create-namespace
```

KEDA is optional to the reconciler: without its CRD, Hearth still creates the serving resources but
skips the `ScaledObject`. Autoscaling and scale-to-zero are then disabled.

## Select one hardware profile

Each directory under [`examples/<vendor>/<device>/`](../examples) contains an independently
deployable `InferenceRuntime` and `LLMService` pair. Apply only a profile matching the extended
resource advertised by the installed device plugin. The validation matrix in
[`examples/README.md`](../examples/README.md) distinguishes physical validation from rendering-only
coverage.

For example, the NVIDIA A100 profile expects `nvidia.com/gpu`, a default dynamic StorageClass with
at least 60 GiB available, and outbound access to ModelScope:

```bash
HEARTH_VERSION=0.2.0
PROFILE_URL="https://github.com/hearth-project/hearth//examples/nvidia/a100?ref=v${HEARTH_VERSION}"

kubectl create namespace ai --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -n ai -k "${PROFILE_URL}"

kubectl get inferenceruntime vllm-nvidia
kubectl get llmservice -n ai
```

`InferenceRuntime` is cluster-scoped. `LLMService` and all generated workloads are created in the
`ai` namespace. If several equal-priority runtimes for the same vendor are installed, pin
`spec.runtime.name`; Hearth deliberately rejects an ambiguous vendor-only selection.

All bundled profiles use `NodeLocalPVC`. If the cluster has no default StorageClass, set
`cache.storageClassName` to a dynamic StorageClass before applying the profile.

## Understand the LLMService

The A100 profile includes the following workload shape. It pins the runtime so deployment does not
depend on vendor-selection priority:

```yaml
apiVersion: serving.hearth.dev/v1alpha1
kind: LLMService
metadata:
  name: qwen3-8b
spec:
  model:
    source:
      uri: modelscope://Qwen/Qwen3-8B-Instruct
  runtime:
    name: vllm-nvidia
    argsOverride:
      - --max-model-len=8192
      - --gpu-memory-utilization=0.9
  resources:
    accelerators: 1
    cpu: "8"
    memory: 32Gi
  scaling:
    min: 0
    max: 3
    metric: queueDepth
    target: 10
    activationTimeout: 5m
  cache:
    strategy: NodeLocalPVC
    size: 60Gi
    prewarm: true
  endpoint:
    openAICompatible: true
    coldStart:
      mode: keepalive
      heartbeatInterval: 10s
```

The important relationships are:

- `runtime.name` selects a cluster-scoped runtime profile;
- `resources.accelerators` is the number of whole devices requested by each backend replica;
- `scaling.min: 0` allows KEDA to release all accelerators while idle;
- `scaling.max` bounds backend replicas, not gateway replicas;
- `cache.prewarm` downloads weights without consuming an accelerator; and
- the always-on gateway exposes the OpenAI-compatible endpoint and KEDA queue signal.

The same API shape can target Ascend by pinning the matching Ascend runtime and using a compatible
model and runtime configuration. This is API portability, not a claim that every model, image, or
runtime flag is interchangeable between devices.

## Observe prewarming and activation

Watch the resources created for the service:

```bash
kubectl get llmservice,deployment,pod,service,pvc,job,scaledobject -n ai -w
```

The prewarm Job hydrates `qwen3-8b-cache`. When no request is pending, KEDA can hold the backend
Deployment at zero while the gateway remains available. Inspect failures with:

```bash
kubectl describe llmservice qwen3-8b -n ai
kubectl logs job/qwen3-8b-prewarm -n ai
kubectl get events -n ai --sort-by=.lastTimestamp
```

## Send a request

Forward the gateway Service from one terminal:

```bash
kubectl port-forward service/qwen3-8b 8000:80 -n ai
```

Then send a streaming request from another terminal:

```bash
curl -N http://127.0.0.1:8000/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "qwen3-8b",
    "messages": [{"role": "user", "content": "Reply with one short sentence."}],
    "stream": true
  }'
```

The first request raises the gateway queue signal and activates the backend. In `keepalive` mode,
SSE comment heartbeats can appear while the model starts. Depending on model size, image locality,
cache state, and hardware, activation can take seconds to minutes; it is not a 60-second guarantee.

After the configured stabilization window and when no requests remain, KEDA scales the backend
back to zero.

## Upgrade considerations

Upgrade with the next versioned chart asset:

```bash
HEARTH_VERSION=<new-version>
helm upgrade hearth \
  "https://github.com/hearth-project/hearth/releases/download/v${HEARTH_VERSION}/hearth-${HEARTH_VERSION}.tgz" \
  --namespace hearth-system
```

If the CRDs were previously managed with `kubectl apply` or `make install`, inspect their field
ownership before moving them under Helm. Helm 4 uses server-side apply for CRDs and can report
conflicts with an existing manager. Back up custom resources and test the migration; do not delete
CRDs containing live `LLMService` or `InferenceRuntime` objects merely to clear ownership.

Cache PVCs and prewarm Jobs contain immutable fields and are created once. Changing the model or
cache configuration may require intentionally replacing those resources; see
[Caching](architecture.md#caching).

## Clean up

Delete the service profile before removing the operator. The profile also contains a cluster-scoped
runtime, so confirm that no other service uses it:

```bash
HEARTH_VERSION=0.2.0
PROFILE_URL="https://github.com/hearth-project/hearth//examples/nvidia/a100?ref=v${HEARTH_VERSION}"

kubectl delete -n ai -k "${PROFILE_URL}"
helm uninstall hearth --namespace hearth-system
```

Helm does not remove CRDs from a chart's `crds/` directory during uninstall. Keep them when custom
resources remain, and remove them only as an explicit cluster-administration decision.

# Hearth and Kthena operational demo

This silent, hardware-neutral demo shows two serving policies sharing one Kubernetes cluster:

- Kthena keeps a frequently used model ready for low-latency traffic.
- Hearth holds a long-tail model at zero replicas until a request arrives.
- KEDA's external-push scaler activates the Hearth backend while the gateway keeps the client
  connection alive with SSE heartbeats.
- Volcano schedules both workloads without making the scheduler part of Hearth itself.

[![Watch the Hearth and Kthena operational demo](assets/hearth-kthena-demo.png)](assets/hearth-kthena-demo.mp4)

The 50-second recording uses real `kubectl` and `curl` commands. Hardware names, device UUIDs,
drivers, registry addresses, and validation namespaces are intentionally omitted so the operational
story applies to any accelerator supported by an installed device plugin and matching runtime
profile.

## What the recording proves

1. The Kthena-managed hot model is Ready while the Hearth `LLMService` is `ScaledToZero`.
2. The Kthena route returns a real OpenAI-compatible response.
3. A request to the Hearth gateway creates demand and receives heartbeats during model startup.
4. KEDA activates the backend, and Kubernetes reports the backend Pod becoming Ready.
5. The Hearth route returns a real OpenAI-compatible response.
6. When demand ends, the Hearth backend returns to zero while the Kthena model remains Running.

The video is product evidence, not a substitute for a hardware-validation report. The exact host,
images, component versions, timings, failure tests, and known limitations are recorded in the
[NVIDIA A10 validation report](nvidia/a10-validation.md).

## Observe the same lifecycle

The commands below assume Hearth, KEDA, Volcano, Kthena, a vendor device plugin, and compatible
runtime profiles are already installed. Kthena remains an independent platform; Hearth does not
install or reconcile its resources.

Set the names used by your environment in each shell:

```bash
export NAMESPACE=ai-serving
export LONGTAIL_MODEL=qwen-longtail
export HOT_MODEL=qwen-hot
```

Confirm the initial placement and lifecycle state:

```bash
kubectl get llmservice "$LONGTAIL_MODEL" -n "$NAMESPACE" \
  -o custom-columns=NAME:.metadata.name,PHASE:.status.phase
kubectl get modelserving "$HOT_MODEL" -n "$NAMESPACE"
kubectl get podgroup -n "$NAMESPACE"
kubectl get deployment,pod -n "$NAMESPACE"
```

To exercise the hot-model route, forward the independently installed Kthena router in one terminal:

```bash
kubectl port-forward -n kthena-system service/kthena-router 8081:80
```

Then send a request from another terminal:

```bash
curl -sS http://127.0.0.1:8081/v1/chat/completions \
  -H 'Content-Type: application/json' \
  --data-binary @- <<JSON
{"model":"$HOT_MODEL","messages":[{"role":"user","content":"Reply with: Kthena hot pass"}]}
JSON
```

In one terminal, watch Hearth activation:

```bash
kubectl get deployment "$LONGTAIL_MODEL" -n "$NAMESPACE" -w
```

In another terminal, expose the stable Hearth gateway:

```bash
kubectl port-forward -n "$NAMESPACE" "service/$LONGTAIL_MODEL" 8080:80
```

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  --data-binary @- <<JSON
{"model":"$LONGTAIL_MODEL","stream":true,"messages":[{"role":"user","content":"Reply with: Hearth long-tail pass"}]}
JSON
```

Inspect KEDA while the request is pending, then wait for the backend to return to zero:

```bash
kubectl get scaledobject "$LONGTAIL_MODEL" -n "$NAMESPACE"
kubectl get hpa "keda-hpa-$LONGTAIL_MODEL" -n "$NAMESPACE"
kubectl get deployment "$LONGTAIL_MODEL" -n "$NAMESPACE" -w
```

External-push mode requires one gateway replica until Hearth supports demand aggregation across
gateway Pods. Whole-device scheduling was used for the recorded run. The demo does not claim HAMi,
MIG, fractional accelerators, multi-node topology, or production readiness.

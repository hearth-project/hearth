# Ascend 310P deployment validation

## Status

Hearth provides **configuration and rendering support** for two Ubuntu-based Ascend 310P targets:

- Huawei Atlas 300I Duo
- Huawei Atlas 300I Pro

Neither profile has been validated by Hearth on physical 310P hardware yet. vLLM-Ascend currently
describes Atlas 300I Duo support as experimental and does not explicitly list Atlas 300I Pro in its
supported-device list. Do not interpret these manifests or rendering tests as a hardware-support
claim.

The profiles follow the official [vLLM-Ascend 310P guide](https://docs.vllm.ai/projects/ascend/en/latest/tutorials/hardwares/310p.html),
[installation guide](https://docs.vllm.ai/projects/ascend/en/main/installation/), and
[Huawei Ascend Device Plugin guide](https://www.hiascend.com/document/detail/en/mindcluster/730/clustersched/schedulingug/dlug_installation_019.html).
Shared image, evidence, and terminology requirements are in the
[Ascend hardware validation guide](ascend-validation.md).

## Validation baseline

| Item | Required value |
|---|---|
| Host OS | Ubuntu |
| Device allocation | Standard non-mixed, whole-device mode |
| Kubernetes resource | `huawei.com/Ascend310P` |
| Runtime image | `quay.io/ascend/vllm-ascend:v0.22.1rc1-310p` |
| Smoke model | `Qwen/Qwen2.5-0.5B-Instruct` from ModelScope |
| Context limit | Explicit `--max-model-len=2048` |
| Execution baseline | `--enforce-eager` |

Confirm the image, host driver, firmware, and CANN compatibility against the vLLM-Ascend release
notes for this exact runtime tag. Do not assume the 910B validation stack is compatible with 310P.

## 1. Record the environment

Run on each NPU node and save the output:

```bash
uname -a
cat /etc/os-release
npu-smi info
cat /usr/local/Ascend/driver/version.info
cat /etc/ascend_install.info
```

From the Kubernetes management node:

```bash
kubectl version
kubectl get nodes -o wide
kubectl get pods -n kube-system | grep -i ascend
kubectl get crd scaledobjects.keda.sh
```

Record this evidence separately for each product:

| Evidence | Atlas 300I Duo | Atlas 300I Pro |
|---|---|---|
| Exact product and NPU identifier | | |
| NPU memory | | |
| Host architecture and Ubuntu version | | |
| Firmware, driver, and CANN versions | | |
| Ascend Device Plugin version | | |
| Container runtime and Kubernetes version | | |
| KEDA version | | |
| Runtime image digest | | |

## 2. Verify the device resource

This runbook assumes Ascend Device Plugin is installed in standard non-mixed mode. Huawei documents
`huawei.com/Ascend310P` as the resource reported in that mode.

```bash
kubectl get nodes \
  -o custom-columns='NAME:.metadata.name,ASCEND310P:.status.allocatable.huawei\.com/Ascend310P'
```

Each target node must report a non-zero value. If not, correct the driver/device-plugin installation
before installing Hearth workloads. Mixed insertion is outside this profile because it reports
product-specific resources such as `huawei.com/Ascend310P-IPro`.

## 3. Label each product node

Huawei's standard label identifies the 310P family but does not distinguish Duo from Pro. The Hearth
label makes each validation result attributable to the intended card.

```bash
# Atlas 300I Duo
kubectl label node <duo-node> accelerator=huawei-Ascend310P --overwrite
kubectl label node <duo-node> serving.hearth.dev/ascend-product=atlas-300i-duo --overwrite

# Atlas 300I Pro
kubectl label node <pro-node> accelerator=huawei-Ascend310P --overwrite
kubectl label node <pro-node> serving.hearth.dev/ascend-product=atlas-300i-pro --overwrite

kubectl get nodes -L accelerator,serving.hearth.dev/ascend-product
```

## 4. Prepare Hearth

Use a dedicated cluster and namespace. Confirm the kube-context before changing cluster state.
Install KEDA if the scale-to-zero loop is in scope, then install Hearth:

```bash
kubectl config current-context
kubectl create namespace hearth-310p-validation
make install
```

If the cluster has no default dynamic StorageClass, set `cache.storageClassName` in both service
samples before applying them, or use a deliberately prepared HostPath cache.

## 5. Deploy one product profile

Run each profile separately so its evidence is unambiguous. Set `PROFILE` to `duo` or `pro`:

```bash
PROFILE=duo
RUNTIME="config/samples/serving_v1alpha1_inferenceruntime_ascend_310p_${PROFILE}.yaml"
SERVICE_FILE="config/samples/serving_v1alpha1_llmservice_ascend_310p_${PROFILE}.yaml"
SERVICE="qwen-310p-${PROFILE}-validation"

kubectl apply -f "$RUNTIME"
kubectl apply -n hearth-310p-validation -f "$SERVICE_FILE"
kubectl get llmservice,pvc,job,deploy,pod -n hearth-310p-validation -w
```

Confirm that the Pod landed on the intended node and requested one 310P:

```bash
kubectl get pod -n hearth-310p-validation \
  -l "serving.hearth.dev/llmservice=$SERVICE" \
  -o custom-columns='NAME:.metadata.name,NODE:.spec.nodeName,NPU:.spec.containers[0].resources.limits.huawei\.com/Ascend310P'
```

## 6. Exercise inference and scale-to-zero

```bash
kubectl get deploy "$SERVICE" -n hearth-310p-validation -w
```

Wait for KEDA to hold the backend at zero while the gateway remains available. In another terminal:

```bash
kubectl port-forward -n hearth-310p-validation "svc/$SERVICE" 8080:80
```

Send a streaming request. The gateway may emit heartbeat comments while the model loads:

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d "{\"model\":\"$SERVICE\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"Reply with: Hearth 310P validation passed\"}]}"
```

Record idle replicas at zero, the cold request causing `0 -> 1`, Loading-to-Ready status, a complete
stream ending in `[DONE]`, and the backend returning to zero after the stabilization window.

## 7. Troubleshooting

### Backend remains Pending

```bash
kubectl describe pod -n hearth-310p-validation <backend-pod>
kubectl get nodes -L accelerator,serving.hearth.dev/ascend-product
kubectl describe node <target-node> | grep -A5 -B5 Ascend310P
```

Check the device resource, product label, taints, and cache PVC binding.

### Prewarm fails

```bash
kubectl logs -n hearth-310p-validation job/<service-name>-prewarm
kubectl describe pvc -n hearth-310p-validation <service-name>-cache
```

Check ModelScope egress, DNS, proxy settings, StorageClass availability, and disk capacity.

### Backend never becomes Ready

```bash
kubectl logs -n hearth-310p-validation deploy/<service-name> --all-containers
kubectl describe pod -n hearth-310p-validation \
  -l serving.hearth.dev/llmservice=<service-name>
```

Check image/driver/CANN compatibility, driver projections, and device assignment first.

### NPU out of memory

Do not remove `--max-model-len=2048`: the official 310P guide warns that automatic context sizing can
allocate a quadratic attention mask. Confirm the rendered arguments and NPU ownership before changing
the model or context limit.

### Gateway activation timeout

Inspect scheduling, startup, and readiness before increasing `activationTimeout`. A longer timeout
does not fix missing devices, incompatible software, failed downloads, or probe failures.

## Success criteria

A product passes physical validation only when:

- the intended node satisfies `huawei.com/Ascend310P` and the product selector;
- prewarming completes and the backend loads the model;
- `/health` gates readiness correctly;
- an OpenAI-compatible streaming request completes through the gateway;
- KEDA completes an observed `0 -> 1 -> 0` loop; and
- driver, firmware, CANN, device-plugin, image digest, logs, and timings are recorded.

Until then, the product remains a configuration/rendering validation target.

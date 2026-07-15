# Contributing to Hearth

Thanks for your interest! Hearth is an early-stage, vendor-neutral Kubernetes operator for
**declarative, scale-to-zero serving of open-source LLMs** — NVIDIA today, Ascend/MLU as the
differentiator. It's small, moving fast, and very open to help. This guide gets you productive
quickly.

By participating you agree to uphold our [Code of Conduct](CODE_OF_CONDUCT.md). Contributions are
accepted under the project's **Apache-2.0** license (inbound = outbound).

## Where Hearth's boundary is (please read first)

Hearth is the **Kubernetes orchestration / lifecycle layer** — declarative deploy, model loading,
health, scheduling adaptation, scale-to-zero, metrics. It deliberately does **not**:

- re-implement the inference engine (that's **vLLM** + vendor plugins), or
- write chip kernels / device plugins / schedulers (that's the vendors / **HAMi** / **Volcano**).

A new accelerator is a thin **`InferenceRuntime` + adapter**, not a rewrite. Keep changes on the K8s
side of that line.

## Good ways to contribute

- **A new backend** — wire a chip as an adapter under [`internal/backend`](internal/backend) +
  golden tests. This is the project's whole thesis; high-value.
- **Validate on real hardware** — especially the complete Ascend device-plugin and scale-to-zero
  path. Start with the [Ascend validation guide](docs/ascend-validation.md).
- **Pick up a roadmap item** — see [`ROADMAP.md`](ROADMAP.md). The **P1/P2** items
  (`oci://` model sources, `SharedPVC`, gateway auth, HA hardening)
  and the **community track** (the KEDA external push scaler, #42) are great entry points.
- **Docs, examples, bug reports, repros** — all welcome, no change too small.

Look for issues labeled **`good first issue`** / **`help wanted`**, or open one to discuss before a
large change.

## Development setup

**Prerequisites:** Go **1.26+**, `make`, Docker or Podman (`CONTAINER_TOOL`), `kubectl`, `helm`, and
[`kind`](https://kind.sigs.k8s.io/) for local clusters. Scale-to-zero needs **KEDA** in the cluster.

```bash
git clone https://github.com/hearth-project/hearth && cd hearth
make build          # compile
make test           # unit + envtest (downloads envtest binaries on first run)
make lint           # golangci-lint (run before every PR)
```

### Run the control plane locally (no GPU needed)

```bash
make install                 # install CRDs into your current kube-context (e.g. a kind cluster)
make run                     # run the operator on your host against that context
kubectl apply -f examples/nvidia/serving_v1alpha1_inferenceruntime_nvidia_a100.yaml
kubectl apply -f examples/nvidia/serving_v1alpha1_llmservice_nvidia_a100.yaml
kubectl get llmservice,deploy,svc,scaledobject -w
```

The operator reconciles all child objects without a GPU; the backend pod stays `Pending` until a
real accelerator node is available (serving tokens requires an NVIDIA GPU + device plugin). For the
data-plane gateway to start, build/push it and pass `--gateway-image=<registry>/hearth-gateway:<tag>`
to the operator (`go run ./cmd/main.go --gateway-image=...`).

To exercise the **gateway and the full scale-to-zero path with no GPU**, use the `vllm-stub` (a CPU
fake of a vLLM server) — see [Developing without a GPU](docs/no-gpu-development.md).

### End-to-end tests

```bash
make setup-test-e2e   # creates an isolated kind cluster
make test-e2e         # run e2e against it (never your dev/prod cluster)
make cleanup-test-e2e
```

## Project layout & generated files

This is a [Kubebuilder](https://book.kubebuilder.io/) project. Key paths:

- `api/v1alpha1/*_types.go` — CRD schemas (edit these; add `+kubebuilder` markers).
- `internal/controller/*` — reconcilers.
- `internal/backend/*` — the multi-backend abstraction (adapters live here).
- `internal/gateway/*` — the data-plane proxy.
- `examples/<vendor>/*`, `charts/hearth/*` — hardware-specific examples and the Helm chart.

**Never hand-edit generated files** — `**/zz_generated.*`, `config/crd/bases/*`, `config/rbac/role.yaml`,
`PROJECT`. After changing API types or RBAC markers, regenerate:

```bash
make manifests generate    # regenerate CRDs, RBAC, deepcopy
make helm-crds             # sync the generated CRDs into charts/hearth/crds/
```

Commit the regenerated files alongside your change.

## Pull requests

1. **Branch** off `main`; keep PRs small and focused (one concern).
2. **Tests** — add/extend unit or golden tests; adapters in particular should be golden-tested so
   they're provable without hardware. `make test` and `make lint` must pass.
3. **Regenerate** manifests/CRDs if you touched API types (see above).
4. **Commit style** — short, imperative, [Conventional Commits](https://www.conventionalcommits.org/)
   prefixes (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`), scoped where useful
   (`feat(gateway): ...`). Reference the issue (`Closes #123`).
5. **Comments** explain *why* / non-obvious decisions, not what the code already says — match the
   surrounding style.
6. **Describe the change** and how you tested it (incl. hardware, if any).

CI runs build + lint + tests on every PR; green is required before review.

## Reporting bugs & proposing features

Open an issue with: what you expected, what happened, your environment (K8s version, accelerator,
runtime image), and minimal repro steps or manifests. For larger features, sketch the design in an
issue first so we can align before you invest time.

Questions are welcome — open a discussion or a `question`-labeled issue. Thanks for helping build
Hearth! 🔥

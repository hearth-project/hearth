#!/usr/bin/env bash
#
# Scale-to-zero demo storyboard, auto-typed for a clean screen recording.
#
# Prereqs (a real cluster with a small model gives the best, authentic GIF):
#   - kind/k8s cluster with KEDA installed
#   - Hearth installed (helm install hearth ./charts/hearth -n hearth-system ...)
#   - an InferenceRuntime + an LLMService named $SVC (min: 0) in namespace $NS,
#     served by a SMALL model (e.g. Qwen3-0.6B) so the cold start is ~tens of seconds
#
# Record it:
#   asciinema rec --idle-time-limit 2 --cols 100 --rows 24 \
#     -c 'NS=ai SVC=qwen3 ./hack/demo/demo.sh' demo.cast
#   agg --font-size 20 --speed 1.4 demo.cast demo.gif
#   gifsicle -O3 --lossy=80 demo.gif -o demo.gif   # shrink for the README
#
set -euo pipefail
NS=${NS:-ai}
SVC=${SVC:-qwen3}
PORT=${PORT:-8000}
TYPE_DELAY=${TYPE_DELAY:-0.02}

# pe: print a prompt, "type" the command, then run it.
pe() {
  local cmd="$1"
  printf '\033[1;32m$\033[0m '
  local i
  for ((i = 0; i < ${#cmd}; i++)); do printf '%s' "${cmd:i:1}"; sleep "$TYPE_DELAY"; done
  printf '\n'
  eval "$cmd"
}
say() { printf '\033[1;36m# %s\033[0m\n' "$1"; }
beat() { sleep "${1:-1.5}"; }

clear
say "Hearth — declarative, scale-to-zero LLM serving on Kubernetes"; beat 2

printf '\n'; say "1) Idle: the model is scaled to ZERO — no GPU burning"; beat 1
pe "kubectl get llmservice $SVC -n $NS"; beat 2
pe "kubectl get pods -n $NS -l serving.hearth.dev/llmservice=$SVC"; beat 2.5

# Open a tunnel to the gateway (background; cleaned up on exit).
kubectl port-forward -n "$NS" "svc/$SVC" "$PORT:80" >/dev/null 2>&1 &
PF=$!; trap 'kill "$PF" 2>/dev/null || true' EXIT
sleep 2

printf '\n'; say "2) One request wakes it — the gateway holds the connection"
say "   (SSE keepalive heartbeats), then streams real tokens"; beat 1
pe "curl -sN localhost:$PORT/v1/chat/completions -H 'content-type: application/json' \\
  -d '{\"model\":\"$SVC\",\"stream\":true,\"messages\":[{\"role\":\"user\",\"content\":\"In one sentence, what is scale-to-zero?\"}]}'"
beat 2

printf '\n'; say "3) The request scaled it 0 -> 1"; beat 1
pe "kubectl get llmservice $SVC -n $NS"; beat 2
pe "kubectl get pods -n $NS -l serving.hearth.dev/llmservice=$SVC"; beat 2.5

printf '\n'; say "4) Idle again -> KEDA scales it back to ZERO."
say "   Your models, your hearth. 🔥  github.com/hearth-project/hearth"; beat 3

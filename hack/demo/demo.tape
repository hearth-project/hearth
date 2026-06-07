# VHS tape (https://github.com/charmbracelet/vhs) — deterministic GIF, no fumbling.
#   vhs hack/demo/demo.tape          # -> hack/demo/demo.gif
#
# Best for a FAST, repeatable demo (a warm/small model or the CPU vllm-stub),
# because VHS waits with fixed `Sleep`s rather than on command completion.
# For an authentic real-GPU cold start (~tens of seconds), prefer demo.sh + asciinema.
#
# Assumes Hearth + KEDA installed and an LLMService `qwen3` (min: 0) in namespace `ai`,
# with a port-forward already open:  kubectl port-forward -n ai svc/qwen3 8000:80

Output hack/demo/demo.gif
Set Theme "Dracula"
Set FontSize 18
Set Width 1100
Set Height 640
Set TypingSpeed 35ms
Set Padding 18

Hide
Type "clear" Enter
Show

Type "# Idle: the model is scaled to ZERO — no GPU burning" Enter Sleep 1s
Type "kubectl get llmservice qwen3 -n ai" Enter Sleep 2.5s

Type "# One request wakes it: gateway holds, then streams real tokens" Enter Sleep 1s
Type `curl -sN localhost:8000/v1/chat/completions -H 'content-type: application/json' -d '{"model":"qwen3","stream":true,"messages":[{"role":"user","content":"In one sentence, what is scale-to-zero?"}]}'` Enter
Sleep 18s

Type "# It scaled 0 -> 1 to serve" Enter Sleep 1s
Type "kubectl get llmservice qwen3 -n ai" Enter Sleep 2.5s

Type "# Idle again -> KEDA scales it back to ZERO. Your models, your hearth. 🔥" Enter Sleep 3s

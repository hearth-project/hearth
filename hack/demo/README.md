# Recording the scale-to-zero demo

A short, **silent, looping GIF** of `idle → request → tokens → back to zero` is the
single most convincing asset for the project — and it's the README convention for
terminal/infra tools. No audio, no narration; it just loops. Put it at
`docs/demo.gif` and uncomment the `<img>` at the top of the main README.

Both options below are **silent and auto-generate the file** — no screen recorder,
no microphone.

## Prerequisites

A cluster with **KEDA** + **Hearth** installed, and an `LLMService` named `qwen3`
(`scaling.min: 0`) in namespace `ai`, served by a **small** model (e.g. Qwen3-0.6B)
so the cold start is seconds, not minutes. A short `scaling.scaleDownStabilization`
(e.g. `30s`) lets you also capture it scaling back to zero.

## Option A — VHS (deterministic, one command)

[VHS](https://github.com/charmbracelet/vhs) renders the tape straight to a GIF:

```bash
kubectl port-forward -n ai svc/qwen3 8000:80 &   # demo.tape assumes this is open
vhs hack/demo/demo.tape                            # -> hack/demo/demo.gif
```

Edit `demo.tape` to tune timing/theme. (Set `Output demo.mp4` for a video instead —
but the README hero should stay a GIF, since GIFs autoplay + loop inline.)

## Option B — asciinema + agg (best for a real model's streaming)

The auto-typed storyboard runs hands-free, so the capture is clean:

```bash
asciinema rec --idle-time-limit 2 --cols 100 --rows 24 \
  -c 'NS=ai SVC=qwen3 ./hack/demo/demo.sh' demo.cast
agg --font-size 20 --speed 1.4 demo.cast docs/demo.gif
gifsicle -O3 --lossy=80 docs/demo.gif -o docs/demo.gif   # shrink to < ~3 MB
```

`--idle-time-limit 2` compresses the cold-start wait; the gateway's `: heartbeat`
lines still show during the hold — which is exactly the behavior worth demoing.

## Tips

- **Keep it ~30–45s.** The hook is `0 → request → tokens`; scale-back-to-zero can be
  a closing line if the cooldown is long.
- **Terminal:** ~100×24, large font, high-contrast theme; close other panes.
- **Readable streaming (optional):** pipe `curl` through a filter to show just the text
  instead of raw SSE JSON:
  `... | sed -u 's/^data: //; /\[DONE\]/d' | jq -rj 'try .choices[0].delta.content // empty'`
- **Reuse it everywhere:** README hero, the GitHub release, and every launch post. For a
  longer narrated walkthrough (audio is fine there), upload to YouTube / Bilibili and link it.

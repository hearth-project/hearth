# Observability

Hearth exposes Prometheus metrics from both the always-on gateway and the selected backend runtime.
The bundled Grafana dashboard at [`config/grafana/hearth-overview.json`](../config/grafana/hearth-overview.json)
visualizes queue depth, request outcomes, cold-start wait, and vLLM runtime signals.

## Import the Grafana dashboard

1. Open Grafana and go to **Dashboards** > **New** > **Import**.
2. Upload or paste [`config/grafana/hearth-overview.json`](../config/grafana/hearth-overview.json).
3. Select the Prometheus data source for the dashboard's `DS_PROMETHEUS` input.
4. Import the dashboard.

The dashboard expects Prometheus to scrape Hearth gateway metrics and the backend vLLM `/metrics`
endpoint. When the Prometheus Operator CRD is installed, Hearth renders a `ServiceMonitor` for each
`LLMService` that selects both the gateway and backend services with
`serving.hearth.dev/llmservice=<llmservice-name>` and scrapes their shared `http` port at `/metrics`.

## Gateway metrics

The gateway exports these metrics on `/metrics`:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `hearth_gateway_pending` | Gauge | none | Requests admitted and waiting or in flight (the scaler's demand signal). |
| `hearth_gateway_requests_total` | Counter | `code` | Responses by HTTP status code. |
| `hearth_gateway_rejections_total` | Counter | `reason` | Rejected requests by reason. |
| `hearth_gateway_activation_wait_seconds` | Histogram | none | Time spent holding a request until the backend was ready. |

`hearth_gateway_rejections_total` uses reasons such as `queue_full`, `cold_start`, and
`activation_timeout`. The activation wait histogram is exposed with the standard Prometheus
`_bucket`, `_sum`, and `_count` series.

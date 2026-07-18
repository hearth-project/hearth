# Observability

Hearth exposes Prometheus metrics from both the always-on gateway and the selected backend runtime.
It does not install or reconcile Prometheus, Grafana, or Prometheus Operator resources. Monitoring
can therefore be deployed, replaced, or removed without changing the serving control plane.

The generated gateway and backend Services expose a named `http` port and carry both of these
stable discovery labels:

```text
app.kubernetes.io/managed-by=hearth
serving.hearth.dev/llmservice=<llmservice-name>
```

## Install the optional ServiceMonitor

The independent [`examples/observability/prometheus`](../examples/observability/prometheus) profile
selects both Services in one workload namespace and scrapes `/metrics` every 15 seconds. Install the
Prometheus Operator separately, then apply the profile once in each namespace containing
`LLMService` objects:

```bash
kubectl apply -k examples/observability/prometheus -n ai
```

The Prometheus instance must be configured to discover `ServiceMonitor` objects in that namespace
and accept the profile's labels. The example matches the bundled runtimes, which expose metrics at
`/metrics`; customize it for a runtime that uses a different path. Hearth serving and autoscaling
continue normally when the profile or Prometheus Operator is absent.

## Import the Grafana dashboard

1. Open Grafana and go to **Dashboards** > **New** > **Import**.
2. Upload or paste
   [`examples/observability/grafana/hearth-overview.json`](../examples/observability/grafana/hearth-overview.json).
3. Select the Prometheus data source for the dashboard's `DS_PROMETHEUS` input.
4. Import the dashboard.

The dashboard expects Prometheus to scrape Hearth gateway metrics and the backend vLLM `/metrics`
endpoint through the optional discovery configuration above.

## Remove legacy controller-owned monitors

Hearth versions before monitoring was decoupled created one `ServiceMonitor` per `LLMService`.
Those resources retain owner references and disappear when their service is deleted, but an upgrade
does not otherwise remove them. Delete legacy monitors when adopting the independent profile to
avoid duplicate scrape targets:

```bash
kubectl delete servicemonitors.monitoring.coreos.com -A \
  -l 'app.kubernetes.io/managed-by=hearth,serving.hearth.dev/llmservice'
```

## Gateway metrics

The gateway exports these metrics on `/metrics`:

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `hearth_gateway_pending` | Gauge | none | Requests admitted and waiting or in flight. |
| `hearth_gateway_demand` | Gauge | none | Effective queue value reported to KEDA, including the activation-lease floor. |
| `hearth_gateway_requests_total` | Counter | `code` | Responses by HTTP status code. |
| `hearth_gateway_rejections_total` | Counter | `reason` | Rejected requests by reason. |
| `hearth_gateway_activation_wait_seconds` | Histogram | none | Time spent holding a request until the backend was ready. |
| `hearth_gateway_scaler_streams` | Gauge | none | Connected KEDA external-push activation streams. |
| `hearth_gateway_activation_events_total` | Counter | none | Inactive-to-active effective-demand transitions. |

`hearth_gateway_rejections_total` uses reasons such as `queue_full`, `cold_start`, and
`activation_timeout`. The activation wait histogram is exposed with the standard Prometheus
`_bucket`, `_sum`, and `_count` series.

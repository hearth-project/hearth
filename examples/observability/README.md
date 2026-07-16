# Optional observability

These assets are independent of Hearth's reconciliation and release lifecycle. Hearth exposes
metrics and stable Service labels, but it does not install Prometheus, Grafana, or Prometheus
Operator resources.

## Prometheus Operator

The [`prometheus`](prometheus) profile creates one `ServiceMonitor` in a workload namespace. It
selects every Hearth-managed gateway and backend Service in that namespace through
`app.kubernetes.io/managed-by=hearth` and `serving.hearth.dev/llmservice`, then scrapes the named
`http` port at `/metrics`.

Install the Prometheus Operator separately, then apply the profile once in each namespace that
contains `LLMService` objects:

```bash
kubectl apply -k examples/observability/prometheus -n ai
```

Your Prometheus instance must select `ServiceMonitor` objects from that namespace and must not
filter out the profile's labels. If a custom runtime exposes metrics on another path, copy and
adjust the example. Removing this profile does not affect serving or autoscaling.

## Grafana

Import [`grafana/hearth-overview.json`](grafana/hearth-overview.json) and select the Prometheus data
source requested by the dashboard.

## Upgrading from automatic ServiceMonitors

Earlier Hearth versions created one `ServiceMonitor` per `LLMService`. They are owner-referenced
and disappear when their `LLMService` is deleted, but an upgrade does not otherwise remove them.
Remove legacy monitors when adopting this profile to avoid duplicate scrape targets:

```bash
kubectl delete servicemonitors.monitoring.coreos.com -A \
  -l 'app.kubernetes.io/managed-by=hearth,serving.hearth.dev/llmservice'
```

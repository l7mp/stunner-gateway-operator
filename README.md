<p align="center">
  <img alt="STUNner", src="doc/stunner.svg" width="50%" height="50%"></br>
  <a href="https://discord.gg/DyPgEsbwzc" alt="Discord">
    <img alt="Discord" src="https://img.shields.io/discord/945255818494902282" /></a>
  <a href="https://hub.docker.com/repository/docker/l7mp/stunner-gateway-operator" alt="Docker pulls">
    <img src="https://img.shields.io/docker/pulls/l7mp/stunner-gateway-operator" /></a>
  <a href="https://github.com/l7mp/stunner-gateway-operator/blob/main/LICENSE" alt="MIT">
    <img src="https://img.shields.io/github/license/l7mp/stunner-gateway-operator" /></a>
</p>

# STUNner Kubernetes Gateway Operator

The STUNner Kubernetes Gateway Operator is an open-source implementation of the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io) using [STUNner](https://github.com/l7mp/stunner) as the data plane. The goal is to implement the part of the core Gateway API, namely Gateway, GatewayClass and UDPRoute resources, that are necessary to fully configure the STUNner WebRTC ingress gateway via the Kubernetes control plane. The STUNner Kubernetes Gateway Operator is currently supports only a subset of the Gateway API.

## Documentation

Full documentation for the stable version can be found [here](https://docs.l7mp.io/en/stable). The documentation of the development version is maintained [here](https://github.com/l7mp/stunner/blob/main/docs/README.md).

## Configuration: Flags and Environment Variables

The operator supports both command-line flags and environment variables.

- `--controller-name` can be set directly or the environment var `STUNNER_GATEWAY_OPERATOR_CONTROLLER_NAME`.
- `--dataplane-mode` can be set directly or the environment var `STUNNER_GATEWAY_OPERATOR_DATAPLANE_MODE`.
- `--config-discovery-address` can be set directly or the environment var `STUNNER_GATEWAY_OPERATOR_ADDRESS`.
- `--pprof-bind-address` can be set directly or the environment var `STUNNER_GATEWAY_OPERATOR_PPROF_BIND_ADDRESS`.
- `CUSTOMER_KEY` is read from the environment for licensing.

Command-line flags take precedence over environment variables. 

### Debug profiling (pprof)

Pprof is disabled by default (`--pprof-bind-address=0`). For debugging, prefer binding to `127.0.0.1:6060` and use `kubectl port-forward` to access `/debug/pprof/` endpoints.

Example:

1. Enable pprof in the operator args by adding `--pprof-bind-address=127.0.0.1:6060` to the operator's startup command.
2. Create a port-forward to the operator pod:
   ```console
   kubectl -n stunner-system port-forward $(kubectl get pod -A -l control-plane=stunner-gateway-operator-controller-manager -o jsonpath='{.items[0].metadata.name}') 6060:6060
   ```
3. Inspect goroutine stacks:
   ```console
   curl -s "http://127.0.0.1:6060/debug/pprof/goroutine?debug=2"
   ```
4. Or open interactive `pprof` UI:
   ```console
   go tool pprof -http=:8081 "http://127.0.0.1:6060/debug/pprof/profile?seconds=30"
   ```

Do not expose pprof publicly, profiles may contain sensitive runtime details.

### Metrics

Prometheus metrics are served at `--metrics-bind-address` (default `:8080/metrics`).

**Controller-runtime built-ins** (most useful for day-to-day monitoring):

| Metric                                                  | Type      | Description                                                                                       |
|---------------------------------------------------------|-----------|---------------------------------------------------------------------------------------------------|
| `controller_runtime_reconcile_total{controller,result}` | Counter   | Reconciliations per controller, split by result (`success`, `error`, `requeue`, `requeue_after`). |
| `controller_runtime_reconcile_time_seconds{controller}` | Histogram | Per-reconciliation latency.                                                                       |
| `controller_runtime_reconcile_errors_total{controller}` | Counter   | Reconciliation errors per controller.                                                             |
| `controller_runtime_active_workers{controller}`         | Gauge     | Workers currently processing a reconcile request.                                                 |

**Operator-specific metrics** (prefix `stunner_gateway_operator_`):

| Metric                                            | Type      | Description                                                                                                                                                                |
|---------------------------------------------------|-----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `render_total`                                    | Counter   | Render cycles completed by the renderer thread.                                                                                                                            |
| `render_time_seconds`                             | Histogram | Duration of a full render cycle.                                                                                                                                           |
| `update_total{result}`                            | Counter   | Update cycles completed by the updater thread (`success` / `error`).                                                                                                       |
| `update_errors_total`                             | Counter   | Update cycles that returned an error.                                                                                                                                      |
| `update_time_seconds`                             | Histogram | Duration of a full update cycle.                                                                                                                                           |
| `resource_operations_total{scope,kind,operation}` | Counter   | Individual Kubernetes API calls made by the updater, labelled by scope (`spec`/`status`), resource kind, and operation (`created`, `updated`, `error`, `suppressed`, ...). |
| `reconcile_events_total{result}`                  | Counter   | Reconcile events received by the operator event loop (`passed` when a render is scheduled, `throttled` when rate-limited).                                                 |
| `generation`                                      | Gauge     | Current config generation number.                                                                                                                                          |
| `generation_last_acked`                           | Gauge     | Generation number of the last update acknowledged by the updater.                                                                                                          |

## Caveats

* STUNner implements its own UDPRoute resource instead of using the official UDPRoute provided by the Gateway API. The reason is that STUNner's UDPRoutes omit the port defined in backend references, in contrast to standard UDPRoutes that make the port mandatory. The rationale is that WebRTC media servers typically spawn zillions of UDP/SRTP listeners on essentially any UDP port, so enforcing a single backend port would block all client access. Instead, STUNner's UDPRoutes do not limit port access on backend services at all by default, and provide an optional pair or port/end-port fields per backend reference to define a target port range in which peer connections to the backend are to be accepted.
* The operator actively reconciles the changes in the GatewayClass resource; e.g., if the `parametersRef` changes then we take this into account (this is not recommended in the spec to [limit the blast radius of a mistaken config update](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClassSpec)).
* ReferenceGrants are not implemented: routes can refer to Services and StaticServices in any namespace.
* The operator does not invalidate the GatewayClass status on exit and does not handle the case when the parent GatewayClass is removed from Gateway.

## Help

STUNner development is coordinated in Discord, feel free to [join](https://discord.gg/DyPgEsbwzc).

## License

Copyright 2021-2026 by its authors. Some rights reserved. See [AUTHORS](https://github.com/l7mp/stunner/blob/main/AUTHORS).

APACHE License - see [LICENSE](/LICENSE) for full text.

## Acknowledgments

Inspired from the [NGINX Kubernetes Gateway](https://github.com/nginxinc/nginx-kubernetes-gateway) and the [Kong Gateway Operator](https://github.com/Kong/gateway-operator).

# Leader/standby configuration discovery

`--leader-discovery-service=<service-name>` is opt-in. It requires `--leader-elect`,
managed dataplane mode, and disabled finalization. Leave it unset to retain the
existing discovery behavior. The companion [Helm change](https://github.com/l7mp/stunner-helm/pull/72)
provides the Service, namespaced RBAC, two replicas, anti-affinity and PDB.

Provide `POD_NAMESPACE`, `POD_NAME`, `POD_UID` and `POD_IP` from the Downward API.
Set `STUNNER_GATEWAY_OPERATOR_ADDRESS` to the discovery Service DNS name. Its TCP
port must match `--config-discovery-address` (13478 by default), with port name
`cds`. The Service must have no selector and exactly one IP family, matching the
Pod IP. IPv4 and IPv6 single-stack are supported; dual-stack is rejected.

The manager warms both replicas' caches. After election, the operator explicitly
reconciles GatewayConfigs, Dataplanes, Gateways, routes and Nodes before allowing
any render. Initial reconciliation is serialized with regular controller work.
Even an empty cluster produces a complete snapshot. The CDS store consumes that
snapshot before its listener opens. Only then does the leader create or update
`<service-name>-leader`, containing its own Pod IP and UID. The publisher uses an
uncached client and resource-version concurrency checks, avoids unchanged writes,
and refuses to overwrite a slice owned by a different Service or controller.

Process readiness is independent of serving: a synchronized standby is Ready
without listening for CDS connections; a leader is Ready only after configuration
initialization and endpoint publication. This lets Deployments and PDBs count both
healthy replicas. Watch connections and the listener close when the manager stops
or the process receives SIGTERM. On lost leadership controller-runtime stops the
manager and the process exits. The exiting process never clears the EndpointSlice,
which may already belong to its successor.

This uses the existing Kubernetes Lease election, including its renewal/expiration
timings and process-failure assumptions. It does not provide instantaneous takeover
or a consensus fence against arbitrarily paused processes/clock-rate violations.
The old leader must stop serving when its lease renewal fails; verify this under
API isolation while keeping its CDS port reachable. The successor takes a fresh
snapshot from its synchronized cache, serves it, and replaces the endpoint. Existing
gateways retain their last configuration and reconnect without a Pod-template change.

For a manual installation, grant EndpointSlice `create` in the operator namespace
and `get,update` for `<service-name>-leader`, in addition to the existing operator
permissions. Keep both replicas in the same election namespace and use the same
controller identity. Do not share the Service with another controller or leave
other EndpointSlices routing to uninitialized operators.

Adoption requires stopping the old operators, removing the discovery Service's
selector and old automatically managed EndpointSlices, and starting two compatible
operators. Do not mix old always-serving operators with this mode. Reversing that
transition also requires stopping both replicas first. First adoption of a stable
Service address changes gateway templates once; plan and validate that rollout.
Operator failover does not preserve allocations when a TURN gateway itself dies.

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

## Caveats

* STUNner implements its own UDPRoute resource instead of using the official UDPRoute provided by the Gateway API. The reason is that STUNner's UDPRoutes omit the port defined in backend references, in contrast to standard UDPRoutes that make the port mandatory. The rationale is that WebRTC media servers typically spawn zillions of UDP/SRTP listeners on essentially any UDP port, so enforcing a single backend port would block all client access. Instead, STUNner's UDPRoutes do not limit port access on backend services at all by default, and provide an optional pair or port/end-port fields per backend reference to define a target port range in which peer connections to the backend are to be accepted.
* The operator actively reconciles the changes in the GatewayClass resource; e.g., if the `parametersRef` changes then we take this into account (this is not recommended in the spec to [limit the blast radius of a mistaken config update](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClassSpec)).
* ReferenceGrants are not implemented: routes can refer to Services and StaticServices in any namespace.
* The operator does not invalidate the GatewayClass status on exit and does not handle the case when the parent GatewayClass is removed from Gateway.

## Help

STUNner development is coordinated in Discord, feel free to [join](https://discord.gg/DyPgEsbwzc).

## License

Copyright 2021-2023 by its authors. Some rights reserved. See [AUTHORS](https://github.com/l7mp/stunner/blob/main/AUTHORS).

APACHE License - see [LICENSE](/LICENSE) for full text.

## Acknowledgments

Inspired from the [NGINX Kubernetes Gateway](https://github.com/nginxinc/nginx-kubernetes-gateway) and the [Kong Gateway Operator](https://github.com/Kong/gateway-operator).

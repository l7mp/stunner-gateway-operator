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

The STUNner Kubernetes Gateway Operator is an open-source implementation of the [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io) using [STUNner](https://github.com/l7mp/stunner) as the data plane. The goal is to implement the part of the core Gateway API -- Gateway, GatewayClass, and UDPRoute resources -- necessary to fully configure the STUNner WebRTC ingress gateway via the Kubernetes control plane. The STUNner Kubernetes Gateway Operator is currently under development and supports a subset of the Gateway API.

> Warning: This project is in active development, consider this before deploying it in a production environment.  All APIs, SDKs, and packages are subject to change.

## Documentation

Full [documentation](https://github.com/l7mp/stunner/blob/main/README.md) can be found in the main STUNner [GitHub repo](https://github.com/l7mp/stunner).

## Caveats

* The operator omits the Port in UDPRoutes and the PortNumber in BackendObjectReferences and ParentReferences. This is because our target services typically span WebRTC media server pools and these may spawn a UDP/SRTP listener for essentially any arbitrary port. Eventually we would need to implement a CustomUDPRoute CRD that would allow the user to specify a port range (just like NetworkPolicies), until then the operator silently ignores ports on routes, services and endpoints.
* The operator actively reconciles the changes in the GatewayClass resource; e.g., if the ParametersRef changes then we take this into account (this is not recommended in the spec to [limit the blast radius of a mistaken config update](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClassSpec)).
* ReferenceGrants are not implemented: routes can refer to Services in any namespace.
* There is no infratructure to handle the case when a GatewayConfig that is being referred to from a GatewayClass, and is being actively rendered by the operator, is deleted. The controller loses the info on the render target and can never invalidate the corresponding STUNner configuration. This will be fixed once we implement managed dataplane support.
* The operator does not invalidate the GatewayClass status on exit.

## Help

STUNner development is coordinated in Discord, feel free to [join](https://discord.gg/DyPgEsbwzc).

## License

Copyright 2021-2023 by its authors. Some rights reserved. See
[AUTHORS](https://github.com/l7mp/stunner/blob/main/AUTHORS).

APACHE License - see [LICENSE](/LICENSE) for full text.

## Acknowledgments

Inspired from the [NGINX Kubernetes Gateway](https://github.com/nginxinc/nginx-kubernetes-gateway)
and the [Kong Gateway Operator](https://github.com/Kong/gateway-operator).

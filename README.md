<p align="center">
  <img alt="STUNner", src="doc/stunner.svg" width="50%" height="50%"></br>
</p>

# STUNner Kubernetes Gateway Operator

The STUNner Kubernetes Gateway Operator is an open-source implementation of the [Kubernetes Gateway
API](https://gateway-api.sigs.k8s.io) using [STUNner](https://github.com/l7mp/stunner) as the data
plane. The goal is to implement the part of the core Gateway API -- Gateway, GatewayClass, and
UDPRoute resources -- necessary to fully configure the STUNner WebRTC ingress gateway via the
Kubernetes control plane. The STUNner Kubernetes Gateway Operator is currently under development
and supports a subset of the Gateway API.

> Warning: This project is in active development (pre-alpha feature state), consider this before
> deploying it in a production environment.  All APIs, SDKs, and packages are subject to change.

## Documentation

Full [documentation](https://github.com/l7mp/stunner/blob/main/README.md) can be found in the main
STUNner [GitHub repo](https://github.com/l7mp/stunner).

<!-- # Run the STUNner Kubernetes Gateway Operator -->

<!-- ## Prerequisites -->

<!-- Before you can build and run the STUNner Kubernetes Gateway Operator, make sure you have the -->
<!-- following software installed on your machine: -->
<!-- - [git](https://git-scm.com/) -->
<!-- - [GNU Make](https://www.gnu.org/software/software.html) -->
<!-- - [Docker](https://www.docker.com/) or [podman](https://podman.io) -->
<!-- - [kubectl](https://kubernetes.io/docs/tasks/tools/) -->

<!-- ## Deploy the STUNner dataplane -->

<!-- The STUNner daemon will serve as the data-plane to ingest media traffic into the cluster; refer to -->
<!-- the [STUNner documentation](https://github.com/l7mp/stunner/blob/main/doc/README.md) for more detail. -->

<!-- 1. Create a namespace called `stunner` that will host all Kubernetes resources related to STUNner. -->

<!--    ``` console -->
<!--    kubectl create namespace stunner -->
<!--    ``` -->

<!-- 1. Deploy the STUNner gateway: this will serve as the data-plane to ingest your WebRTC traffic into -->
<!--    the Kubernetes cluster: -->

<!--    ``` console -->
<!--    helm repo add stunner https://l7mp.io/stunner -->
<!--    helm repo update -->
<!--    helm install stunner stunner/stunner --set stunner.namespace=stunner -->
<!--    ``` -->

<!-- 1. Restart STUNner to pick up the configuration that will be rendered by the operator (to be -->
<!--    configured next). The operator will be in charge of watching the Gateway API resources created -->
<!--    by the user in the Kubernetes control plane (i.e., via kubectl-applying various YAMLs) and -->
<!--    creating a configuration file for the STUNner data-plane pods into a ConfigMap. This config-map -->
<!--    is then mapped into the filesystem of the STUNner pods as a configmap volume, so that the -->
<!--    STUNner daemons can reconcile the new configuration according to the policies specified by the -->
<!--    user. -->

<!--    In order to do that, we have to restart the STUNner data-plane using the below manifest. The -->
<!--    `-w` command line argument switches the STUNner daemon into watch mode: the daemon will get -->
<!--    notified by Kubernetes whenever the operator renders a new configuration into the ConfigMap -->
<!--    (e.g., when a Gateway or a UDPRoute changes) so that it can reconcile the most up-to-date -->
<!--    configuration. -->

<!--    ```console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: apps/v1 -->
<!--    kind: Deployment -->
<!--    metadata: -->
<!--      name: stunner -->
<!--      namespace: stunner -->
<!--    spec: -->
<!--      selector: -->
<!--        matchLabels: -->
<!--          app: stunner -->
<!--      template: -->
<!--        metadata: -->
<!--          labels: -->
<!--            app: stunner -->
<!--        spec: -->
<!--          containers: -->
<!--            - command: ["stunnerd"] -->
<!--              args: ["-w", "-c", "/etc/stunnerd/stunnerd.conf"] -->
<!--              image: l7mp/stunnerd:latest -->
<!--              imagePullPolicy: Always -->
<!--              name: stunnerd -->
<!--              env: -->
<!--                - name: STUNNER_ADDR -->
<!--                  valueFrom: -->
<!--                    fieldRef: -->
<!--                      apiVersion: v1 -->
<!--                      fieldPath: status.podIP -->
<!--              volumeMounts: -->
<!--                - name: stunnerd-config-volume -->
<!--                  mountPath: /etc/stunnerd -->
<!--          volumes: -->
<!--            - name: stunnerd-config-volume -->
<!--              configMap: -->
<!--                name: stunnerd-configmap -->
<!--    EOF -->
<!--    ``` -->

<!-- ## Build the control-plane operator image -->

<!-- 1. Clone the STUNner gateway operator git repo and enter into the root directory: -->

<!--    ``` console -->
<!--    git clone https://github.com/l7mp/stunner-gateway-operator.git -->
<!--    cd stunner-gateway-operator -->
<!--    ``` -->

<!-- 1. Build the image, either with Docker of [podman](https://podman.io) (requires `sudo`): -->

<!--    ``` console -->
<!--    IMG=<my-image> make podman-build -->
<!--    ``` -->

<!-- 1. Push the image to your container registry: -->

<!--    ``` console -->
<!--    IMG=<my-image> make podman-push -->
<!--    ``` -->

<!-- ## Deploy the operator -->

<!-- You can deploy the STUNner Kubernetes Gateway Operator on an existing Kubernetes 1.22+ cluster. The -->
<!-- following instructions walk through the steps for deploying on a [kind](https://kind.sigs.k8s.io/) -->
<!-- cluster. -->

<!-- 1. Install the Kubernetes Gateway CRDs from the official source (these are not part of the STUNner -->
<!--    distribution). The operator targets version 0.4.3 of the Gateway `v1alpha2` API: -->

<!--    ``` console -->
<!--    kubectl apply -k "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.3" -->
<!--    ``` -->

<!-- 1. Deploy the STUNner Kubernetes Gateway Operator: -->

<!--    ``` console -->
<!--    make install -->
<!--    make deploy -->
<!--    ``` -->

<!-- 1. Confirm the operator is running in `stunner-gateway` namespace: -->

<!--    ``` console -->
<!--    kubectl get pods -n stunner-gateway-operator-system -->
<!--    NAME                                                          READY   STATUS    RESTARTS   AGE -->
<!--    stunner-gateway-operator-controller-manager-65dbf8fb4-hjrjr   2/2     Running   0          42m -->
<!--    ``` -->

<!-- ## Create a UDP echo service -->

<!-- For the sake if this demo, we create a UDP echo service that we will expose through STUNner to our -->
<!-- clients.  In a real-use of STUNner, the target service would be, for instance, a WebRTC media -->
<!-- servers pool or an SFU. -->

<!-- 1. Fire up the UDP echo server from the [STUNner UDP tunnel -->
<!--    demo](https://github.com/l7mp/stunner/blob/main/examples/simple-tunnel): -->

<!--    ``` console -->
<!--    kubectl create deployment -n stunner udp-echo --image=l7mp/net-debug:latest -->
<!--    kubectl expose deployment -n stunner  udp-echo --name=udp-echo --type=ClusterIP --protocol=UDP --port=9001 -->
<!--    kubectl exec -it -n stunner $(kubectl get pod -l app=udp-echo -n stunner -o jsonpath="{.items[0].metadata.name}") -- \ -->
<!--         socat -d -d udp-l:9001,fork EXEC:"echo Greetings from STUNner!" -->
<!--    ``` -->

<!-- ## Configure the operator -->

<!-- The STUNner operator (partially) implements the official Kubernetes [Gateway -->
<!-- API](https://gateway-api.sigs.k8s.io), which allows you to interact with STUNner using the -->
<!-- convenience of `kubectl` and declarative YAML configuration. Below we configure a minimal STUNner -->
<!-- gateway setup that exposes the UDP echo server we just fired up above via the STUNner gateway as a -->
<!-- standard STUN/TURN service, over the conventional TURN port UDP:3478. -->

<!-- 1. Create a -->
<!--    [GatewayClass](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClass). This -->
<!--    will serve as the root level configuration for your STUNner deployment and specifies the name -->
<!--    and the description of the service implemented by the GatewayClass, as well as a Kubernetes -->
<!--    resource (the `GatewayConfig` resource given under the `parametersRef`) that will define some -->
<!--    general parameters for the data-plane implementing the GatewayClass. -->

<!--    ``` console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: gateway.networking.k8s.io/v1alpha2 -->
<!--    kind: GatewayClass -->
<!--    metadata: -->
<!--      name: stunner-gatewayclass -->
<!--    spec: -->
<!--      controllerName: "stunner.l7mp.io/gateway-operator" -->
<!--      parametersRef: -->
<!--        group: "stunner.l7mp.io" -->
<!--        kind: GatewayConfig -->
<!--        name: stunner-gatewayconfig -->
<!--        namespace: stunner -->
<!--      description: "STUNner is a WebRTC ingress gateway for Kubernetes" -->
<!--    EOF -->
<!--    ``` -->

<!-- 1. Next, we specify some important configuration for STUNner, by loading a `GatewayConfig` custom -->
<!--    resource into Kubernetes. Make sure to use the `stunner` namespace we have just created; this -->
<!--    will be the target namespace where the operator will render the running STUNner data-plane -->
<!--    configuration. -->

<!--    Make sure to customize the authentication mode and credentials used for STUNner; consult the -->
<!--    [STUNner authentication guide](https://github.com/l7mp/stunner/blob/main/doc/AUTH.md) to -->
<!--    understand how to set the realm and the authentication type and credentials below: -->

<!--    ```console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: stunner.l7mp.io/v1alpha1 -->
<!--    kind: GatewayConfig -->
<!--    metadata: -->
<!--      name: stunner-gatewayconfig -->
<!--      namespace: stunner -->
<!--    spec: -->
<!--      stunnerConfig: "stunnerd-configmap" -->
<!--      realm: stunner.l7mp.io -->
<!--      authType: plaintext -->
<!--      userName: "user-1" -->
<!--      password: "pass-1" -->
<!--    EOF -->
<!--    ``` -->

<!-- 1. Create your first STUNner -->
<!--    [Gateway](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.Gateway). -->
<!--    The below Gateway specification will expose the STUNner gateway over the STUN/TURN listener -->
<!--    service running on the UDP listener port 3478.  STUnner will await clients to connect to this -->
<!--    listener port and, once authenticated, let them connect to the services running inside the -->
<!--    Kubernetes cluster; meanwhile, the NAT traversal functionality implemented by the STUN/TURN -->
<!--    server embedded into STUNner will make sure that clients can connect from behind even the most -->
<!--    over-zealous enterprise NAT or firewall. -->

<!--    ```console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: gateway.networking.k8s.io/v1alpha2 -->
<!--    kind: Gateway -->
<!--    metadata: -->
<!--      name: udp-gateway -->
<!--      namespace: stunner -->
<!--    spec: -->
<!--      gatewayClassName: stunner-gatewayclass -->
<!--      listeners: -->
<!--        - name: udp-listener -->
<!--          port: 3478 -->
<!--          protocol: UDP -->
<!--    EOF -->
<!--    ``` -->

<!-- 1. Finally, attach a [UDP -->
<!--    route](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.UDPROute) -->
<!--    to the Gateway, so that clients will be able to connect via the public STUN/TURN listener -->
<!--    UDP:3478 to the UDP echo service. -->

<!--    ```console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: gateway.networking.k8s.io/v1alpha2 -->
<!--    kind: UDPRoute -->
<!--    metadata: -->
<!--      name: udp-echo -->
<!--      namespace: stunner -->
<!--    spec: -->
<!--      parentRefs: -->
<!--        - name: udp-gateway -->
<!--      rules: -->
<!--        - backendRefs: -->
<!--            - name: udp-echo -->
<!--    EOF -->
<!--    ``` -->

<!-- 1. Check the result: the operator should have rendered a valid and up to date STUNner configuration -->
<!--    in the ConfigMap you specified in the above GatewayConfig (called `stunnerd-configmap` in our -->
<!--    example), in the same namespace where the root GatewayConfig lives. -->

<!--    ```console -->
<!--    kubectl get cm -n stunner stunnerd-configmap -o yaml -->
<!--    apiVersion: v1 -->
<!--    kind: ConfigMap -->
<!--    metadata: -->
<!--      name: stunnerd-configmap -->
<!--      namespace: stunner -->
<!--    data: -->
<!--      stunnerd.conf: '{"version":"v1alpha1","admin":{"name":"stunner-daemon","loglevel":"all:INFO"},"auth":{"type":"plaintext","realm":"stunner.l7mp.io","credentials":{"password":"pass-1","username":"user-1"}},"listeners":[{"name":"udp-listener","protocol":"UDP","public_address":"34.116.220.190","public_port":3478,"address":"$STUNNER_ADDR","port":3478,"min_relay_port":32768,"max_relay_port":65535,"routes":["udp-echo"]}],"clusters":[{"name":"udp-echo","type":"STRICT_DNS","endpoints":["udp-echo.stunner.svc.cluster.local"]}]}' -->
<!--    ``` -->

<!--    The data under the key `stunnerd.conf` is the STUNner configuration rendered by the -->
<!--    operator. Pretty-printing the JSON content will look something like the below: -->

<!--    ```yaml -->
<!--    { -->
<!--      "version": "v1alpha1", -->
<!--      "admin": { -->
<!--        "name": "stunner-daemon", -->
<!--        "loglevel": "all:INFO" -->
<!--      }, -->
<!--      "auth": { -->
<!--        "type": "plaintext", -->
<!--        "realm": "stunner.l7mp.io", -->
<!--        "credentials": { -->
<!--          "password": "pass-1", -->
<!--          "username": "user-1" -->
<!--        } -->
<!--      }, -->
<!--      "listeners": [ -->
<!--        { -->
<!--          "name": "udp-listener", -->
<!--          "protocol": "UDP", -->
<!--          "public_address": "34.116.220.190", -->
<!--          "public_port": 3478, -->
<!--          "address": "$STUNNER_ADDR", -->
<!--          "port": 3478, -->
<!--          "min_relay_port": 32768, -->
<!--          "max_relay_port": 65535, -->
<!--          "routes": [ -->
<!--            "udp-echo" -->
<!--          ] -->
<!--        } -->
<!--      ], -->
<!--      "clusters": [ -->
<!--        { -->
<!--          "name": "udp-echo", -->
<!--          "type": "STRICT_DNS", -->
<!--          "endpoints": [ -->
<!--            "udp-echo.stunner.svc.cluster.local" -->
<!--          ] -->
<!--        } -->
<!--      ] -->
<!--    } -->
<!--    ``` -->

<!-- ## Send a request via STUNner -->

<!-- 1. In order for clients to be able to connect to our UDP echo service, they need to know the public -->
<!--    IP address and port associated with the Gateway we have created above. In order to simplify -->
<!--    this, the STUNner gateway operator automatically exposes all Gateways in standard Kubernetes -->
<!--    LoadBalancer services over a publicly available IP address and port. The name of the service is -->
<!--    using the template `stunner-gateway-<YOUR_GATEWAY_NAME>-svc` and it will always be created in -->
<!--    the same namespace as the Gateway. The corresponding public IP and port for each listener can be -->
<!--    learned from the External IP field for the service; for instance, in the below example -->
<!--    Kubernetes assigned the IP-pot pair 34.118.16.31:3478 for the UDP listener -->

<!--    ```console -->
<!--    kubectl get svc -n stunner -->
<!--    NAME                              TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)          AGE -->
<!--    stunner-gateway-udp-gateway-svc   LoadBalancer   10.120.13.130   34.116.220.190   3478:30398/UDP   21m -->
<!--    udp-echo                          ClusterIP      10.120.0.28     <none>           9001/UDP         3d22h -->
<!--    ``` -->

<!--    Observe how the `udp-echo` service does not have an externally reachable IP/port; the only way -->
<!--    to reach this service from the Internet is via STUNner over STUN/TURN. You can now easily -->
<!--    substitute the UDP echo service with your WebRTC service and imagine how STUNner would work in -->
<!--    your media plane. -->

<!--    Note that, for convenience, the operator readily includes the public IP and port for each -->
<!--    STUNner listener in the STUNner configuration file it creates (under the keys `public_address` -->
<!--    and `public_port`). -->

<!-- 1. Memoize the IP addresses and ports to be used to reach the UDP echo server behind STUNner: -->

<!--    ```console -->
<!--    export STUNNER_PUBLIC_ADDR=$(kubectl get svc -n stunner stunner-gateway-udp-gateway-svc \ -->
<!--        -o jsonpath='{.status.loadBalancer.ingress[0].ip}') -->
<!--    export STUNNER_PUBLIC_PORT=$(kubectl get svc -n stunner stunner-gateway-udp-gateway-svc \ -->
<!--        -o jsonpath='{.spec.ports[0].port}') -->
<!--    export UDP_ECHO_IP=$(kubectl get svc -n stunner udp-echo -o jsonpath='{.spec.clusterIP}') -->
<!--    ``` -->

<!-- 1. Fire up a local [`turncat`](https://github.com/l7mp/stunner/blob/main/cmd/turncat) client to -->
<!--    tunnel the UDP port `localhost:9000` to the UDP service: -->

<!--    ```console -->
<!--    cd stunner -->
<!--    go run cmd/turncat/main.go --log=all:DEBUG udp://127.0.0.1:9000 \ -->
<!--        turn://user-1:pass-1@${STUNNER_PUBLIC_ADDR}:${STUNNER_PUBLIC_PORT} \ -->
<!--        udp://${UDP_ECHO_IP}:9001 -->
<!--    ``` -->

<!-- 1. And finally open a local `socat` and send anything to the UDP echo server: you should see it -->
<!--    echoing back a nice greeting: -->

<!--    ```console -->
<!--    echo "Hello STUNner" | socat - udp:localhost:9000 -->
<!--    Greetings from STUNner! -->
<!--    ``` -->

<!-- ## Add a TCP listener to the Gateway -->

<!-- Suppose your clients report that they cannot reach your fancy UDP echo service exposed via the -->
<!-- public STUNner UDP Gateway due to, say, an overly restrictive enterprise firewall/NAT. No problem -->
<!-- for STUNner: we can easily set up a new TCP Gateway that will accept connections over the port -->
<!-- TCP:3478 and route the client connection requests received on this listener to the same UDP echo -->
<!-- service. Note that STUNner will conveniently handle the TCP bytestream received over the TCP -->
<!-- listener and convert into a message-stream as expected by the UDP echo service. -->

<!-- 1. Create a new -->
<!--    [Gateway](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.Gateway), -->
<!--    but this time with a listener over TCP:3478. -->

<!--    ```console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: gateway.networking.k8s.io/v1alpha2 -->
<!--    kind: Gateway -->
<!--    metadata: -->
<!--      name: tcp-gateway -->
<!--      namespace: stunner -->
<!--    spec: -->
<!--      gatewayClassName: stunner-gatewayclass -->
<!--      listeners: -->
<!--        - name: tcp-listener -->
<!--          port: 3478 -->
<!--          protocol: TCP -->
<!--    EOF -->
<!--    ``` -->

<!--    NOTE: adding/removing gateway listeners currently induces an automatic STUN/TURN server restart -->
<!--    in the STUNner data-plane, which will disconnect all active users. As a best-practice, try to -->
<!--    avoid modifying listeners in a production deployment; you can always fire up a new STUNner -->
<!--    deployment in another Kubernetes namespace with the new configuration, direct new users there, -->
<!--    and remove the old deployment once all active clients have disconnected. -->

<!-- 1. Finally, modify the [UDP -->
<!--    route](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.UDPROute) -->
<!--    we created above that routes client connections to the UDP echo service to attach to the new -->
<!--    gateway as well. This requires adding the new TCP Gateway as a new "parent" to the route. This -->
<!--    is a general logic in the STUNner gateway operator: gateways accept all routes from their own -->
<!--    namespace and routes can choose, by enlisting a set of "parentRefs", which gateways they wish to -->
<!--    attach to. In general, STUNner will allow any client to connect via a gateway listener to any -->
<!--    backend service for which there is a route attaching to the gateway; in the below both -->
<!--    `gateway-udp` and `gateway-tcp` can connect to the `udp-echo` service, but /only/ to this -->
<!--    service and nothing else, via STUNner. -->

<!--    ```console -->
<!--    kubectl apply -f - <<EOF -->
<!--    apiVersion: gateway.networking.k8s.io/v1alpha2 -->
<!--    kind: UDPRoute -->
<!--    metadata: -->
<!--      name: udp-echo -->
<!--      namespace: stunner -->
<!--    spec: -->
<!--      parentRefs: -->
<!--        - name: udp-gateway -->
<!--        - name: tcp-gateway -->
<!--      rules: -->
<!--        - backendRefs: -->
<!--            - name: udp-echo -->
<!--    EOF -->
<!--    ``` -->

<!-- ## Connect to the TCP Gateway -->

<!-- Once we added the TCP Gateway and modified the `udp-echo` Route to attach to both the UDP and the -->
<!-- TCP Gateway, STUNner is ready to accept client connections over TCP as well. Let's check this! -->

<!-- 1. Memoize the IP addresses and ports to be used to reach the TCP Gateway: -->

<!--    ```console -->
<!--    export STUNNER_PUBLIC_ADDR=$(kubectl get svc -n stunner stunner-gateway-tcp-gateway-svc \ -->
<!--        -o jsonpath='{.status.loadBalancer.ingress[0].ip}') -->
<!--    export STUNNER_PUBLIC_PORT=$(kubectl get svc -n stunner stunner-gateway-tcp-gateway-svc \ -->
<!--        -o jsonpath='{.spec.ports[0].port}') -->
<!--    export UDP_ECHO_IP=$(kubectl get svc -n stunner udp-echo -o jsonpath='{.spec.clusterIP}') -->
<!--    ``` -->

<!-- 1. Fire up the same local [`turncat`](https://github.com/l7mp/stunner/blob/main/cmd/turncat) client -->
<!--    as before, but now set the TURN protocol to TCP: -->

<!--    ```console -->
<!--    cd stunner -->
<!--    go run cmd/turncat/main.go --log=all:DEBUG udp://127.0.0.1:9000 \ -->
<!--        turn://user-1:pass-1@${STUNNER_PUBLIC_ADDR}:${STUNNER_PUBLIC_PORT}?transport=tcp \ -->
<!--        udp://${UDP_ECHO_IP}:9001 -->
<!--    ``` -->

<!-- 1. And finally open again a local `socat` client and send anything to the UDP echo server. Note -->
<!--    that this time `turncat` will send the request over the TCP Gateway to STUNner, but it can still -->
<!--    reach the UDP echo service! -->

<!--    ```console -->
<!--    echo "Hello STUNner" | socat - udp:localhost:9000 -->
<!--    Greetings from STUNner! -->
<!--    ``` -->

## Caveats

* The operator omits the Port in UDPRoutes and the PortNumber in BackendObjectReferences and
  ParentReferences. This is because our target services typically span WebRTC media server pools
  and these may spawn a UDP/SRTP listener for essentially any arbitrary port. Eventually we would
  need to implement a CustomUDPRoute CRD that would allow the user to specify a port range (just
  like NetworkPolicies), until then the operator silently ignores ports on routes, services and
  endpoints.
* The operator actively reconciles the changes in the GatewayClass resource; e.g., if the
  ParametersRef changes then we take this into account (this is not recommended in the spec to
  [limit the blast radius of a mistaken config update](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClassSpec)
* Gateway.Listener[*].AllowedRoutes is ignored: all routes from the Gateway's namespace are allowed
  to attach to the Gateway.
* ReferenceGrants are not implemented: routes can refer to resources in any namespace.
* There is no infratructure to handle the case when a GatewayConfig that is being referred to from
  a GatewayClass, and is being actively rendered by the operator, is deleted. The controller loses
  the info on the render target and can never invalidate the corresponding STUNner configuration;
  in the long-term, we'll most probably put a finalizer to the GatewayConfigs we use but for now
  this case is not handled.
* The operator does not invalidate the GatewayClass status on exit.

## Help

STUNner development is coordinated in Discord, feel free to [join](https://discord.gg/DyPgEsbwzc).

## License

Copyright 2021-2022 by its authors. Some rights reserved. See
[AUTHORS](https://github.com/l7mp/stunner/blob/main/AUTHORS).

APACHE License - see [LICENSE](/LICENSE) for full text.

## Acknowledgments

Inspired from the [NGINX Kubernetes Gateway](https://github.com/nginxinc/nginx-kubernetes-gateway)
and the [Kong Gateway Operator](https://github.com/Kong/gateway-operator).

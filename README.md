# STUNner Kubernetes Gateway

*UNDER CONSTRUCTION*

STUNner Kubernetes Gateway is an open-source implementation of the [Gateway
API](https://gateway-api.sigs.k8s.io) using [STUNner](https://github.com/l7mp/stunner) as the data
plane. The goal of this project is to implement the part of the core Gateway APIs -- `Gateway`,
`GatewayClass`, and `UDPRoute` -- to configure a WebRTC ingress gateway Kubernetes. The STUNner
Kubernetes Gateway is currently under development and supports a subset of the Gateway API.

> Warning: This project is actively in development (pre-alpha feature state) and should not be
> deployed in a production environment.  All APIs, SDKs, designs, and packages are subject to
> change.

# Run the STUNner Kubernetes Gateway

## Prerequisites

Before you can build and run the STUNner Kubernetes Gateway, make sure you have the following software installed on your machine:
- [git](https://git-scm.com/)
- [GNU Make](https://www.gnu.org/software/software.html)
- [Docker](https://www.docker.com/) v18.09+
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

## Build the image

1. Clone the repo and change into the `stunner-gateway-operator` directory:

``` console
   git clone https://github.com/l7mp/stunner-gateway-operator.git
   cd stunner-gateway-operator
```

1. Build the image with [podman](https://podman.io) (required `sudo`):
 
``` console
IMG=<my-image> make podman-build
```

1. Push the image to your container registry:

``` console
IMG=<my-image> make podman-push
```

## Deploy the operator

You can deploy STUNner Kubernetes Gateway on an existing Kubernetes 1.22+ cluster. The following instructions walk through the steps for deploying on a [kind](https://kind.sigs.k8s.io/) cluster.

1. Install the Gateway CRDs:

``` console
kubectl apply -k "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.3"
```

1. Deploy the STUNner Kubernetes Gateway:

``` console
make deploy
```

1. Confirm the STUNner Kubernetes Gateway is running in `stunner-gateway` namespace:

``` console
kubectl get pods -n stunner-gateway-operator-system
NAME                                                          READY   STATUS    RESTARTS   AGE
stunner-gateway-operator-controller-manager-65dbf8fb4-hjrjr   2/2     Running   0          42m
```

<!-- ## Expose the STUNner Kubernetes Gateway -->

<!-- You can gain access to STUNner Kubernetes Gateway by creating a `NodePort` Service or a `LoadBalancer` Service. -->

<!-- ### Create a NodePort Service -->

<!-- Create a service with type `NodePort`: -->

<!-- ``` -->
<!-- kubectl apply -f deploy/manifests/service/nodeport.yaml -->
<!-- ``` -->

<!-- A `NodePort` service will randomly allocate one port on every node of the cluster. To access the STUNner Kubernetes Gateway, use an IP address of any node in the cluster along with the allocated port. -->

<!-- ### Create a LoadBalancer Service -->

<!-- Create a service with type `LoadBalancer` using the appropriate manifest for your cloud provider.  -->

<!-- ``` -->
<!-- kubectl apply -f deploy/manifests/service/loadbalancer.yaml -->
<!-- ``` -->

<!-- Lookup the public IP of the load balancer: -->
   
<!-- ``` -->
<!-- kubectl get svc stunner-gateway -n stunner-gateway -->
<!-- ```  -->

## Configure the operator

The STUNner operator (partially) implements the official Kubernetes [Gateway
API](https://gateway-api.sigs.k8s.io), which allows you to interact with STUNner using the
convenience of `kubectl` and declarative YAML configurations. 

Below we configure a minimal STUNner gateway setup that exposes the STUN/TURN service of the
STUNner gateway over UDP:3478 and TCP:3478.

1. Create a namespace called `stunner` that will host all Kubernetes configuration of STUNner.

``` console
kubectl create namespace stunner
```

1. Deploy the STUNner gateway: this will serve as the data-plane to ingest your WebRTC traffic into
   the Kubernetes cluster:

``` console
helm repo add stunner https://l7mp.io/stunner
helm repo update
helm install stunner stunner/stunner --set stunner.namespace=stunner
```

1. Create a
   [GatewayClass](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewayClass). This
   will serve as the root level configuration for your STUNner deployment:
   
``` console
cd stunner-gateway-operator
kubectl apply -f config/samples/gateway_v1alpha2_gatewayclass.yaml 
```

1. Now, we have to specify some important configuration for STUNner, by loading a `GatewayConfig`
   custom resource into Kubernetes. Make sure to use the `stunner` namespace we have just created;
   this will be the target namespace where the operator will render the running STUNner data-plane
   configuration. 
   
   Make sure to customize the authentication mode and credentials used for STUNner; consult the
   [STUNner authentication guide](https://github.com/l7mp/stunner/blob/main/doc/AUTH.md) to
   understand how to set the `authType` parameter and the credentials below:

```console
kubectl apply -f - <<EOF
apiVersion: stunner.l7mp.io/v1alpha1
kind: GatewayConfig
metadata:
  name: gatewayconfig-sample
  namespace: stunner
spec:
  stunnerConfig: "stunnerd-configmap"
  authType: plaintext
  userName: "user-1"
  password: "pass-1"
EOF
```

1. Create your first STUNner
   [Gateway](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.Gateway)!
   This will define the listener protocol and port on which STUN/TURN traffic from clients will
   enter the Kubernetes cluster via STUNner.

```console
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: Gateway
metadata:
  name: gateway-sample
  namespace: stunner
spec:
  gatewayClassName: gatewayclass-sample
  listeners:
    - name: sample-listener-udp
      port: 3478
      protocol: UDP
EOF
```

1. Then, create a target service that you will expose through STUNner to your clients.  For
   instance, if you could let your clients to connect to your WebRTC media servers via STUNner (a
   fairly probable setup for STUNner). For simplicity, below we use the simple UDP echo server from
   the [STUNner UDP tunnel demo](https://github.com/l7mp/stunner/blob/main/examples/simple-tunnel):

``` console
kubectl create deployment -n stunner udp-echo --image=l7mp/net-debug:latest
kubectl expose deployment -n stunner  udp-echo --name=udp-echo --type=ClusterIP --protocol=UDP --port=9001
kubectl exec -it -n stunner $(kubectl get pod -l app=udp-echo -n stunner -o jsonpath="{.items[0].metadata.name}") -- \
     socat -d -d udp-l:9001,fork EXEC:"echo Greetings from STUNner!"
```

1. Finally, add an [UDP
   route](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.UDPROute)
   to actually allow clients to connect to the UDP echo service via the STUNner gateway:

```console
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: UDPRoute
metadata:
  name: udproute-sample
  namespace: stunner
spec:
  parentRefs:
    - name: gateway-sample
      sectionName: sample-listener-udp
  rules:
    - backendRefs:
        - name: udp-echo
          namespace: stunner
EOF
```

1. Check the result: the operator should have rendered a valid and up to date STUNner configuration
   in the ConfigMap you specified in the above GatewayConfig (called `stunnerd-configmap` in our
   example), in the same namespace where the root GatewayConfig lives. 
   
```console
kubectl get cm -n stunner stunnerd-configmap -o yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: stunnerd-configmap
  namespace: stunner
data:
  stunnerd.conf: '{"version":"v1alpha1","admin":{"name":"stunner-daemon","loglevel":"all:INFO"},"auth":{"type":"plaintext","realm":"stunner.l7mp.io","credentials":{"password":"pass-1","username":"user-1"}},"listeners":[{"name":"sample-listener-udp","protocol":"UDP","public_address":"34.116.153.23","public_port":31273,"address":"$STUNNER_ADDR","port":3478,"min_relay_port":32768,"max_relay_port":65535,"routes":["udproute-sample"]},{"name":"sample-listener-tcp","protocol":"TCP","public_address":"34.116.153.23","public_port":31273,"address":"$STUNNER_ADDR","port":3478,"min_relay_port":32768,"max_relay_port":65535,"routes":["udproute-sample"]}],"clusters":[{"name":"udproute-sample","type":"STRICT_DNS","endpoints":["udp-echo.stunner.svc.cluster.local"]}]}'
retvari@weber:/export/l7mp/stunner-gateway-operator$ kubectl get cm -n stunner stunnerd-configmap -o yaml
```

1. Map the configuration rendered by the operator in the above ConfigMap into the STUNner pods, so
   that the STUNner daemons can pick this configuration up. The `-w` command line argument below
   switches the STUNner daemon into watch mode: the daemon will get notified by Kubernetes whenever
   the operator renders a new configuration (e.g., when a Gateway or a UDPRoute changes) so that it
   can reconcile the most up-to-date configuration.
   
```console
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: stunner
  namespace: stunner
spec:
  selector:
    matchLabels:
      app: stunner
  template:
    metadata:
      labels:
        app: stunner
    spec:
      containers:
        - command: ["stunnerd"]
          args: ["-v", "-w", "-c", "/etc/stunnerd/stunnerd.conf"]
          image: l7mp/stunnerd:latest
          imagePullPolicy: Always
          name: stunnerd
          env:
            - name: STUNNER_ADDR
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: status.podIP
          volumeMounts:
            - name: stunnerd-config-volume
              mountPath: /etc/stunnerd
      volumes:
        - name: stunnerd-config-volume
          configMap:
            name: stunnerd-configmap
EOF
```

1. The STUNner gateway operator automatically exposes _all_ Gateway listeners in standard
   Kubernetes services. The corresponding public IP and port for each listener can be learned from
   the External IP field for the service; for instance, in the below example Kubernetes assigned
   the IP-pot pair 34.118.16.31:3478 for the UDP listener
   
```console
kubectl get svc -n stunner 
NAME                                 TYPE           CLUSTER-IP     EXTERNAL-IP    PORT(S)          AGE
stunner-gateway-gateway-sample-svc   LoadBalancer   10.120.5.177   34.118.16.31   3478:31273/UDP   58m
udp-echo                             ClusterIP      10.120.0.28    <none>         9001/UDP         32m
```

1. Memorize the IP addresses and ports to be used to reach the UDP echo server behind STUNner:

```console
export STUNNER_PUBLIC_ADDR=$(kubectl get svc -n stunner stunner-gateway-gateway-sample-svc \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export STUNNER_PUBLIC_PORT=$(kubectl get svc -n stunner stunner-gateway-gateway-sample-svc \
    -o jsonpath='{.spec.ports[0].port}')
export UDP_ECHO_IP=$(kubectl get svc -n stunner udp-echo -o jsonpath='{.spec.clusterIP}')
```

1. Fire up a local [`turncat`](https://github.com/l7mp/stunner/blob/main/cmd/turncat) client to
   tunnel the UDP port `localhost:9000` to the UDP service:

```console
cd stunner
go run cmd/turncat/main.go --log=all:DEBUG udp://127.0.0.1:9000 \
    turn://user-1:pass-1@${STUNNER_PUBLIC_ADDR}:${STUNNER_PUBLIC_PORT} \
    udp://${UDP_ECHO_IP}:9001
```

1. And finally open a local `socat` and send anything to the UDP echo server: you should see it
   echoing the same input back:
   
```console
echo "Hello STUNner" | socat - udp:localhost:9000
```
   
## Help

STUNner development is coordinated in Discord, send
[us](https://github.com/l7mp/stunner/blob/main/AUTHORS) an email to ask an invitation.

## License

Copyright 2021-2022 by its authors. Some rights reserved. See
[AUTHORS](https://github.com/l7mp/stunner/blob/main/AUTHORS).

APACHE License - see [LICENSE](/LICENSE) for full text.

## Acknowledgments

Inspired from the [NGINX Kubernetes Gateway](https://github.com/nginxinc/nginx-kubernetes-gateway)
and the [Kong Gateway Operator](https://github.com/Kong/gateway-operator).

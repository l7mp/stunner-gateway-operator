# STUNner Kubernetes Gateway

*UNDER CONSTRUCTION*

STUNNER Kubernetes Gateway is an open-source implementation of the [Gateway
API](https://gateway-api.sigs.k8s.io/) using [STUNner](https://github.com/l7mp/stunner) as the data
plane. The goal of this project is to implement the part of the core Gateway APIs -- `Gateway`,
`GatewayClass`, `TCPRoute`, and `UDPRoute` -- to configure a WebRTC ingress gateway Kubernetes. The
STUNner Kubernetes Gateway is currently under development and supports a subset of the Gateway API.

> Warning: This project is actively in development (pre-alpha feature state) and should not be deployed in a production environment.
> All APIs, SDKs, designs, and packages are subject to change.

<!-- # Run the STUNner Kubernetes Gateway -->

<!-- ## Prerequisites -->

<!-- Before you can build and run the STUNner Kubernetes Gateway, make sure you have the following software installed on your machine: -->
<!-- - [git](https://git-scm.com/) -->
<!-- - [GNU Make](https://www.gnu.org/software/software.html) -->
<!-- - [Docker](https://www.docker.com/) v18.09+ -->
<!-- - [kubectl](https://kubernetes.io/docs/tasks/tools/) -->

<!-- ## Build the image -->

<!-- 1. Clone the repo and change into the `stunner-gateway-operator` directory: -->

<!--    ``` -->
<!--    git clone https://github.com/l7mp/stunner-gateway-operator.git -->
<!--    cd stunner-gateway-operator -->
<!--    ``` -->

<!-- 1. Build the image: -->
 
<!--    ``` -->
<!--    make container -->
<!--    ``` -->

<!-- 1. Push the image to your container registry: -->

<!--    ``` -->
<!--    docker push stunner-gateway-operator -->
<!--    ``` -->

<!-- ## Deploy the STUNner Kubernetes Gateway -->

<!-- You can deploy STUNNER Kubernetes Gateway on an existing Kubernetes 1.22+ cluster. The following instructions walk through the steps for deploying on a [kind](https://kind.sigs.k8s.io/) cluster.  -->

<!-- 1. Install the Gateway CRDs: -->

<!--    ``` -->
<!--    kubectl apply -k "github.com/kubernetes-sigs/gateway-api/config/crd?ref=v0.4.2"  -->
<!--    ``` -->

<!-- 1. Deploy the STUNner Kubernetes Gateway: -->

<!--    Before deploying, make sure to update the Deployment spec in `stunner-gateway.yaml` to reference the image you built. -->

<!--    ``` -->
<!--    kubectl apply -f deploy/manifests/stunner-gateway.yaml -->
<!--    ```  -->

<!-- 1. Confirm the STUNNER Kubernetes Gateway is running in `stunner-gateway` namespace: -->

<!--    ``` -->
<!--    kubectl get pods -n stunner-gateway -->
<!--    NAME                               READY   STATUS    RESTARTS   AGE -->
<!--    stunner-gateway-5d4f4c7db7-xk2kq   2/2     Running   0          112s -->
<!--    ``` -->

<!-- ## Expose the STUNner Kubernetes Gateway -->

<!-- You can gain access to STUNNER Kubernetes Gateway by creating a `NodePort` Service or a `LoadBalancer` Service. -->

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
   
<!-- Use the public IP of the load balancer to access the STUNner Kubernetes Gateway. -->
   
## Help

STUNner development is coordinated in Discord, send
[us](https://github.com/l7mp/stunner/blob/main/AUTHORS) an email to ask an invitation.

## License

Copyright 2021-2022 by its authors. Some rights reserved. See
[AUTHORS](https://github.com/l7mp/stunner/blob/main/AUTHORS).

APACHE License - see [LICENSE](/LICENSE) for full text.

## Acknowledgments

Initially forked from the [NGINX Kubernetes Gateway](https://github.com/nginxinc/nginx-kubernetes-gateway).

package controllers

// RBAC for directly watched resources.
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses;gateways;udproutes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses/status;gateways/status;udproutes/status,verbs=update;patch
// +kubebuilder:rbac:groups="stunner.l7mp.io",resources=gatewayconfigs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="stunner.l7mp.io",resources=gatewayconfigs/finalizers,verbs=update

// RBAC for references in watched resources.
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes;secrets;endpoints;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status;services/status;endpoints/status,verbs=get

// RBAC for the rendering target
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update

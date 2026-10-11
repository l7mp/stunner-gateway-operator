package controllers

// RBAC for directly watched resources.

// core
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=nodes;secrets;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes/status;services/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update

// apps
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status;deployments/finalizers,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets/status;daemonsets/finalizers,verbs=get;list;watch

// discovery.k8s.io
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices/status,verbs=get;list;watch

// gateway.networking.k8s.io
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses;gateways;udproutes;tcproutes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses/status;gateways/status;udproutes/status;tcproutes/status,verbs=update;patch

// stunner.l7mp.io
// +kubebuilder:rbac:groups="stunner.l7mp.io",resources=gatewayconfigs;staticservices;dataplanes;udproutes;tcproutes;turnroutes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="stunner.l7mp.io",resources=staticservices/finalizers;udproutes/finalizers;udproutes/status;tcproutes/finalizers;tcproutes/status;turnroutes/finalizers;turnroutes/status,verbs=update;patch

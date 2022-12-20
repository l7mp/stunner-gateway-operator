package controllers

// RBAC for directly watched resources.
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses;gateways;udproutes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="gateway.networking.k8s.io",resources=gatewayclasses/status;gateways/status;udproutes/status,verbs=update;patch
// +kubebuilder:rbac:groups="stunner.l7mp.io",resources=gatewayconfigs,verbs=get;list;watch

// RBAC for references in watched resources.
// +kubebuilder:rbac:groups="",resources=secrets;services;endpoints;verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps;verbs=get;list;watch;create;update;patch;delete

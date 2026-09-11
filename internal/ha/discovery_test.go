package ha

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func fixture(t *testing.T) *Discovery {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, discoveryv1.AddToScheme(scheme))
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "discovery", Namespace: "system", UID: "service-uid"},
		Spec: corev1.ServiceSpec{IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol}}}
	return &Discovery{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build(),
		Service: types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, Address: "10.0.0.1", Port: 13478,
		Pod: corev1.ObjectReference{APIVersion: "v1", Kind: "Pod", Namespace: "system", Name: "first", UID: "first-uid"}, Log: logr.Discard()}
}

func TestLeaderDiscoveryWaitsForElectionAndSnapshot(t *testing.T) {
	d := fixture(t)
	elected, initialized := make(chan struct{}), make(chan struct{})
	bootstrap, serving := make(chan struct{}), make(chan context.Context, 1)
	d.Elected, d.Initialized = elected, initialized
	d.Initialize = func(context.Context) error { close(bootstrap); return nil }
	d.Serve = func(ctx context.Context) error { serving <- ctx; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Start(ctx) }()
	require.Eventually(t, func() bool { return d.Ready(nil) == nil }, time.Second, time.Millisecond)
	require.False(t, d.NeedLeaderElection(), "standby must participate in readiness without being leader")
	select {
	case <-bootstrap:
		t.Fatal("standby bootstrapped before election")
	default:
	}
	close(elected)
	select {
	case <-bootstrap:
	case <-time.After(time.Second):
		t.Fatal("leader did not initialize")
	}
	require.Error(t, d.Ready(nil), "leader is not ready before its snapshot and endpoint")
	select {
	case <-serving:
		t.Fatal("served before complete snapshot")
	default:
	}
	list := &discoveryv1.EndpointSliceList{}
	require.NoError(t, d.Client.List(ctx, list))
	require.Empty(t, list.Items)
	close(initialized)
	var servingCtx context.Context
	select {
	case servingCtx = <-serving:
	case <-time.After(time.Second):
		t.Fatal("initialized leader did not serve")
	}
	require.Eventually(t, func() bool { return d.Client.List(ctx, list) == nil && len(list.Items) == 1 }, time.Second, time.Millisecond)
	require.Equal(t, []string{"10.0.0.1"}, list.Items[0].Endpoints[0].Addresses)
	require.Eventually(t, func() bool { return d.Ready(nil) == nil }, time.Second, time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	require.Error(t, servingCtx.Err(), "lease/manager cancellation must fence existing connections locally")
	require.Error(t, d.Ready(nil))
}

func TestLeaderDiscoveryCanceledStandbyNeverServes(t *testing.T) {
	for _, phase := range []string{"election", "configuration"} {
		t.Run(phase, func(t *testing.T) {
			d := fixture(t)
			elected := make(chan struct{})
			d.Elected = elected
			d.Initialized = make(chan struct{})
			if phase == "configuration" {
				close(elected)
			}
			d.Initialize = func(context.Context) error { return nil }
			d.Serve = func(context.Context) error { t.Error("uninitialized standby served"); return nil }
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			require.NoError(t, d.Start(ctx))
			list := &discoveryv1.EndpointSliceList{}
			require.NoError(t, d.Client.List(context.Background(), list))
			require.Empty(t, list.Items)
		})
	}
}

func TestLeaderDiscoveryReplacesEndpointAndDoesNotWithdrawSuccessor(t *testing.T) {
	d := fixture(t)
	ctx := context.Background()
	require.NoError(t, d.Publish(ctx))
	d.Address = "10.0.0.2"
	d.Pod.Name = "second"
	d.Pod.UID = "second-uid"
	require.NoError(t, d.Publish(ctx))
	slice := &discoveryv1.EndpointSlice{}
	require.NoError(t, d.Client.Get(ctx, types.NamespacedName{Namespace: "system", Name: "discovery-leader"}, slice))
	require.Len(t, slice.Endpoints, 1)
	require.Equal(t, []string{"10.0.0.2"}, slice.Endpoints[0].Addresses)
	require.Equal(t, types.UID("second-uid"), slice.Endpoints[0].TargetRef.UID)
	require.True(t, *slice.Endpoints[0].Conditions.Ready)
	require.Equal(t, int32(13478), *slice.Ports[0].Port)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	d.Address = "10.0.0.1"
	require.ErrorIs(t, d.Publish(canceled), context.Canceled)
	require.NoError(t, d.Client.Get(ctx, types.NamespacedName{Namespace: "system", Name: "discovery-leader"}, slice))
	require.Equal(t, []string{"10.0.0.2"}, slice.Endpoints[0].Addresses)
}

func TestLeaderDiscoveryRejectsUnsafeServiceAndForeignSlice(t *testing.T) {
	for _, tc := range []string{"selector", "dual-stack", "wrong-family", "invalid-ip", "foreign-slice"} {
		t.Run(tc, func(t *testing.T) {
			d := fixture(t)
			ctx := context.Background()
			svc := &corev1.Service{}
			require.NoError(t, d.Client.Get(ctx, d.Service, svc))
			switch tc {
			case "selector":
				svc.Spec.Selector = map[string]string{"app": "operator"}
			case "dual-stack":
				svc.Spec.IPFamilies = append(svc.Spec.IPFamilies, corev1.IPv6Protocol)
			case "wrong-family":
				d.Address = "2001:db8::1"
			case "invalid-ip":
				d.Address = "invalid"
			case "foreign-slice":
				require.NoError(t, d.Client.Create(ctx, &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "discovery-leader", Namespace: "system"}}))
			}
			require.NoError(t, d.Client.Update(ctx, svc))
			require.Error(t, d.Publish(ctx))
		})
	}
}

func TestLeaderDiscoveryLabelIsValid(t *testing.T) {
	require.Empty(t, validation.IsValidLabelValue(ManagedBy))
}

func TestLeaderDiscoveryIPv6(t *testing.T) {
	d := fixture(t)
	ctx := context.Background()
	svc := &corev1.Service{}
	require.NoError(t, d.Client.Get(ctx, d.Service, svc))
	svc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv6Protocol}
	require.NoError(t, d.Client.Update(ctx, svc))
	d.Address = "2001:db8::1"
	require.NoError(t, d.Publish(ctx))
	list := &discoveryv1.EndpointSliceList{}
	require.NoError(t, d.Client.List(ctx, list))
	require.Equal(t, discoveryv1.AddressTypeIPv6, list.Items[0].AddressType)
}

// Package ha publishes only the initialized leader as a discovery endpoint.
package ha

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ManagedBy = "leader-discovery.stunner.l7mp.io"

type Discovery struct {
	Signal      context.Context // optional process shutdown context
	Client      client.Client   // uncached: resource versions must be current at publication
	Service     types.NamespacedName
	Pod         corev1.ObjectReference
	Address     string
	Port        int32
	Elected     <-chan struct{}
	Initialized <-chan struct{}
	Initialize  func(context.Context) error
	Serve       func(context.Context) error
	Log         logr.Logger
	warm        atomic.Bool
	leader      atomic.Bool
	published   atomic.Bool
}

// Standbys are ready once the manager's caches are synchronized. Pod readiness
// must not depend on leadership: Deployments and PDBs count both healthy replicas.
func (d *Discovery) NeedLeaderElection() bool { return false }

func (d *Discovery) Ready(_ *http.Request) error {
	if !d.warm.Load() {
		return fmt.Errorf("waiting for manager cache synchronization")
	}
	if d.leader.Load() && !d.published.Load() {
		return fmt.Errorf("waiting for initialized leader discovery endpoint")
	}
	return nil
}

func (d *Discovery) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if d.Signal != nil {
		stop := context.AfterFunc(d.Signal, cancel)
		defer stop()
	}
	// controller-runtime starts non-leader runnables after synchronizing caches.
	d.warm.Store(true)
	defer d.warm.Store(false)
	select {
	case <-d.Elected:
	case <-ctx.Done():
		return nil
	}
	d.leader.Store(true)
	for {
		if err := d.Initialize(ctx); err == nil {
			break
		} else {
			d.Log.Error(err, "Cannot initialize leader configuration")
		}
		if !pause(ctx) {
			return nil
		}
	}
	select {
	case <-d.Initialized:
	case <-ctx.Done():
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	if err := d.Serve(ctx); err != nil {
		return err
	}
	// On cancellation the listener and existing WebSockets close locally. Never
	// clear the EndpointSlice on exit: the successor may already have replaced it.
	for {
		if err := d.Publish(ctx); err != nil {
			d.published.Store(false)
			d.Log.Error(err, "Cannot publish leader discovery endpoint")
		} else {
			d.published.Store(true)
		}
		if !pause(ctx) {
			return nil
		}
	}
}

func pause(ctx context.Context) bool {
	t := time.NewTimer(2 * time.Second)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Publish updates one EndpointSlice with optimistic concurrency. A stale request
// cannot overwrite a successor's update using an older resource version.
func (d *Discovery) Publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	svc := &corev1.Service{}
	if err := d.Client.Get(ctx, d.Service, svc); err != nil {
		return err
	}
	if len(svc.Spec.Selector) != 0 || len(svc.Spec.IPFamilies) != 1 {
		return fmt.Errorf("leader discovery requires a selectorless, single-stack Service")
	}
	ip := net.ParseIP(d.Address)
	if ip == nil {
		return fmt.Errorf("invalid Pod IP %q", d.Address)
	}
	family := corev1.IPv6Protocol
	addressType := discoveryv1.AddressTypeIPv6
	if ip.To4() != nil {
		family = corev1.IPv4Protocol
		addressType = discoveryv1.AddressTypeIPv4
	}
	if svc.Spec.IPFamilies[0] != family {
		return fmt.Errorf("Pod IP and discovery Service have different address families")
	}
	key := types.NamespacedName{Namespace: d.Service.Namespace, Name: d.Service.Name + "-leader"}
	slice := &discoveryv1.EndpointSlice{}
	err := d.Client.Get(ctx, key, slice)
	create := apierrors.IsNotFound(err)
	if err != nil && !create {
		return err
	}
	if create {
		slice.ObjectMeta = metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}
	} else if slice.Labels[discoveryv1.LabelManagedBy] != ManagedBy || !metav1.IsControlledBy(slice, svc) {
		return fmt.Errorf("refusing to overwrite an EndpointSlice not owned by this discovery Service")
	}
	previous := slice.DeepCopy()
	slice.Labels = map[string]string{discoveryv1.LabelServiceName: svc.Name, discoveryv1.LabelManagedBy: ManagedBy}
	slice.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(svc, corev1.SchemeGroupVersion.WithKind("Service"))}
	slice.AddressType = addressType
	slice.Ports = []discoveryv1.EndpointPort{{Name: ptr.To("cds"), Port: ptr.To(d.Port), Protocol: ptr.To(corev1.ProtocolTCP)}}
	slice.Endpoints = []discoveryv1.Endpoint{{Addresses: []string{d.Address}, TargetRef: d.Pod.DeepCopy(),
		Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true), Serving: ptr.To(true), Terminating: ptr.To(false)}}}
	if err := ctx.Err(); err != nil {
		return err
	}
	if create {
		return d.Client.Create(ctx, slice)
	}
	if apiequality.Semantic.DeepEqual(previous, slice) {
		return nil
	}
	return d.Client.Update(ctx, slice)
}

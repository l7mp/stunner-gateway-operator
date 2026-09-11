package operator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

type snapshotManager struct {
	manager.Manager
	client client.Client
}

func (m snapshotManager) GetClient() client.Client { return m.client }

type snapshotController struct {
	noopController
	name  string
	calls *[]string
	err   error
}

func (c *snapshotController) Reconcile(_ context.Context, r reconcile.Request) (reconcile.Result, error) {
	*c.calls = append(*c.calls, c.name+r.Name)
	return reconcile.Result{}, c.err
}

func TestInitialSnapshotRequiresEveryController(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "failed-route-list"}[failed], func(t *testing.T) {
			ch := make(chan event.Event, 10)
			o := newTestOperator(ch, nil, nil, nil)
			o.waitingForSnapshot.Store(true)
			s := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(s))
			o.mgr = snapshotManager{client: fake.NewClientBuilder().WithScheme(s).WithObjects(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}}).Build()}
			calls := []string{}
			o.gwConfC = &snapshotController{name: "config", calls: &calls}
			o.dpC = &snapshotController{name: "dataplane", calls: &calls}
			o.gwC = &snapshotController{name: "gateway", calls: &calls}
			r := &snapshotController{name: "route", calls: &calls}
			o.rouC = r
			o.nodeC = &snapshotController{name: "node:", calls: &calls}
			if failed {
				r.err = errors.New("route List failed")
			}
			err := o.Initialize(context.Background())
			if failed {
				require.EqualError(t, err, "route List failed")
				require.True(t, o.waitingForSnapshot.Load())
				require.Empty(t, ch)
				require.Equal(t, []string{"config", "dataplane", "gateway", "route"}, calls)
			} else {
				require.NoError(t, err)
				require.False(t, o.waitingForSnapshot.Load())
				require.Equal(t, []string{"config", "dataplane", "gateway", "route", "node:node"}, calls)
				require.Equal(t, event.EventTypeReconcile, (<-ch).GetType())
			}
		})
	}
}

func TestInitialSnapshotSuppressesPartialRendering(t *testing.T) {
	ch, updates, configs, renders := make(chan event.Event, 10), make(chan event.Event, 10), make(chan event.Event, 10), make(chan event.Event, 10)
	o := newTestOperator(ch, updates, configs, renders)
	o.waitingForSnapshot.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o.operatorCh.Get()
	go o.eventLoop(ctx, nil)
	ch <- event.NewEventReconcile()
	ch <- event.NewEventUpdate(1)
	ch <- event.NewEventAck(100)
	require.Eventually(t, func() bool { return o.GetLastAckedGeneration() == 100 }, time.Second, time.Millisecond)
	require.Empty(t, renders)
	require.Empty(t, updates)
	require.Empty(t, configs)
	o.waitingForSnapshot.Store(false)
	ch <- event.NewEventUpdate(2)
	select {
	case e := <-configs:
		require.Equal(t, 2, e.(*event.EventUpdate).Generation)
	case <-time.After(time.Second):
		t.Fatal("complete snapshot not forwarded")
	}
}

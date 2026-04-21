package updater

import (
	"fmt"
	"reflect"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/lens"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (u *Updater) upsertResourceObject(desired client.Object, gen int) (ctrlutil.OperationResult, error) {
	l, err := lens.New(desired)
	if err != nil {
		return ctrlutil.OperationResultNone, err
	}

	kind := objectKind(desired)
	prefix := "spec." + kind
	resource := store.GetObjectKey(desired)
	u.incCounter(prefix + ".attempt")

	current, err := emptyObjectFor(desired)
	if err != nil {
		u.incCounter(prefix + ".error")
		return ctrlutil.OperationResultNone, err
	}

	cli := u.manager.GetClient()
	if err := cli.Get(u.ctx, client.ObjectKeyFromObject(desired), current); err == nil {
		if l.EqualResource(current) {
			u.incCounter(prefix + ".suppressed")
			u.log.V(2).Info(fmt.Sprintf("%s unchanged, skipping upsert", kind),
				"resource", resource, "generation", gen)
			return ctrlutil.OperationResultNone, nil
		}
	} else if !apierrors.IsNotFound(err) {
		u.incCounter(prefix + ".error")
		return ctrlutil.OperationResultNone, fmt.Errorf("cannot get %s %q: %w", kind, resource, err)
	}

	op, err := ctrlutil.CreateOrPatch(u.ctx, cli, current, func() error {
		return l.ApplyToResource(current)
	})
	if err != nil {
		u.incCounter(prefix + ".error")
		return ctrlutil.OperationResultNone, fmt.Errorf("cannot upsert %s %q: %w", kind, resource, err)
	}

	switch op {
	case ctrlutil.OperationResultCreated:
		u.incCounter(prefix + ".created")
	case ctrlutil.OperationResultUpdated:
		u.incCounter(prefix + ".updated")
	case ctrlutil.OperationResultUpdatedStatus:
		u.incCounter(prefix + ".updatedStatus")
	case ctrlutil.OperationResultUpdatedStatusOnly:
		u.incCounter(prefix + ".updatedStatusOnly")
	case ctrlutil.OperationResultNone:
		u.incCounter(prefix + ".none")
	}

	u.log.V(1).Info(fmt.Sprintf("%s upserted", kind), "resource", resource,
		"generation", gen, "result", store.DumpObject(current))

	return op, nil
}

func (u *Updater) updateStatusObject(desired client.Object, gen int) error {
	l, err := lens.New(desired)
	if err != nil {
		return err
	}

	kind := objectKind(desired)
	prefix := "status." + kind
	resource := store.GetObjectKey(desired)
	u.incCounter(prefix + ".attempt")

	current, err := emptyObjectFor(desired)
	if err != nil {
		u.incCounter(prefix + ".error")
		return err
	}

	cli := u.manager.GetClient()
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		u.incCounter(prefix + ".retryPass")
		if err := cli.Get(u.ctx, client.ObjectKeyFromObject(desired), current); err != nil {
			return err
		}

		if l.EqualStatus(current) {
			u.incCounter(prefix + ".suppressed")
			u.log.V(2).Info(fmt.Sprintf("%s status unchanged, skipping update", kind),
				"resource", resource, "generation", gen)
			return nil
		}

		if err := l.ApplyToStatus(current); err != nil {
			return err
		}

		if err := cli.Status().Update(u.ctx, current); err != nil {
			return err
		}

		u.incCounter(prefix + ".updated")
		u.log.V(1).Info(fmt.Sprintf("%s status updated", kind), "resource", resource,
			"generation", gen, "result", store.DumpObject(current))
		return nil
	})

	if err != nil {
		u.incCounter(prefix + ".error")
	}

	return err
}

func (u *Updater) deleteObject(o client.Object, gen int) error {
	u.log.V(2).Info("Delete object", "resource", store.GetObjectKey(o), "generation", gen)
	return u.manager.GetClient().Delete(u.ctx, o)
}

func emptyObjectFor(o client.Object) (client.Object, error) {
	meta := metav1.ObjectMeta{Name: o.GetName(), Namespace: o.GetNamespace()}

	switch o.(type) {
	case *corev1.ConfigMap:
		return &corev1.ConfigMap{ObjectMeta: meta}, nil
	case *corev1.Service:
		return &corev1.Service{ObjectMeta: meta}, nil
	case *appv1.Deployment:
		return &appv1.Deployment{ObjectMeta: meta}, nil
	case *appv1.DaemonSet:
		return &appv1.DaemonSet{ObjectMeta: meta}, nil
	case *gwapiv1.GatewayClass:
		return &gwapiv1.GatewayClass{ObjectMeta: meta}, nil
	case *gwapiv1.Gateway:
		return &gwapiv1.Gateway{ObjectMeta: meta}, nil
	case *stnrgwv1.UDPRoute:
		return &stnrgwv1.UDPRoute{ObjectMeta: meta}, nil
	case *gwapiv1a2.UDPRoute:
		return &gwapiv1a2.UDPRoute{ObjectMeta: meta}, nil
	default:
		return nil, fmt.Errorf("unsupported object type %T", o)
	}
}

func objectKind(o client.Object) string {
	t := reflect.TypeOf(o)
	if t == nil {
		return "Unknown"
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t.Name()
}

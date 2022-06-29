package updater

// updater uploads client updates
import (
	// "context"
	"fmt"
	// "reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// corev1 "k8s.io/api/core/v1"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (u *Updater) updateGatewayClass(gc *gatewayv1alpha2.GatewayClass) error {
	u.log.V(2).Info("update gateway class", "resource", store.GetObjectKey(gc))

	cli := u.manager.GetClient()
	current := &gatewayv1alpha2.GatewayClass{ObjectMeta: metav1.ObjectMeta{
		Name:      gc.GetName(),
		Namespace: gc.GetNamespace(),
	}}

	if err := cli.Get(u.ctx, client.ObjectKeyFromObject(current), current); err != nil {
		return err
	}

	// the only thing we change on gatewayclasses is the status: copy
	gc.Status.DeepCopyInto(&current.Status)

	if err := cli.Status().Update(u.ctx, current); err != nil {
		return err
	}

	u.log.V(1).Info("gateway-class updated", "resource", store.GetObjectKey(gc), "result",
		current)

	return nil
}

func (u *Updater) updateGateway(gw *gatewayv1alpha2.Gateway) error {
	u.log.V(2).Info("updating gateway", "resource", store.GetObjectKey(gw))

	cli := u.manager.GetClient()
	current := &gatewayv1alpha2.Gateway{ObjectMeta: metav1.ObjectMeta{
		Name:      gw.GetName(),
		Namespace: gw.GetNamespace(),
	}}

	if err := cli.Get(u.ctx, client.ObjectKeyFromObject(current), current); err != nil {
		return err
	}

	gw.Status.DeepCopyInto(&current.Status)

	if err := cli.Status().Update(u.ctx, current); err != nil {
		return err
	}

	u.log.V(1).Info("gateway updated", "resource", store.GetObjectKey(gw), "result",
		current)

	return nil
}

func (u *Updater) updateUDPRoute(ro *gatewayv1alpha2.UDPRoute) error {
	u.log.V(2).Info("updating UDP-route", "resource", store.GetObjectKey(ro))

	cli := u.manager.GetClient()
	current := &gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{
		Name:      ro.GetName(),
		Namespace: ro.GetNamespace(),
	}}

	if err := cli.Get(u.ctx, client.ObjectKeyFromObject(current), current); err != nil {
		return err
	}

	ro.Status.DeepCopyInto(&current.Status)

	if err := cli.Status().Update(u.ctx, current); err != nil {
		return err
	}

	u.log.V(1).Info("UDP-route updated", "resource", store.GetObjectKey(ro), "result",
		current)

	return nil
}

func (u *Updater) upsertService(svc *corev1.Service) (ctrlutil.OperationResult, error) {
	u.log.V(2).Info("upsert service", "resource", store.GetObjectKey(svc))

	client := u.manager.GetClient()
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
	}}

	op, err := ctrlutil.CreateOrUpdate(u.ctx, client, current, func() error {
		// upsert: ownerrefs, our own annotation, and the spec
		as := svc.GetAnnotations()
		if name, found := as[config.GatewayAddressAnnotationKey]; found != false {
			metav1.SetMetaDataAnnotation(&current.ObjectMeta,
				config.GatewayAddressAnnotationKey, name)
		}
		current.SetOwnerReferences(svc.GetOwnerReferences())
		svc.Spec.DeepCopyInto(&current.Spec)

		return nil
	})

	if err != nil {
		return ctrlutil.OperationResultNone, fmt.Errorf("cannot upsert service %q: %w",
			store.GetObjectKey(svc), err)
	}

	u.log.V(1).Info("service upserted", "resource", store.GetObjectKey(svc), "result",
		fmt.Sprintf("%+v", current))

	return op, nil
}

func (u *Updater) upsertConfigMap(cm *corev1.ConfigMap) (ctrlutil.OperationResult, error) {
	u.log.V(2).Info("upsert config-map", "resource", store.GetObjectKey(cm))

	client := u.manager.GetClient()
	current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      cm.GetName(),
		Namespace: cm.GetNamespace(),
	}}

	op, err := ctrlutil.CreateOrUpdate(u.ctx, client, current, func() error {
		// thing that might have been changed by the controler: the owner ref and the data

		// u.log.Info("before", "cm", fmt.Sprintf("%#v\n", current))

		current.SetOwnerReferences(cm.GetOwnerReferences())
		current.Data = make(map[string]string)
		for k, v := range cm.Data {
			current.Data[k] = v
		}

		// u.log.Info("after", "cm", fmt.Sprintf("%#v\n", current))

		return nil
	})

	if err != nil {
		return ctrlutil.OperationResultNone, fmt.Errorf("cannot upsert config-map %q: %w",
			store.GetObjectKey(cm), err)
	}

	u.log.V(1).Info("config-map upserted", "resource", store.GetObjectKey(cm), "result",
		fmt.Sprintf("%+v", current))

	return op, nil
}

func (u *Updater) deleteObject(o client.Object) error {
	return u.manager.GetClient().Delete(u.ctx, o)
}

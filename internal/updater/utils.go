package updater

// updater uploads client updates
import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (u *Updater) updateGatewayClass(gc *gwapiv1a2.GatewayClass, gen int) error {
	u.log.V(2).Info("update gateway class", "resource", store.GetObjectKey(gc), "generation",
		gen)

	cli := u.manager.GetClient()
	current := &gwapiv1a2.GatewayClass{ObjectMeta: metav1.ObjectMeta{
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

	u.log.V(1).Info("gateway-class updated", "resource", store.GetObjectKey(gc), "generation",
		gen, "result", store.DumpObject(current))

	return nil
}

func (u *Updater) updateGateway(gw *gwapiv1a2.Gateway, gen int) error {
	u.log.V(2).Info("updating gateway", "resource", store.GetObjectKey(gw), "generation",
		gen)

	cli := u.manager.GetClient()
	current := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
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

	u.log.V(1).Info("gateway updated", "resource", store.GetObjectKey(gw), "generation", gen,
		"result", store.DumpObject(current))

	return nil
}

func (u *Updater) updateUDPRoute(ro *gwapiv1a2.UDPRoute, gen int) error {
	u.log.V(2).Info("updating UDP-route", "resource", store.GetObjectKey(ro), "generation",
		gen)

	cli := u.manager.GetClient()
	current := &gwapiv1a2.UDPRoute{ObjectMeta: metav1.ObjectMeta{
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

	u.log.V(1).Info("UDP-route updated", "resource", store.GetObjectKey(ro), "generation",
		gen, "result", store.DumpObject(current))

	return nil
}

func (u *Updater) upsertService(svc *corev1.Service, gen int) (ctrlutil.OperationResult, error) {
	u.log.V(2).Info("upsert service", "resource", store.GetObjectKey(svc), "generation", gen)

	client := u.manager.GetClient()
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
	}}

	op, err := ctrlutil.CreateOrUpdate(u.ctx, client, current, func() error {
		// merge metadata
		labs := labels.Merge(current.GetLabels(), svc.GetLabels())
		current.SetLabels(labs)

		annotations := current.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		for k, v := range svc.GetAnnotations() {
			annotations[k] = v
		}
		current.SetAnnotations(annotations)

		if err := addOwnerRef(current, svc); err != nil {
			return err
		}

		// rewrite spec
		svc.Spec.DeepCopyInto(&current.Spec)

		return nil
	})

	if err != nil {
		return ctrlutil.OperationResultNone, fmt.Errorf("cannot upsert service %q: %w",
			store.GetObjectKey(svc), err)
	}

	u.log.V(1).Info("service upserted", "resource", store.GetObjectKey(svc), "generation",
		gen, "result", store.DumpObject(current))

	return op, nil
}

func (u *Updater) upsertConfigMap(cm *corev1.ConfigMap, gen int) (ctrlutil.OperationResult, error) {
	u.log.V(2).Info("upsert config-map", "resource", store.GetObjectKey(cm), "generation",
		gen)

	client := u.manager.GetClient()
	current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      cm.GetName(),
		Namespace: cm.GetNamespace(),
	}}

	op, err := ctrlutil.CreateOrUpdate(u.ctx, client, current, func() error {
		// thing that might have been changed by the controler: the owner ref, annotations
		// and the data

		// u.log.Info("before", "cm", fmt.Sprintf("%#v\n", current))

		current.SetOwnerReferences(cm.GetOwnerReferences())
		current.SetAnnotations(cm.GetAnnotations())
		current.SetLabels(cm.GetLabels())

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

	u.log.V(1).Info("config-map upserted", "resource", store.GetObjectKey(cm), "generation",
		gen, "result", store.DumpObject(current))

	return op, nil
}

func (u *Updater) deleteObject(o client.Object, gen int) error {
	u.log.V(1).Info("delete objec", "resource", store.GetObjectKey(o), "generation", gen)

	return u.manager.GetClient().Delete(u.ctx, o)
}

func addOwnerRef(dst, src client.Object) error {
	ownerRefs := src.GetOwnerReferences()
	if len(ownerRefs) != 1 {
		return fmt.Errorf("addOwnerRef: expecting a singleton ownerRef in %q, found %d",
			store.GetObjectKey(src), len(ownerRefs))
	}
	ownerRef := src.GetOwnerReferences()[0]

	for i, ref := range dst.GetOwnerReferences() {
		if ref.Name == ownerRef.Name && ref.Kind == ownerRef.Kind {
			ownerRefs = dst.GetOwnerReferences()
			ownerRef.DeepCopyInto(&ownerRefs[i])
			dst.SetOwnerReferences(ownerRefs)

			return nil
		}
	}

	ownerRefs = dst.GetOwnerReferences()
	ownerRefs = append(ownerRefs, ownerRef)
	dst.SetOwnerReferences(ownerRefs)

	return nil
}

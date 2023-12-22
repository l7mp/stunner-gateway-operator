package updater

// updater uploads client updates
import (
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func (u *Updater) updateGatewayClass(gc *gwapiv1.GatewayClass, gen int) error {
	u.log.V(2).Info("update gateway class", "resource", store.GetObjectKey(gc), "generation",
		gen)

	cli := u.manager.GetClient()
	current := &gwapiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{
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

func (u *Updater) updateGateway(gw *gwapiv1.Gateway, gen int) error {
	u.log.V(2).Info("updating gateway", "resource", store.GetObjectKey(gw), "generation",
		gen)

	cli := u.manager.GetClient()
	current := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
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

func (u *Updater) updateUDPRoute(ro *stnrgwv1.UDPRoute, gen int) error {
	u.log.V(2).Info("updating UDP-route", "resource", store.GetObjectKey(ro), "generation",
		gen)

	cli := u.manager.GetClient()
	current := &stnrgwv1.UDPRoute{ObjectMeta: metav1.ObjectMeta{
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

func (u *Updater) updateUDPRouteV1A2(ro *stnrgwv1.UDPRoute, gen int) error {
	u.log.V(2).Info("updating UDPRouteV1A2", "resource", store.GetObjectKey(ro), "generation", gen)

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

	u.log.V(1).Info("UDPRouteV1A2 updated", "resource", store.GetObjectKey(ro), "generation",
		gen, "result", store.DumpObject(stnrgwv1.ConvertV1UDPRouteToV1A2(ro)))

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
		if err := mergeMetadata(current, svc); err != nil {
			return nil
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

		if err := mergeMetadata(current, cm); err != nil {
			return nil
		}

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

func (u *Updater) upsertDeployment(dp *appv1.Deployment, gen int) (ctrlutil.OperationResult, error) {
	u.log.V(2).Info("upsert deployment", "resource", store.GetObjectKey(dp), "generation", gen)

	client := u.manager.GetClient()
	current := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      dp.GetName(),
		Namespace: dp.GetNamespace(),
	}}

	// use CreateOrPatch: hopefully more clever than CreateOrUpdate
	op, err := ctrlutil.CreateOrPatch(u.ctx, client, current, func() error {
		if err := mergeMetadata(current, dp); err != nil {
			return nil
		}

		current.Spec.Selector = dp.Spec.Selector

		// only update the replica count if it must be enforced
		if dp.Spec.Replicas != nil && int(*dp.Spec.Replicas) != 1 {
			replicas := *dp.Spec.Replicas
			current.Spec.Replicas = &replicas
		}

		// the pod template is copied verbatim
		dp.Spec.Template.ObjectMeta.DeepCopyInto(&current.Spec.Template.ObjectMeta)
		dpspec := &dp.Spec.Template.Spec
		currentspec := &current.Spec.Template.Spec
		currentspec.Containers = make([]corev1.Container, len(dpspec.Containers))
		for i := range dpspec.Containers {
			dpspec.Containers[i].DeepCopyInto(&currentspec.Containers[i])
		}
		currentspec.Volumes = make([]corev1.Volume, len(dpspec.Volumes))
		for i := range dpspec.Volumes {
			dpspec.Volumes[i].DeepCopyInto(&currentspec.Volumes[i])
		}

		// rest is optional
		if dpspec.TerminationGracePeriodSeconds != nil {
			currentspec.TerminationGracePeriodSeconds = dpspec.TerminationGracePeriodSeconds
		}

		currentspec.HostNetwork = dpspec.HostNetwork

		// affinity
		if dpspec.Affinity != nil {
			currentspec.Affinity = dpspec.Affinity
		}

		// tolerations
		if dpspec.Tolerations != nil {
			currentspec.Tolerations = dpspec.Tolerations
		}

		// security context
		if dpspec.SecurityContext != nil {
			currentspec.SecurityContext = dpspec.SecurityContext
		}

		// u.log.Info("after", "cm", fmt.Sprintf("%#v\n", current))

		return nil
	})

	// u.log.V(4).Info("upserting deployment", "resource", store.GetObjectKey(dp), "generation",
	// 	gen, "deployment", store.DumpObject(dp))

	if err != nil {
		return ctrlutil.OperationResultNone, fmt.Errorf("cannot upsert deployment %q: %w",
			store.GetObjectKey(dp), err)
	}

	u.log.V(1).Info("deployment upserted", "resource", store.GetObjectKey(dp), "generation",
		gen, "result", store.DumpObject(current))

	return op, nil
}

func (u *Updater) deleteObject(o client.Object, gen int) error {
	u.log.V(2).Info("delete object", "resource", store.GetObjectKey(o), "generation", gen)

	return u.manager.GetClient().Delete(u.ctx, o)
}

func mergeMetadata(dst, src client.Object) error {
	labs := labels.Merge(dst.GetLabels(), src.GetLabels())
	dst.SetLabels(labs)

	annotations := dst.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	for k, v := range src.GetAnnotations() {
		annotations[k] = v
	}
	dst.SetAnnotations(annotations)

	return addOwnerRef(dst, src)
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

package lens

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1 "k8s.io/api/apps/v1"
)

type DaemonSetLens struct {
	appv1.DaemonSet `json:",inline"`
}

func NewDaemonSetLens(d *appv1.DaemonSet) *DaemonSetLens {
	return &DaemonSetLens{DaemonSet: *d.DeepCopy()}
}

func (l *DaemonSetLens) EqualResource(current client.Object) bool {
	ds, ok := current.(*appv1.DaemonSet)
	if !ok {
		return false
	}

	return apiequality.Semantic.DeepEqual(projectDaemonSet(ds, &l.DaemonSet), projectDaemonSet(&l.DaemonSet, &l.DaemonSet))
}

func (l *DaemonSetLens) ApplyToResource(target client.Object) error {
	ds, ok := target.(*appv1.DaemonSet)
	if !ok {
		return fmt.Errorf("daemonset lens: invalid target type %T", target)
	}

	return applyDaemonSet(ds, &l.DaemonSet)
}

func (l *DaemonSetLens) EqualStatus(_ client.Object) bool {
	return true
}

func (l *DaemonSetLens) ApplyToStatus(_ client.Object) error {
	return nil
}

func (l *DaemonSetLens) DeepCopy() *DaemonSetLens {
	return &DaemonSetLens{DaemonSet: *l.DaemonSet.DeepCopy()}
}

func (l *DaemonSetLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

// * DaemonSet.ObjectMeta.Labels / DaemonSet.ObjectMeta.Annotations / DaemonSet.ObjectMeta.OwnerReferences
// - renderer: in managed mode, dataplane rendering may emit a DaemonSet object with operator-owned
//   metadata and Gateway owner reference.
// - updater: merges top-level metadata and updates owner reference via setMetadata/addOwnerRef.
//
// * DaemonSet.Spec.Selector
// - renderer: same as Deployment
// - updater: deep-copies desired selector into current selector.
//
// * DaemonSet.Spec.Template.ObjectMeta + DaemonSet.Spec.Template.Spec owned fields
// - renderer: same policy as Deployment pod-template rendering.
// - updater: shared helper applyPodTemplateSpec performs deep-copy/overwrite semantics.
//   See deployment.go for the full per-field pod-template policy list.

func applyDaemonSet(current, desired *appv1.DaemonSet) error {
	if err := setMetadata(current, desired); err != nil {
		return err
	}

	current.Spec.Selector = copyLabelSelector(desired.Spec.Selector)
	applyPodTemplateSpec(&current.Spec.Template, &desired.Spec.Template)

	return nil
}

func projectDaemonSet(d, owned *appv1.DaemonSet) *appv1.DaemonSet {
	src := d.DeepCopy()
	k8sscheme.Scheme.Default(src)

	ret := &appv1.DaemonSet{ObjectMeta: projectMetadata(src, owned)}
	ret.Spec.Selector = copyLabelSelector(src.Spec.Selector)
	ret.Spec.Template.ObjectMeta = projectTemplateMeta(&src.Spec.Template, &owned.Spec.Template)
	ret.Spec.Template.Spec = projectPodSpec(&src.Spec.Template.Spec)
	return ret
}

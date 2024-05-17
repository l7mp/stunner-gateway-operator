/*
Copyright 2022 The l7mp/stunner team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"

	. "github.com/onsi/gomega"

	// "k8s.io/client-go/kubernetes/scheme"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

type GatewayClassMutator func(current *gwapiv1.GatewayClass)

func createOrUpdateGatewayClass(template *gwapiv1.GatewayClass, f GatewayClassMutator) {
	current := &gwapiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type GatewayConfigMutator func(current *stnrgwv1.GatewayConfig)

func createOrUpdateGatewayConfig(template *stnrgwv1.GatewayConfig, f GatewayConfigMutator) {
	current := &stnrgwv1.GatewayConfig{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type GatewayMutator func(current *gwapiv1.Gateway)

func createOrUpdateGateway(template *gwapiv1.Gateway, f GatewayMutator) {
	current := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type ServiceMutator func(current *corev1.Service)

func createOrUpdateService(template *corev1.Service, f ServiceMutator) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type StaticServiceMutator func(current *stnrgwv1.StaticService)

func createOrUpdateStaticService(template *stnrgwv1.StaticService, f StaticServiceMutator) {
	current := &stnrgwv1.StaticService{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type EndpointsMutator func(current *corev1.Endpoints)

func createOrUpdateEndpoints(template *corev1.Endpoints, f EndpointsMutator) {
	current := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		current.Subsets = make([]corev1.EndpointSubset, len(template.Subsets))
		for i := range template.Subsets {
			template.Subsets[i].DeepCopyInto(&current.Subsets[i])
		}
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type EndpointSliceMutator func(current *discoveryv1.EndpointSlice)

func createOrUpdateEndpointSlice(template *discoveryv1.EndpointSlice, f EndpointSliceMutator) {
	current := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		// copy the label otherwise the endpointslice will not attach to the svc
		current.SetLabels(template.GetLabels())
		current.AddressType = template.AddressType
		current.Endpoints = make([]discoveryv1.Endpoint, len(template.Endpoints))
		for i := range template.Endpoints {
			template.Endpoints[i].DeepCopyInto(&current.Endpoints[i])
		}
		current.Ports = make([]discoveryv1.EndpointPort, len(template.Ports))
		for i := range template.Ports {
			template.Ports[i].DeepCopyInto(&current.Ports[i])
		}
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type SecretMutator func(current *corev1.Secret)

func createOrUpdateSecret(template *corev1.Secret, f SecretMutator) {
	current := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		current.Type = template.Type
		current.Data = make(map[string][]byte)
		for k, v := range template.Data {
			current.Data[k] = v
		}
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type UDPRouteMutator func(current *stnrgwv1.UDPRoute)

func createOrUpdateUDPRoute(template *stnrgwv1.UDPRoute, f UDPRouteMutator) {
	current := &stnrgwv1.UDPRoute{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())
}

type NodeMutator func(current *corev1.Node)

func statusUpdateNode(name string, f NodeMutator) {
	current := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: name,
	}}

	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(current), current)
	Expect(err).Should(Succeed())

	if f != nil {
		f(current)
	}

	err = k8sClient.Status().Update(ctx, current)
	Expect(err).Should(Succeed())
}

// also updates status
func createOrUpdateNode(template *corev1.Node, f NodeMutator) {
	current := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:      template.GetName(),
		Namespace: template.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.SetLabels(template.GetLabels())
		template.Spec.DeepCopyInto(&current.Spec)
		if f != nil {
			f(current)
		}
		return nil
	})
	Expect(err).Should(Succeed())

	template.Status.DeepCopyInto(&current.Status)
	err = k8sClient.Status().Update(ctx, current)
	Expect(err).Should(Succeed())
}

// createOrUpdate will retry when ctrlutil.CreateOrUpdate fails. This is useful to robustify tests:
// sometimes an update is going on while we are trying to run the next test and then the CreateOrUpdate
// may fail with a Conflict.
func createOrUpdate(ctx context.Context, c client.Client, obj client.Object, f ctrlutil.MutateFn) (ctrlutil.OperationResult, error) {
	res, err := ctrlutil.CreateOrUpdate(ctx, c, obj, f)
	if err == nil {
		return res, err
	}

	for i := 1; i < 5; i++ {
		res, err = ctrlutil.CreateOrUpdate(ctx, c, obj, f)
		if err == nil {
			return res, err
		}
	}

	return res, err
}

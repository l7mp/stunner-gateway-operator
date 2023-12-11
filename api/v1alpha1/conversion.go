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

package v1alpha1

import (
	v1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	conv "sigs.k8s.io/controller-runtime/pkg/conversion"
)

// Mandatory conversions:
//
// * health-check-port: cannot be set in the Dataplane and the GatewayConfig any more (this resulted
//   in possibly conflicting settings): health-check port is now fixed at 8086, but health-checks
//   can be explicitly disabled in using DisableHealthCheck in the Dataplane v1 spec
//
// * metrics endpoint: cannot be set in the GatewayConfig any more, only specifically enabled at a
//   fix port in Dataplane.v1 (this makes the generation of the Kubernetes manifests simpler)
//
// * MinPort/MaxPort cannot be set in the GatewayConfig any more: it did not make sense anyway (the
//   setting used to configure the *source* port range for generating transport relay addresses) so
//   it was substituted with a way to specify the *target* port range in the UDPRoute CR
//
// * Port-range setting in StaticServices have been removed (it used to be omitted anyway)

var _ conv.Convertible = &Dataplane{}
var _ conv.Convertible = &GatewayConfig{}
var _ conv.Convertible = &StaticService{}

// Dataplane conversion:
func (in *Dataplane) ConvertTo(hub conv.Hub) error {
	s, err := SchemeBuilder.Build()
	if err != nil {
		return err
	}
	return s.Convert(in, hub.(*v1.Dataplane), nil)
}

func (in *Dataplane) ConvertFrom(hub conv.Hub) error {
	s, err := SchemeBuilder.Build()
	if err != nil {
		return err
	}
	return s.Convert(hub.(*v1.Dataplane), in, nil)
}

func Convert_v1alpha1_DataplaneSpec_To_v1_DataplaneSpec(in *DataplaneSpec, out *v1.DataplaneSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_DataplaneSpec_To_v1_DataplaneSpec(in, out, s)

	if in.HealthCheckPort != nil && *in.HealthCheckPort == 0 {
		out.DisableHealthCheck = true
	}

	return err
}

func Convert_v1_DataplaneSpec_To_v1alpha1_DataplaneSpec(in *v1.DataplaneSpec, out *DataplaneSpec, s conversion.Scope) error {
	err := autoConvert_v1_DataplaneSpec_To_v1alpha1_DataplaneSpec(in, out, s)

	if in.DisableHealthCheck {
		out.HealthCheckPort = nil
	}

	return err
}

// GatewayConfig
func (in *GatewayConfig) ConvertTo(hub conv.Hub) error {
	s, err := SchemeBuilder.Build()
	if err != nil {
		return err
	}
	return s.Convert(in, hub.(*v1.GatewayConfig), nil)
}

func (in *GatewayConfig) ConvertFrom(hub conv.Hub) error {
	s, err := SchemeBuilder.Build()
	if err != nil {
		return err
	}
	return s.Convert(hub.(*v1.GatewayConfig), in, nil)
}

func Convert_v1alpha1_GatewayConfigSpec_To_v1_GatewayConfigSpec(in *GatewayConfigSpec, out *v1.GatewayConfigSpec, s conversion.Scope) error {
	err := autoConvert_v1alpha1_GatewayConfigSpec_To_v1_GatewayConfigSpec(in, out, s)

	out.Realm = in.Realm
	out.AuthType = in.AuthType
	out.Username = in.Username
	out.Password = in.Password
	out.SharedSecret = in.SharedSecret
	out.AuthLifetime = in.AuthLifetime
	out.AuthRef = in.AuthRef
	out.LoadBalancerServiceAnnotations = in.LoadBalancerServiceAnnotations
	out.LogLevel = in.LogLevel
	out.Dataplane = in.Dataplane

	return err
}

// StaticService
func (in *StaticService) ConvertTo(hub conv.Hub) error {
	s, err := SchemeBuilder.Build()
	if err != nil {
		return err
	}
	return s.Convert(in, hub.(*v1.StaticService), nil)
}

func (in *StaticService) ConvertFrom(hub conv.Hub) error {
	s, err := SchemeBuilder.Build()
	if err != nil {
		return err
	}
	return s.Convert(hub.(*v1.StaticService), in, nil)
}

func Convert_v1alpha1_StaticServiceSpec_To_v1_StaticServiceSpec(in *StaticServiceSpec, out *v1.StaticServiceSpec, s conversion.Scope) error {
	return autoConvert_v1alpha1_StaticServiceSpec_To_v1_StaticServiceSpec(in, out, s)
}

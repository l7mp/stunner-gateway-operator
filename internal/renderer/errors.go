package renderer

import (
	"fmt"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ErrorType specifies the type of a rendering error.
type ErrorType int

const (
	NoError ErrorType = iota

	// critical
	InvalidAuthType
	InvalidUsernamePassword
	InvalidSharedSecret
	InvalidDataplane
	NoRuleFound
	ExternalAuthCredentialsNotFound
	InvalidAuthConfig
	RenderingError
	InternalError

	// noncritical
	InvalidBackendGroup
	InvalidBackendKind
	BackendNotFound
	ServiceNotFound
	ClusterIPNotFound
	EndpointNotFound
	InvalidEndpoint
	InvalidTunnelURI
	TunnelAuthNotFound
	InvalidTunnelAuth
	InvalidProtocol
	PortUnavailable
	InvalidCertificateRef
	InvalidPortRange
	PublicAddressNotFound
	PublicListenerAddressNotFound
)

type TypedError struct {
	reason ErrorType
}

// CriticalError is a fatal rendering error that prevents the rendering the dataplane config as a
// whole, or in parts.
type CriticalError struct {
	TypedError
}

// NewCriticalError creates a new fatal error.
func NewCriticalError(reason ErrorType) error {
	return &CriticalError{TypedError{reason: reason}}
}

// Error returns an error message.
func (e *CriticalError) Error() string {
	switch e.reason {
	case InvalidAuthType:
		return "invalid authentication type"
	case InvalidUsernamePassword:
		return "missing username and/or password for plaintext authentication"
	case InvalidSharedSecret:
		return "missing shared-secret for longterm authentication"
	case InvalidAuthConfig:
		return "internal error: could not validate generated auth config"
	case InvalidDataplane:
		return "missing Dataplane resource for Gateway"
	case NoRuleFound:
		return "no rules found in route"
	case ExternalAuthCredentialsNotFound:
		return "missing or invalid external authentication credentials"
	case RenderingError:
		return "could not render dataplane config"
	case InternalError:
		return "internal error"
	}
	return "Unknown error"
}

// NonCriticalError is a non-fatal error that affects a Gateway or a Route status: this is an event
// that is worth reporting but otherwise does not prevent the rendering of a valid dataplane
// config.
type NonCriticalError struct {
	TypedError
}

// NewNonCriticalError creates a new non-critical render error object.
func NewNonCriticalError(reason ErrorType) error {
	return &NonCriticalError{TypedError{reason: reason}}
}

// Error returns an error message.
func (e *NonCriticalError) Error() string {
	switch e.reason {
	case InvalidBackendGroup:
		return "invalid Group in backend reference (expecting core or stunner.l7mp.io)"
	case InvalidBackendKind:
		return "invalid Kind in backend reference (expecting Service or StaticService)"
	case BackendNotFound:
		return "backend not found"
	case ServiceNotFound:
		return "no Service found for backend"
	case ClusterIPNotFound:
		return "no ClusterIP found for Service (this is fine for headless Services)"
	case EndpointNotFound:
		return "no endpoints found for backend"
	case InvalidTunnelURI:
		return "invalid tunnel URI (expecting a TURN URI)"
	case TunnelAuthNotFound:
		return "missing or invalid Secret for the tunnel authentication"
	case InvalidTunnelAuth:
		return "invalid tunnel authentication in Secret"
	case InvalidEndpoint:
		return "invalid backend address (expecting an IP address or prefix)"
	case PortUnavailable:
		return "port unavailable"
	case InvalidCertificateRef:
		return "invalid certificate reference"
	case InvalidProtocol:
		return "invalid protocol"
	case InvalidPortRange:
		return "invalid port range"
	case PublicAddressNotFound:
		return "no public address found for gateway"
	case PublicListenerAddressNotFound:
		return "no public address found for one or more listeners"
	}
	return "Unknown error"
}

// ParametersError is why a Gateway is not accepted for its parameters, with the reason of its
// Accepted status condition.
type ParametersError struct {
	Reason  gwapiv1.GatewayConditionReason
	message string
}

// NewParametersError creates a new parameters error.
func NewParametersError(reason gwapiv1.GatewayConditionReason, format string, args ...any) error {
	return &ParametersError{Reason: reason, message: fmt.Sprintf(format, args...)}
}

// Error returns an error message.
func (e *ParametersError) Error() string { return e.message }

// IsCritical returns true if an error is critical.
func IsCritical(e error) bool {
	_, ok := e.(*CriticalError)
	return ok
}

// IsNonCritical returns true if an error is non-critical.
func IsNonCritical(e error) bool {
	_, ok := e.(*NonCriticalError)
	return ok
}

// IsNonCriticalError returns true if an error is a non-critical error of the given type.
func IsNonCriticalError(e error, reason ErrorType) bool {
	err, ok := e.(*NonCriticalError)
	return ok && err.reason == reason
}

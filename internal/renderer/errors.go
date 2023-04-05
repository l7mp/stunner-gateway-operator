package renderer

// ErrorType species the type of a non-critical rendering error
type ErrorType int

const (
	NoError ErrorType = iota

	// critical
	InvalidAuthType
	InvalidUsernamePassword
	InvalidSharedSecret
	ExternalAuthCredentialsNotFound
	InvalidAuthConfig
	ConfigMapRenderingError

	// noncritical
	InvalidBackendGroup
	InvalidBackendKind
	ServiceNotFound
	ClusterIPNotFound
	EndpointNotFound
)

type TypedError struct {
	reason ErrorType
}

// CriticalError is a fatal rendering error that prevents the rendering of a dataplane config.
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
		return "missing username and/or password for plaintext authetication"
	case InvalidSharedSecret:
		return "missing shared-secret for longterm authetication"
	case InvalidAuthConfig:
		return "internal error: could not validate generated auth config"
	case ExternalAuthCredentialsNotFound:
		return "missing or invalid external authentication credentials"
	case ConfigMapRenderingError:
		return "could not render dataplane config"
	}
	return "Unknown error"
}

// NonCriticalError is a non-fatal error that affects a Gateway or a Route status.
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
		return "Invalid Group in backend reference (expecing: None)"
	case InvalidBackendKind:
		return "Invalid Kind in backend reference (expecting Service)"
	case ServiceNotFound:
		return "No Service found for backend"
	case ClusterIPNotFound:
		return "No ClusterIP found (use a STRICT_DNS cluster if service is headless)"
	case EndpointNotFound:
		return "No Endpoint found for backend"
	}
	return "Unknown error"
}

// IsCritical returns true of an error is critical.
func IsCritical(e error) bool {
	_, ok := e.(*CriticalError)
	return ok
}

// IsCriticalError returns true of an error is a critical error of the given type.
func IsCriticalError(e error, reason ErrorType) bool {
	err, ok := e.(*CriticalError)
	return ok && err.reason == reason
}

// IsNonCritical returns true of an error is critical.
func IsNonCritical(e error) bool {
	_, ok := e.(*NonCriticalError)
	return ok
}

// IsNonCriticalError returns true of an error is a critical error of the given type.
func IsNonCriticalError(e error, reason ErrorType) bool {
	err, ok := e.(*NonCriticalError)
	return ok && err.reason == reason
}

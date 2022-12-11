package renderer

// NonCriticalRenderErrorType species the type of a non-critical rendering error
type NonCriticalRenderErrorType int

const (
	NoError NonCriticalRenderErrorType = iota
	InvalidBackendGroup
	InvalidBackendKind
	ServiceNotFound
	ClusterIPNotFound
	EndpointNotFound
)

// NonCriticalRenderError is a non-fatal rendering error that affects a Gateway or a Route status
type NonCriticalRenderError struct {
	ErrorReason NonCriticalRenderErrorType
}

// Error returns an error message
func (e NonCriticalRenderError) Error() string {
	switch e.ErrorReason {
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
	return "No error"
}

// NewNonCriticalRenderError creates a new non-critical render error object
func NewNonCriticalRenderError(reason NonCriticalRenderErrorType) NonCriticalRenderError {
	return NonCriticalRenderError{ErrorReason: reason}
}

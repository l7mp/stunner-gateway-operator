package store

import (
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// TURNRoutes stores the TURNRoute objects.
var TURNRoutes = NewTypedStore[*stnrgwv1a1.TURNRoute]()

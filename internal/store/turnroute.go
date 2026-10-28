package store

import (
	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

// TURNRoutes stores the TURNRoute objects.
var TURNRoutes = NewTypedStore[*stnrgwv1.TURNRoute]()

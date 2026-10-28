package store

import (
	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

// TCPRoutes stores the STUNner-native TCPRoute objects.
var TCPRoutes = NewTypedStore[*stnrgwv1.TCPRoute]()

// TCPRoutesGwAPI stores the official Gateway API TCPRoute objects (watched at whichever API
// version the cluster serves), converted to the STUNner-native TCPRoute representation.
var TCPRoutesGwAPI = NewTypedStore[*stnrgwv1.TCPRoute]()

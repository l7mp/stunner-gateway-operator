package store

import (
	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

// UDPRoutes stores the STUNner-native UDPRoute objects.
var UDPRoutes = NewTypedStore[*stnrgwv1.UDPRoute]()

// UDPRoutesGwAPI stores the official Gateway API UDPRoute objects (watched at whichever API
// version the cluster serves), converted to the STUNner-native UDPRoute representation.
var UDPRoutesGwAPI = NewTypedStore[*stnrgwv1.UDPRoute]()

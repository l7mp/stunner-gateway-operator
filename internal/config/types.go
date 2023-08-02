// Package config allows to override some of the default settings from the exported default config
// package.
package config

// DataplaneModeType species the type of the STUN/TURN authentication mechanism used by STUNner
type DataplaneModeType int

const (
	DataplaneModeManaged DataplaneModeType = iota // default
	DataplaneModeLegacy
)

const (
	dataplaneModeManagedStr = "managed"
	dataplaneModeLegacyStr  = "legacy"
)

// NewDataplaneModeType parses the dataplane mode specification.
func NewDataplaneMode(raw string) DataplaneModeType {
	switch raw {
	case dataplaneModeManagedStr:
		return DataplaneModeManaged
	case dataplaneModeLegacyStr:
		return DataplaneModeLegacy
	default:
		return DataplaneModeManaged
	}
}

// String returns a string representation for the dataplane mode.
func (a DataplaneModeType) String() string {
	switch a {
	case DataplaneModeManaged:
		return dataplaneModeManagedStr
	case DataplaneModeLegacy:
		return dataplaneModeLegacyStr
	default:
		return "<unknown>"
	}
}

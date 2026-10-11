package config

import "strings"

// DataplaneModeType specifies the dataplane mode.
type DataplaneModeType int

const (
	DataplaneModeManaged DataplaneModeType = iota // default
	DataplaneModeLegacy
)

const (
	dataplaneModeManagedStr = "managed"
	dataplaneModeLegacyStr  = "legacy"
)

// NewDataplaneMode parses the dataplane mode specification.
func NewDataplaneMode(raw string) DataplaneModeType {
	switch strings.ToLower(raw) {
	case dataplaneModeManagedStr:
		return DataplaneModeManaged
	case dataplaneModeLegacyStr:
		return DataplaneModeLegacy
	default:
		return DataplaneModeLegacy
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

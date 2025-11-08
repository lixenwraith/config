// FILE: lixenwraith/config/constant.go
package config

import "time"

// Timing constants for production use
const (
	SpinWaitInterval     = 5 * time.Millisecond
	MinPollInterval      = 100 * time.Millisecond
	ShutdownTimeout      = 100 * time.Millisecond
	DefaultDebounce      = 500 * time.Millisecond
	DefaultPollInterval  = time.Second
	DefaultReloadTimeout = 5 * time.Second
)

// Network validation limits
const (
	MaxIPv6Length = 45   // Maximum IPv6 address string length
	MaxCIDRLength = 49   // Maximum IPv6 CIDR string length
	MaxURLLength  = 2048 // Maximum URL string length
	MinPortNumber = 1
	MaxPortNumber = 65535
)

// File system permissions
const (
	DirPermissions  = 0755
	FilePermissions = 0644
)

// Format identifiers
const (
	FormatTOML = "toml"
	FormatJSON = "json"
	FormatYAML = "yaml"
	FormatAuto = "auto"
)

// Watch event types
const (
	EventFileDeleted        = "file_deleted"
	EventPermissionsChanged = "permissions_changed"
	EventReloadError        = "reload_error"
	EventReloadTimeout      = "reload_timeout"
	EventPrecedenceChanged  = "precedence"
)

// debounceSettleMultiplier ensures sufficient time for debounce to complete
const debounceSettleMultiplier = 3 // Wait 3x debounce period for value stabilization

// Channel and resource limits
const (
	DefaultMaxWatchers = 100
	WatchChannelBuffer = 10
	MaxValueSize       = 1024 * 1024 // 1MB
)

// Network defaults
const (
	IPv4Any = "0.0.0.0"
	IPv6Any = "::"
)

// File discovery defaults
var (
	DefaultConfigExtensions = []string{".toml", ".conf", ".config"}
	XDGSystemPaths          = []string{"/etc/xdg", "/etc"}
)
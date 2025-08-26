// FILE: lixenwraith/config/timing.go
package config

import "time"

// Core timing constants for production use.
// These define the fundamental timing behavior of the config package.
const (
	// File watching intervals (ordered by frequency)
	SpinWaitInterval     = 5 * time.Millisecond   // CPU-friendly busy-wait quantum
	MinPollInterval      = 100 * time.Millisecond // Hard floor for file stat polling
	ShutdownTimeout      = 100 * time.Millisecond // Graceful watcher termination window
	DefaultDebounce      = 500 * time.Millisecond // File change coalescence period
	DefaultPollInterval  = time.Second            // Standard file monitoring frequency
	DefaultReloadTimeout = 5 * time.Second        // Maximum duration for reload operations
)

// Derived timing relationships for internal use.
// These maintain consistent ratios between related timers.
const (
	// shutdownPollCycles defines how many spin-wait cycles comprise a shutdown timeout
	shutdownPollCycles = ShutdownTimeout / SpinWaitInterval // = 20 cycles

	// debounceSettleMultiplier ensures sufficient time for debounce to complete
	debounceSettleMultiplier = 3 // Wait 3x debounce period for value stabilization
)
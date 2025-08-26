// FILE: lixenwraith/config/watch.go
package config

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultMaxWatchers = 100 // Prevent resource exhaustion

// WatchOptions configures file watching behavior
type WatchOptions struct {
	// PollInterval for file stat checks (minimum 100ms)
	PollInterval time.Duration

	// Debounce duration to avoid rapid reloads
	Debounce time.Duration

	// MaxWatchers limits concurrent watch channels
	MaxWatchers int

	// ReloadTimeout for file reload operations
	ReloadTimeout time.Duration

	// VerifyPermissions checks file hasn't been replaced with different permissions
	VerifyPermissions bool
}

// DefaultWatchOptions returns sensible defaults for file watching
func DefaultWatchOptions() WatchOptions {
	return WatchOptions{
		PollInterval:      DefaultPollInterval,
		Debounce:          DefaultDebounce,
		MaxWatchers:       DefaultMaxWatchers,
		ReloadTimeout:     DefaultReloadTimeout,
		VerifyPermissions: true,
	}
}

// watcher manages file watching state
type watcher struct {
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	opts             WatchOptions
	filePath         string
	lastModTime      time.Time
	lastSize         int64
	lastMode         os.FileMode
	watching         atomic.Bool
	reloadInProgress atomic.Bool
	watchers         map[int64]chan string // subscriber channels
	watcherID        atomic.Int64
	debounceTimer    *time.Timer
}

// configWatcher extends Config with watching capabilities
type configWatcher struct {
	*Config
	watcher *watcher
}

// AutoUpdate enables automatic configuration reloading when the file changes
func (c *Config) AutoUpdate() {
	c.AutoUpdateWithOptions(DefaultWatchOptions())
}

// AutoUpdateWithOptions enables automatic configuration reloading with custom options
func (c *Config) AutoUpdateWithOptions(opts WatchOptions) {
	// Validate options
	if opts.PollInterval < MinPollInterval {
		opts.PollInterval = MinPollInterval
	}
	if opts.MaxWatchers <= 0 {
		opts.MaxWatchers = 100
	}
	if opts.ReloadTimeout <= 0 {
		opts.ReloadTimeout = DefaultReloadTimeout
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Get path of current file to watch
	filePath := c.getConfigFilePath()
	if filePath == "" {
		// No file configured, nothing to watch
		return
	}

	// Stop existing watcher if path changed
	if c.watcher != nil && c.watcher.filePath != filePath {
		c.watcher.stop()
		c.watcher = nil
	}

	// Initialize watcher if needed
	if c.watcher == nil {
		ctx, cancel := context.WithCancel(context.Background())
		c.watcher = &watcher{
			ctx:      ctx,
			cancel:   cancel,
			opts:     opts,
			filePath: filePath,
			watchers: make(map[int64]chan string),
		}

		// Get initial file state
		if info, err := os.Stat(filePath); err == nil {
			c.watcher.lastModTime = info.ModTime()
			c.watcher.lastSize = info.Size()
			c.watcher.lastMode = info.Mode()
		}

		// Start watching
		go c.watcher.watchLoop(c)
	}
}

// StopAutoUpdate stops automatic configuration reloading
func (c *Config) StopAutoUpdate() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.watcher != nil {
		c.watcher.stop()
		c.watcher = nil
	}
}

// Watch returns a channel that receives paths of changed configuration values
func (c *Config) Watch() <-chan string {
	return c.WatchWithOptions(DefaultWatchOptions())
}

// WatchFile stops any existing file watcher, loads a new configuration file,
// and starts a new watcher on that file path. Optionally accepts format hint.
func (c *Config) WatchFile(filePath string, formatHint ...string) error {
	// Stop any currently running watcher
	c.StopAutoUpdate()

	// Set format hint if provided
	if len(formatHint) > 0 {
		if err := c.SetFileFormat(formatHint[0]); err != nil {
			return fmt.Errorf("invalid format hint: %w", err)
		}
	}

	// Load the new file
	if err := c.LoadFile(filePath); err != nil {
		return fmt.Errorf("failed to load new file for watching: %w", err)
	}

	// Get previous watcher options if available
	c.mutex.RLock()
	opts := DefaultWatchOptions()
	if c.watcher != nil {
		opts = c.watcher.opts
	}
	c.mutex.RUnlock()

	// Start new watcher (AutoUpdateWithOptions will create a new watcher with the new file path)
	c.AutoUpdateWithOptions(opts)
	return nil
}

// WatchWithOptions returns a channel with custom watch options
// should not restart the watcher if it's already running with the same file
func (c *Config) WatchWithOptions(opts WatchOptions) <-chan string {
	c.mutex.RLock()
	watcher := c.watcher
	filePath := c.configFilePath
	c.mutex.RUnlock()

	// If no file configured, return closed channel
	if filePath == "" {
		ch := make(chan string)
		close(ch)
		return ch
	}

	// If watcher exists and is watching the current file, just subscribe
	if watcher != nil && watcher.filePath == filePath && watcher.watching.Load() {
		return watcher.subscribe()
	}

	// First ensure auto-update is running
	c.AutoUpdateWithOptions(opts)

	c.mutex.RLock()
	watcher = c.watcher
	c.mutex.RUnlock()

	if watcher == nil {
		// No file to watch, return closed channel
		ch := make(chan string)
		close(ch)
		return ch
	}

	return watcher.subscribe()
}

// IsWatching returns true if auto-update is enabled
func (c *Config) IsWatching() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.watcher != nil && c.watcher.watching.Load()
}

// WatcherCount returns the number of active watch channels
func (c *Config) WatcherCount() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.watcher == nil {
		return 0
	}

	c.watcher.mu.RLock()
	defer c.watcher.mu.RUnlock()
	return len(c.watcher.watchers)
}

// watchLoop is the main file watching loop
func (w *watcher) watchLoop(c *Config) {
	if !w.watching.CompareAndSwap(false, true) {
		return // Already watching
	}
	defer w.watching.Store(false)

	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.checkAndReload(c)
		}
	}
}

// checkAndReload checks if file changed and triggers reload
func (w *watcher) checkAndReload(c *Config) {
	info, err := os.Stat(w.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted, notify watchers
			w.notifyWatchers("file_deleted")
		}
		return
	}

	// Check for changes
	changed := false

	// Compare modification time and size
	if !info.ModTime().Equal(w.lastModTime) || info.Size() != w.lastSize {
		changed = true
	}

	// SECURITY: Verify permissions haven't changed suspiciously
	if w.opts.VerifyPermissions && w.lastMode != 0 {
		if info.Mode() != w.lastMode {
			// Permission change detected
			if (info.Mode() & 0077) != (w.lastMode & 0077) {
				// World/group permissions changed - potential security issue
				w.notifyWatchers("permissions_changed")
				// Don't reload on permission change for security
				return
			}
		}
	}

	if changed {
		// Update tracked state
		w.lastModTime = info.ModTime()
		w.lastSize = info.Size()
		w.lastMode = info.Mode()

		// Debounce rapid changes
		w.mu.Lock()
		if w.debounceTimer != nil {
			w.debounceTimer.Stop()
		}
		w.debounceTimer = time.AfterFunc(w.opts.Debounce, func() {
			w.performReload(c)
		})
		w.mu.Unlock()
	}
}

// performReload reloads the configuration file
func (w *watcher) performReload(c *Config) {
	// Prevent concurrent reloads
	if !w.reloadInProgress.CompareAndSwap(false, true) {
		return
	}
	defer w.reloadInProgress.Store(false)

	// Create a timeout context for reload
	ctx, cancel := context.WithTimeout(w.ctx, w.opts.ReloadTimeout)
	defer cancel()

	// Track what changed
	oldValues := c.snapshot()

	// Reload file in a goroutine with timeout
	done := make(chan error, 1)
	go func() {
		done <- c.loadFile(w.filePath)
	}()

	select {
	case err := <-done:
		if err != nil {
			// Reload failed, notify error
			w.notifyWatchers(fmt.Sprintf("reload_error:%v", err))
			return
		}

		// Compare and notify changes
		newValues := c.snapshot()
		for path, newVal := range newValues {
			if oldVal, existed := oldValues[path]; !existed || !reflect.DeepEqual(oldVal, newVal) {
				w.notifyWatchers(path)
			}
		}

		// Check for deletions
		for path := range oldValues {
			if _, exists := newValues[path]; !exists {
				w.notifyWatchers(path)
			}
		}

	case <-ctx.Done():
		// Reload timeout
		w.notifyWatchers("reload_timeout")
	}
}

// subscribe creates a new watcher channel
func (w *watcher) subscribe() <-chan string {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check watcher limit
	if len(w.watchers) >= w.opts.MaxWatchers {
		// Return closed channel to prevent resource exhaustion
		ch := make(chan string)
		close(ch)
		return ch
	}

	// Create buffered channel to prevent blocking
	ch := make(chan string, 10)
	id := w.watcherID.Add(1)
	w.watchers[id] = ch

	// Cleanup goroutine
	go func() {
		<-w.ctx.Done()
		w.mu.Lock()
		delete(w.watchers, id)
		close(ch)
		w.mu.Unlock()
	}()

	return ch
}

// notifyWatchers sends change notification to all subscribers
func (w *watcher) notifyWatchers(path string) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for id, ch := range w.watchers {
		select {
		case ch <- path:
			// Sent successfully
		default:
			// Channel full or closed, skip
			// Could implement removal of dead watchers here
			_ = id
		}
	}
}

// stop terminates the watcher
func (w *watcher) stop() {
	if w.cancel != nil {
		w.cancel()
	}

	// Stop debounce timer
	w.mu.Lock()
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
		w.debounceTimer = nil
	}
	w.mu.Unlock()

	// Wait for watch loop to exit with timeout
	deadline := time.Now().Add(ShutdownTimeout)
	for w.watching.Load() && time.Now().Before(deadline) {
		time.Sleep(SpinWaitInterval)
	}
}

// getConfigFilePath returns the current config file path
func (c *Config) getConfigFilePath() string {
	// Access the tracked config file path
	return c.configFilePath
}

// snapshot creates a snapshot of current values
func (c *Config) snapshot() map[string]any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	snapshot := make(map[string]any, len(c.items))
	for path, item := range c.items {
		snapshot[path] = item.currentValue
	}
	return snapshot
}
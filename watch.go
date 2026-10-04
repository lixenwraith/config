package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

type WatchOptions struct {
	PollInterval      time.Duration // Minimum 100ms; each poll reads and fingerprints the file.
	Debounce          time.Duration // Quiet interval before publishing a value or error.
	MaxWatchers       int
	ReloadTimeout     time.Duration // Expired reads/parses cannot publish.
	VerifyPermissions bool
}

func DefaultWatchOptions() WatchOptions {
	return WatchOptions{DefaultPollInterval, DefaultDebounce, DefaultMaxWatchers, DefaultReloadTimeout, true}
}

type watcher struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	opts       WatchOptions
	filePath   string
	lastMode   os.FileMode
	watching   atomic.Bool
	watchers   map[int64]chan string
	watcherID  int64
	observed   string
	handled    string
	timeoutKey string
	observedAt time.Time
	read       func(context.Context, string, fileSettings) (*fileSnapshot, error)
}

func normalizeWatchOptions(opts WatchOptions) WatchOptions {
	if opts.PollInterval < MinPollInterval {
		opts.PollInterval = MinPollInterval
	}
	if opts.Debounce < 0 {
		opts.Debounce = 0
	}
	if opts.MaxWatchers <= 0 {
		opts.MaxWatchers = DefaultMaxWatchers
	}
	if opts.ReloadTimeout <= 0 {
		opts.ReloadTimeout = DefaultReloadTimeout
	}
	return opts
}

func (c *Config) AutoUpdate() { c.AutoUpdateWithOptions(DefaultWatchOptions()) }
func (c *Config) AutoUpdateWithOptions(opts WatchOptions) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.startWatcherLocked(opts)
}
func (c *Config) startWatcherLocked(opts WatchOptions) {
	if c.configFilePath == "" {
		return
	}
	if c.watcher != nil {
		if c.watcher.filePath == c.configFilePath && c.watcher.watching.Load() {
			return
		}
		c.watcher.stop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &watcher{ctx: ctx, cancel: cancel, opts: normalizeWatchOptions(opts), filePath: c.configFilePath, watchers: make(map[int64]chan string), read: readConfigFile}
	if info, err := os.Stat(w.filePath); err == nil {
		w.lastMode = info.Mode()
	}
	w.watching.Store(true)
	c.watcher = w
	go w.watchLoop(c)
}

func (c *Config) StopAutoUpdate() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.watcher != nil {
		c.watcher.stop()
		c.watcher = nil
	}
}
func (c *Config) Watch() <-chan string { return c.WatchWithOptions(DefaultWatchOptions()) }
func (c *Config) WatchWithOptions(opts WatchOptions) <-chan string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.startWatcherLocked(opts)
	if c.watcher == nil {
		return closedWatchChannel()
	}
	return c.watcher.subscribe()
}
func (c *Config) WatchFile(path string, formatHint ...string) error {
	if len(formatHint) > 1 {
		return ErrFileFormat
	}
	if len(formatHint) == 1 {
		if err := c.SetFileFormat(formatHint[0]); err != nil {
			return err
		}
	}
	c.mutex.RLock()
	opts := DefaultWatchOptions()
	if c.watcher != nil {
		opts = c.watcher.opts
	}
	c.mutex.RUnlock()
	// A failed load leaves the existing watcher and valid state intact.
	if err := c.LoadFile(path); err != nil {
		return err
	}
	c.AutoUpdateWithOptions(opts)
	return nil
}
func (c *Config) IsWatching() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.watcher != nil && c.watcher.watching.Load()
}
func (c *Config) WatcherCount() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	if c.watcher == nil {
		return 0
	}
	c.watcher.mu.Lock()
	defer c.watcher.mu.Unlock()
	return len(c.watcher.watchers)
}
func (w *watcher) watchLoop(c *Config) {
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

// One loop owns observations and debounce state. I/O runs outside the Config lock;
// publication checks context, watcher identity and file generation under that lock.
func (w *watcher) checkAndReload(c *Config) {
	if w.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(w.ctx, w.opts.ReloadTimeout)
	defer cancel()
	c.mutex.RLock()
	opts := c.fileSettingsLocked()
	generation := c.fileGeneration
	c.mutex.RUnlock()
	snapshot, readErr := w.read(ctx, w.filePath, opts)
	key := ""
	event := ""
	if readErr != nil {
		key = "error:" + readErr.Error()
		switch {
		case errors.Is(readErr, context.DeadlineExceeded):
			event = EventReloadTimeout
		case errors.Is(readErr, ErrConfigNotFound):
			event = EventFileDeleted
		default:
			event = EventReloadError + ":" + readErr.Error()
		}
	} else {
		key = fmt.Sprintf("%x:%o", snapshot.digest, snapshot.info.Mode())
		if w.opts.VerifyPermissions && w.lastMode != 0 && (snapshot.info.Mode()&0077) != (w.lastMode&0077) {
			event = EventPermissionsChanged
		}
	}
	if key != w.observed {
		w.observed, w.observedAt = key, time.Now()
	}
	if key == w.handled || time.Since(w.observedAt) < w.opts.Debounce {
		return
	}
	if w.ctx.Err() != nil {
		return
	}
	if event != "" {
		if event == EventReloadTimeout {
			w.notifyTimeout(key)
		} else {
			w.notifyWatchers(event)
			w.handled = key
			if event == EventPermissionsChanged {
				// Reported once; the next change applies under the new mode
				w.lastMode = snapshot.info.Mode()
			}
		}
		return
	}
	root, comments, err := parseFile(snapshot)
	if err != nil {
		w.notifyWatchers(EventReloadError + ":" + err.Error())
		w.handled = key
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.watcher != w || c.configFilePath != w.filePath || c.fileGeneration != generation || w.ctx.Err() != nil {
		return
	}
	if ctx.Err() != nil {
		w.notifyTimeout(key)
		return
	}
	old := make(map[string]any, len(c.items))
	for path, item := range c.items {
		old[path] = item.currentValue
	}
	if err := c.applyFileLocked(root, comments, w.filePath, ctx.Err); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			w.notifyTimeout(key)
		} else {
			w.notifyWatchers(EventReloadError + ":" + err.Error())
			w.handled = key
		}
		return
	}
	w.lastMode, w.handled = snapshot.info.Mode(), key
	w.timeoutKey = ""
	var changed []string
	for path, item := range c.items {
		if !reflect.DeepEqual(old[path], item.currentValue) {
			changed = append(changed, path)
		}
	}
	slices.Sort(changed)
	for _, path := range changed {
		w.notifyWatchers(path)
	}
}

// A timeout is retryable even when the file contents have not changed. Suppress
// repeated notifications for the same observation without suppressing its retry.
func (w *watcher) notifyTimeout(key string) {
	if w.timeoutKey != key {
		w.notifyWatchers(EventReloadTimeout)
		w.timeoutKey = key
	}
}

func closedWatchChannel() <-chan string { ch := make(chan string); close(ch); return ch }
func (w *watcher) subscribe() <-chan string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx.Err() != nil || len(w.watchers) >= w.opts.MaxWatchers {
		return closedWatchChannel()
	}
	ch := make(chan string, WatchChannelBuffer)
	w.watcherID++
	w.watchers[w.watcherID] = ch
	return ch
}
func (w *watcher) notifyWatchers(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ctx.Err() != nil {
		return
	}
	for _, ch := range w.watchers {
		select {
		case ch <- path:
		default:
		}
	}
}
func (w *watcher) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cancel()
	w.watching.Store(false)
	for id, ch := range w.watchers {
		close(ch)
		delete(w.watchers, id)
	}
}
func (c *Config) snapshot() map[string]any {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	out := make(map[string]any, len(c.items))
	for path, item := range c.items {
		out[path] = cloneOwned(item.currentValue)
	}
	return out
}

# Live Reconfiguration

The config package supports automatic configuration reloading when files change, enabling zero-downtime reconfiguration.

## Basic File Watching

### Enable Auto-Update

```go
cfg, _ := config.NewBuilder().
    WithDefaults(&Config{}).
    WithFile("config.toml").
    Build()

// Enable automatic reloading
cfg.AutoUpdate()

// Your application continues running
// Config reloads automatically when file changes

// Stop watching when done
defer cfg.StopAutoUpdate()
```

### Watch for Changes

```go
// Get notified of configuration changes
changes := cfg.Watch()

go func() {
    for path := range changes {
        log.Printf("Configuration changed: %s", path)
        
        // React to specific changes
        switch path {
        case "server.port":
            // Restart server with new port
            restartServer()
        case "log.level":
            // Update log level
            updateLogLevel()
        }
    }
}()
```

## Watch Options

### Custom Watch Configuration

```go
opts := config.WatchOptions{
    PollInterval:      500 * time.Millisecond,  // Check every 500ms
    Debounce:          200 * time.Millisecond,  // Wait 200ms after changes
    MaxWatchers:       50,                      // Limit concurrent watchers
    ReloadTimeout:     10 * time.Second,        // Timeout for reload
    VerifyPermissions: true,                    // Security check
}

cfg.AutoUpdateWithOptions(opts)
```

### Watch Without Auto-Update

```go
// Just watch, don't auto-reload
changes := cfg.WatchWithOptions(config.WatchOptions{
    PollInterval: time.Second,
})

// Manually reload when desired
go func() {
    for range changes {
        if shouldReload() {
            cfg.LoadFile("config.toml")
        }
    }
}()
```

## Change Detection

### Value Changes

The watcher detects and notifies about:
- New values added
- Existing values modified  
- Values removed
- Type changes

```go
changes := cfg.Watch()

for path := range changes {
    newVal, exists := cfg.Get(path)
    if !exists {
        log.Printf("Removed: %s", path)
        continue
    }
    
    sources := cfg.GetSources(path)
    fileVal, hasFile := sources[config.SourceFile]
    
    log.Printf("Changed: %s = %v (from file: %v)", 
        path, newVal, hasFile)
}
```

### Special Notifications

```go
changes := cfg.Watch()

for notification := range changes {
    switch notification {
    case "file_deleted":
        log.Warn("Config file was deleted")
        
    case "permissions_changed":
        log.Error("Config file permissions changed - potential security issue")
        
    case "reload_timeout":
        log.Error("Config reload timed out")
        
    default:
        if strings.HasPrefix(notification, "reload_error:") {
            log.Error("Reload error:", notification)
        } else {
            // Normal path change
            handleConfigChange(notification)
        }
    }
}
```

## Debouncing

Rapid file changes are automatically debounced:

```go
// Multiple rapid saves to config.toml
// Only triggers one reload after debounce period

opts := config.WatchOptions{
    PollInterval: 100 * time.Millisecond,
    Debounce:     500 * time.Millisecond,  // Wait 500ms
}

cfg.AutoUpdateWithOptions(opts)
```

## Permission Monitoring

```go
opts := config.WatchOptions{
    VerifyPermissions: true,  // Enabled by default
}

cfg.AutoUpdateWithOptions(opts)

// Detects if file becomes world-writable
changes := cfg.Watch()
for change := range changes {
    if change == "permissions_changed" {
        // File permissions changed
        // Possible security breach
        alert("Config file permissions modified!")
    }
}
```

## Pattern: Reconfiguration

```go
type Server struct {
    cfg      *config.Config
    listener net.Listener
    mu       sync.RWMutex
}

func (s *Server) watchConfig() {
    changes := s.cfg.Watch()
    
    for path := range changes {
        switch {
        case strings.HasPrefix(path, "server."):
            s.scheduleRestart()
            
        case path == "log.level":
            s.updateLogLevel()
            
        case strings.HasPrefix(path, "feature."):
            s.reloadFeatures()
        }
    }
}

func (s *Server) scheduleRestart() {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Graceful restart logic
    log.Info("Scheduling server restart for config changes")
    // ... drain connections, restart listener ...
}
```

## Pattern: Feature Flags

```go
type FeatureFlags struct {
    cfg *config.Config
    mu  sync.RWMutex
}

func (ff *FeatureFlags) Watch() {
    changes := ff.cfg.Watch()
    
    for path := range changes {
        if strings.HasPrefix(path, "features.") {
            feature := strings.TrimPrefix(path, "features.")
            enabled, _ := ff.cfg.Get(path)
            
            log.Printf("Feature %s: %v", feature, enabled)
            ff.notifyFeatureChange(feature, enabled.(bool))
        }
    }
}

func (ff *FeatureFlags) IsEnabled(feature string) bool {
    ff.mu.RLock()
    defer ff.mu.RUnlock()
    
    val, exists := ff.cfg.Get("features." + feature)
    return exists && val.(bool)
}
```

## Pattern: Multi-Stage Reload

```go
func watchConfigWithValidation(cfg *config.Config) {
    changes := cfg.Watch()
    
    for range changes {
        // Stage 1: Snapshot current config
        backup := cfg.Clone()
        
        // Stage 2: Validate new configuration
        if err := validateNewConfig(cfg); err != nil {
            log.Error("Invalid configuration:", err)
            continue
        }
        
        // Stage 3: Apply changes
        if err := applyConfigChanges(cfg, backup); err != nil {
            log.Error("Failed to apply changes:", err)
            // Could restore from backup here
            continue
        }
        
        log.Info("Configuration successfully reloaded")
    }
}
```

## Monitoring

### Watch Status

```go
// Check if watching is active
if cfg.IsWatching() {
    log.Printf("Auto-update is enabled")
    log.Printf("Active watchers: %d", cfg.WatcherCount())
}
```

### Resource Management

```go
// Limit watchers to prevent resource exhaustion
opts := config.WatchOptions{
    MaxWatchers: 10,  // Max 10 concurrent watch channels
}

// Watchers beyond limit receive closed channels
cfg.AutoUpdateWithOptions(opts)
```

## Best Practices

1. **Always Stop Watching**: Use `defer cfg.StopAutoUpdate()` to clean up
2. **Handle All Notifications**: Check for special error notifications
3. **Validate After Reload**: Ensure new config is valid before applying
4. **Use Debouncing**: Prevent reload storms from rapid edits
5. **Monitor Permissions**: Enable permission verification for security
6. **Graceful Updates**: Plan how your app handles config changes
7. **Log Changes**: Audit configuration modifications

## Limitations

- File watching uses polling (not inotify/kqueue)
- No support for watching multiple files
- Changes only detected for registered paths
- Reloads entire file (no partial updates)

## Common Issues

### Changes Not Detected

```go
// Ensure path is registered before watching
cfg.Register("new.value", "default")

// Now changes to new.value will be detected
```

### Rapid Reloads

```go
// Increase debounce to prevent rapid reloads
opts := config.WatchOptions{
    Debounce: 2 * time.Second,  // Wait 2s after changes stop
}
```

### Memory Leaks

```go
// Always stop watching to prevent goroutine leaks
watcher := cfg.Watch()

// Use context for cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go func() {
    for {
        select {
        case change := <-watcher:
            handleChange(change)
        case <-ctx.Done():
            return
        }
    }
}()
```

## See Also

- [File Configuration](file.md) - File format and loading
- [Access Patterns](access.md) - Reacting to changed values
- [Builder Pattern](builder.md) - Setting up watching with builder
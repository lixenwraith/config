# Config Package Architecture

## Overview

The `lixenwraith/config` package provides thread-safe configuration management with support for multiple sources, type-safe struct population, and live reconfiguration.

## Logical Architecture

```
┌──────────────────────────────────────────────────────────┐
│                       User API Layer                     │
├────────────────┬────────────────┬────────────────────────┤
│    Builder     │   Type-Safe    │    Dynamic Access      │
│    Pattern     │   Struct API   │     Key-Value API      │
├────────────────┴────────────────┴────────────────────────┤
│                    Core Config Engine                    │
│  • Path Registration  • Source Merging  • Thread Safety  │
├──────────────────────────────────────────────────────────┤
│                     Source Loaders                       │
│     File │ Environment │ CLI Arguments │ Defaults        │
├──────────────────────────────────────────────────────────┤
│                   Supporting Systems                     │
│  Validation │ Type Decode │ File Watch │ Error Handling  │
└──────────────────────────────────────────────────────────┘
```

## Component Interactions

```
Builder Flow:
    NewBuilder() 
        ↓
    Configure (WithTarget, WithFile, WithEnvPrefix)
        ↓
    Build() → Register Paths → Load Sources → Merge Values
        ↓
    Config Instance
        ↓
    AsStruct() / Get() / Watch()

Value Resolution:
    Path Request
        ↓
    Check Registration
        ↓
    Search Sources (CLI → Env → File → Default)
        ↓
    Type Conversion
        ↓
    Return Value

Live Reload:
    File Change Detected
        ↓
    Debounce Timer
        ↓
    Reload File
        ↓
    Merge with Sources
        ↓
    Notify Watchers
```

## File Organization

### Core (`config.go`)
- **Config struct**: Thread-safe state management with atomic versioning
- **Initialization**: `New()`, `NewWithOptions()`
- **State Management**: `Get()`, `Set()`, `GetSource()`, `SetSource()`
- **Precedence Control**: `SetPrecedence()`, `GetPrecedence()`, `computeValue()`
- **Cache Management**: `AsStruct()`, `populateStruct()`, `invalidateCache()`
- **Source Operations**: `Reset()`, `ResetSource()`, `GetSources()`

### Registration (`register.go`)
- **Path Registration**: `Register()`, `RegisterWithEnv()`, `RegisterRequired()`, `Unregister()`
- **Struct Registration**: `RegisterStruct()`, `RegisterStructWithTags()`, `registerFields()`
- **Discovery**: `GetRegisteredPaths()`, `GetRegisteredPathsWithDefaults()`
- **Decoding Bridge**: `Scan()`, `ScanSource()` - delegates to decode.go

### Builder Pattern (`builder.go`)
- **Fluent API**: `NewBuilder()`, `Build()`, `MustBuild()`
- **Configuration**: `WithTarget()`, `WithDefaults()`, `WithFile()`, `WithEnvPrefix()`
- **Discovery Integration**: `WithFileDiscovery()`
- **Validation**: `WithValidator()`, `WithTypedValidator()`
- **Security**: `WithSecurityOptions()`

### Source Loading (`loader.go`)
- **Multi-Source Loading**: `Load()`, `LoadWithOptions()`
- **Individual Sources**: `LoadFile()`, `LoadEnv()`, `LoadCLI()`
- **Persistence**: `Save()`, `SaveSource()`, `atomicWriteFile()`
- **Environment Mapping**: `DiscoverEnv()`, `ExportEnv()`, `defaultEnvTransform()`
- **Parsing**: `parseArgs()`, `parseValue()`, `detectFileFormat()`
- **Security Checks**: Path traversal, file ownership, size limits

### Type System (`decode.go`)
- **Unified Decoding**: `unmarshal()` - single authoritative decoder
- **Hook Composition**: `getDecodeHook()`, `customDecodeHook()`
- **Type Converters**: `jsonNumberHookFunc()`, `stringToNetIPHookFunc()`, `stringToURLHookFunc()`
- **Navigation**: `navigateToPath()`, `normalizeMap()`

### File Watching (`watch.go`)
- **Auto-Reload**: `AutoUpdate()`, `AutoUpdateWithOptions()`, `StopAutoUpdate()`
- **Subscriptions**: `Watch()`, `WatchWithOptions()`, `WatchFile()`
- **Watcher State**: `watcher` struct with debouncing, polling, version tracking
- **Change Detection**: `checkAndReload()`, `performReload()`, `notifyWatchers()`
- **Resource Management**: Max watcher limits, graceful shutdown

### Convenience API (`convenience.go`)
- **Quick Setup**: `Quick()`, `QuickCustom()`, `MustQuick()`, `QuickTyped()`
- **Flag Integration**: `GenerateFlags()`, `BindFlags()`
- **Type-Safe Access**: `GetTyped()`, `GetTypedWithDefault()`, `ScanTyped()`
- **Utilities**: `Validate()`, `Debug()`, `Dump()`, `Clone()`

### File Discovery (`discovery.go`)
- **Options**: `FileDiscoveryOptions` struct with search strategies
- **XDG Compliance**: `getXDGConfigPaths()` for standard config locations
- **Defaults**: `DefaultDiscoveryOptions()` with sensible patterns

### Validation Library (`validator.go`)
- **Network**: `Port()`, `IPAddress()`, `IPv4Address()`, `IPv6Address()`
- **Numeric**: `Positive()`, `NonNegative()`, `Range()`
- **String**: `NonEmpty()`, `Pattern()`, `OneOf()`, `URLPath()`

### Error System (`error.go`)
- **Categories**: Sentinel errors for `errors.Is()` checking
- **Wrapping**: `wrapError()` maintains dual error chains

### Internal Utilities (`helper.go`)
- **Map Operations**: `flattenMap()`, `setNestedValue()`
- **Validation**: `isValidKeySegment()` for path validation

### Constants (`constant.go`)
- **Shared Constants**: Defines shared constants for timing, formats, limits, file watcher and discovery.

## Data Flow Patterns

### Configuration Loading
1. **Registration Phase**: Paths registered with types/defaults
2. **Source Collection**: Each source populates its layer
3. **Merge Phase**: Precedence determines final values
4. **Population Phase**: Struct populated via reflection

### Thread Safety Model
- **Read Operations**: Multiple concurrent readers via RLock
- **Write Operations**: Exclusive access via Lock
- **Atomic Updates**: Prepare → Lock → Swap → Unlock pattern
- **Version Tracking**: Atomic counter for cache invalidation

### Error Propagation
- **Wrapped Errors**: Category + specific error detail
- **Early Return**: Builder accumulates first error
- **Panic Mode**: MustBuild for fail-fast scenarios

## Extension Points

### Custom Types
Implement decode hooks in `customDecodeHook()`:
```go
func(f reflect.Type, t reflect.Type, data any) (any, error)
```

### Source Transformation
Environment variable mapping via `WithEnvTransform()`:
```go
func(path string) string // Return env var name
```

### Validation Layers
- Pre-decode: `WithValidator(func(*Config) error)`
- Post-decode: `WithTypedValidator(func(*YourType) error)`

### File Discovery
Search strategy via `WithFileDiscovery()`:
- CLI flag check → Env var → XDG paths → Current dir
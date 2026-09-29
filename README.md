# config

Thread-safe configuration from TOML files, environment variables, command-line
arguments and registered defaults. Requires Go 1.27.1. The only runtime dependency
is [lixenwraith/toml](https://github.com/lixenwraith/toml).

```go
package main

import (
    "log"
    "time"
    "github.com/lixenwraith/config"
)

type Settings struct {
    Port uint16 `toml:"port"`
    Timeout time.Duration `toml:"timeout"`
}

func main() {
    initial := Settings{Port: 8080, Timeout: 5 * time.Second}
    cfg, err := config.NewBuilder().
        WithTarget(&initial).
        WithFile("config.toml").
        WithEnvPrefix("APP_").
        WithArgs([]string{"--port=9090"}).
        Build()
    if err != nil { log.Fatal(err) }
    latest, err := cfg.AsStruct()
    if err != nil { log.Fatal(err) }
    log.Printf("%+v", latest.(*Settings))
}
```

```toml
# Service settings
timeout = "10s"
port = 8080
```

Precedence is CLI > environment > file > default. Register paths with `Register`
or structs with `RegisterStruct`; defaults define the types used to validate new
values. `LoadFile`, `LoadEnv` and `LoadCLI` replace their complete source. A failed
source load keeps its previous values. Removing an environment variable, CLI flag
or file key removes the old override on the next load.

`Get` returns a detached raw source value: environment and CLI values remain
strings, TOML integers are int64. `GetTyped[T]`, `Scan`, `ScanSource`, `ScanTyped[T]`
and `ScanMap` provide checked conversions. Errors leave destinations unchanged;
missing struct fields retain their existing values. `AsStruct` returns a detached
snapshot on every call. Build fills `WithTarget` once; subsequent reloads never
mutate that caller-owned object.

## Migration

- Only TOML files and `toml` tags are supported. JSON/YAML parsers, format constants
  and alternate tag modes are removed. Existing format/tag option methods accept
  only `toml` (`auto` remains an alias for TOML for file options).
- Numeric conversion rejects overflow, negative unsigned inputs, fractional
  integer conversion, non-finite floats and integer-to-float precision loss.
  Float64-to-float32 rounding within range remains supported. Full uint64 is
  supported in memory; saving a value above MaxInt64 fails without changing the
  destination file. Use an application-level string representation if needed.
- Strings can represent decimal numbers, booleans, comma-separated slices/arrays,
  durations, RFC3339 timestamps, IP addresses, CIDR networks and URLs. Boolean to
  string conversion uses `true`/`false`. There are no user decode hooks or automatic
  `encoding.TextUnmarshaler` calls.
- Setters and source loads reject values incompatible with registered defaults
  immediately. Check their errors. Untyped nil defaults leave the value type open;
  typed nil pointers retain their declared type. Unsupported values, non-finite
  floats and cyclic/deep graphs are rejected. Configuration value depth is 128.
- All public values, defaults, options and clones own their mutable containers.
  Existing references returned from `AsStruct` are snapshots, not live objects.
- Saves standardize formatting and retain loaded comments as a preamble. Comment
  positions are not preserved. Existing file permissions are retained; new files
  use 0600. Only registered paths are saved, so unknown file keys are omitted.
- Paths use dot-separated ASCII key segments. Numeric-only segments and overlapping
  registrations such as `server` plus `server.port` are rejected. Embedded fields
  are not promoted. Recursive struct schemas are unsupported.

## Watching

```go
changes := cfg.WatchWithOptions(config.DefaultWatchOptions())
defer cfg.StopAutoUpdate()
for event := range changes {
    // Events are hints; always read the latest complete state.
    log.Print(event)
    latest, err := cfg.AsStruct()
    if err != nil { log.Print(err); continue }
    _ = latest.(*Settings)
}
```

The watcher fingerprints file contents, waits for a quiet debounce interval, then
validates and publishes. Invalid edits, timeouts, deletion and blocked permission
changes retain the last valid state. Cancelled, expired or superseded reloads
cannot publish. Stop closes subscriptions synchronously; a new Watch call can
restart watching. Notifications are buffered and best effort.

## Documentation

- [Quick start](doc/quick-start.md), [builder](doc/builder.md), [access](doc/access.md)
- [Files and saving](doc/file.md), [environment](doc/env.md), [CLI](doc/cli.md)
- [Live reconfiguration](doc/reconfiguration.md), [validation](doc/validator.md)
- [Architecture](doc/architecture.md), [audit evidence](doc/audit.md)

Run `go test -race ./...` and `go vet ./...`. CI also executes the 386 suite,
cross-compiles for Windows and runs bounded numeric conversion fuzzing.

BSD-3-Clause; see LICENSE.

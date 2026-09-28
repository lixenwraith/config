# Builder

`NewBuilder` collects defaults, an optional typed target, file, environment and CLI
settings. Build registers defaults, loads each configured source, checks required
paths, runs Config validators and then typed validators. Missing files are nonfatal
only when other loading and validation succeeds.

| Option | Purpose |
| --- | --- |
| `WithDefaults(value)` | Explicit struct defaults |
| `WithTarget(&value)` | Target type and fallback defaults; filled once by Build |
| `WithPrefix("section")` | Prefix registered paths and the typed snapshot section |
| `WithFile(path)` | TOML file; missing path can be watched for later creation |
| `WithFileDiscovery(options)` | Search CLI, environment and configured directories |
| `WithEnvPrefix`, `WithEnvTransform`, `WithEnvWhitelist` | Environment mapping |
| `WithArgs(args)` | Explicit CLI input; nil disables it |
| `WithSources(...)` | Source precedence; first wins |
| `WithSecurityOptions(options)` | File traversal, owner and size checks |
| `WithValidator(fn)` | Build-time validation using Config accessors |
| `WithTypedValidator(func(*T) error)` | Build-time validation of the target snapshot |

Only TOML tags and file format are supported. WithTagName accepts `toml`;
WithFileFormat accepts `toml` or `auto`. JSON/YAML modes are removed.

Use a builder from one goroutine during initialization. Runtime updates go through
Config methods. `AsStruct` returns independent snapshots and never mutates the
original target after Build. Validators run during Build, not on every later Set
or reload; later changes are always checked against registered value types.

Explicit defaults must use the same struct type as the target. `WithDefaults`
takes precedence over the initial target values. A typed nil validator is ignored;
signatures must match the target pointer and return error.

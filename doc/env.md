# Environment variables

`LoadEnv("APP_")` maps `server.port` to `APP_SERVER_PORT`. It replaces the complete
environment source: variables removed since the previous load lose their overrides.
Set `LoadOptions.EnvTransform` for custom names and `EnvWhitelist` to limit paths.
Options are copied. Custom transforms run outside Config locks and may inspect Config.

Environment values are stored as strings and validated against registered types
before any part of the replacement is published. Numbers use decimal text; durations
use Go duration syntax; timestamps use RFC3339Nano-compatible text. Slices and arrays
use comma-separated elements without an escaping convention. Empty text gives an
empty slice. Values larger than MaxValueSize are rejected.

`RegisterWithEnv` and a struct field's `env` tag load an explicitly named variable
during registration. Later `LoadEnv` calls use the configured transform/whitelist,
so explicit registration-time mappings are not automatically repeated on reload.

`DiscoverEnv(prefix)` returns registered paths with present variable names.
`ExportEnv(prefix)` returns formatted effective values that differ from defaults;
it safely handles containers. It is diagnostic output, not a general lossless
serialization format for maps, slices or strings containing delimiters. Use Save
for round-trip persistence.

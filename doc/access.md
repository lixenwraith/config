# Access and conversion

| API | Behavior |
| --- | --- |
| `Get(path)` | Detached raw effective value and registration flag |
| `GetSource(path, source)` | Detached source value; Default returns the registered default |
| `GetSources(path)` | Detached map of explicit source overrides |
| `GetTyped[T](cfg, path)` | Checked conversion to T |
| `GetTypedWithDefault[T]` | Registers a default only when the path is absent; existing conversion errors are returned |
| `Scan(&target, optionalPath)` | Transactional decoding of current values |
| `ScanSource(source, &target, optionalPath)` | Decodes only the selected source |
| `ScanTyped[T]` | Allocates and populates a typed destination |
| `ScanMap(values, &target)` | Uses the same decoder without registration |
| `AsStruct()` | Independent snapshot of the configured target type |

Raw environment/CLI values remain strings. TOML integer literals are int64.
Defaults retain their declared types. Use typed access instead of assuming a raw
source type. Matching uses `toml` tags, then case-sensitive Go field names. Tags
with an empty name use the field name; `-` skips a field. Unknown input fields and
unexported destination fields are ignored. Maps and slices replace existing
containers; fixed arrays require exact lengths. Missing struct fields remain as
provided by the caller. A present nil value clears its destination.

Conversions support signed/unsigned numbers, floats, booleans, strings, slices,
arrays, string-keyed maps, structs and pointers. Durations, time.Time, net.IP,
net.IPNet and url.URL are atomic configuration values, including pointer forms.
They accept their native types or documented text representations. Comma-separated
strings can populate typed numeric slices as well as string slices.

No truncating integer conversions or integer-to-float precision loss is allowed.
Float narrowing may round within range. NaN/Inf and unsupported object graphs are
rejected. All decoding failures leave the destination unchanged. There are no
custom decode hooks or automatic text-unmarshal interfaces.

A Config owns accepted values. Modifying Get results, defaults after registration,
clones or AsStruct snapshots cannot modify stored state. Do not mutate arguments
concurrently while a call is reading them. Builder setup itself is not concurrent.

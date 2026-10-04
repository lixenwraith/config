# TOML files and saves

`LoadFile(path)` parses TOML regardless of the usual TOML/conf/config extension;
JSON/YAML extensions are rejected. `SetFileFormat` and `WithFileFormat` accept
`toml` and the compatibility alias `auto`; neither enables format detection.
Only registered paths are applied. Registered map fields accept whole tables.
Replacing a registered parent table with a scalar is an error. A missing key
removes that source override, exposing the next source or registered default.

Loads parse and validate before publication. Syntax errors, incompatible values,
overflow and read failures leave prior values, comments and the loaded path intact.
Readers must be regular files, checked on the opened descriptor. The open does not
block, so a FIFO swapped in after the path's check fails at once, and a file whose
reads would block fails its read. Security options check ownership of the opened
file on Unix and enforce size limits both before and during reading. Ownership
checks fail explicitly on unsupported platforms. PreventPathTraversal checks the
path text only: it rejects relative paths escaping through `..`, not symlinks or
absolute paths. A zero MaxFileSize means no configured byte limit.

`Save(path)` writes effective values; `SaveSource(path, source)` writes one source.
Default selects registered defaults. Both use a temporary file in the destination
directory, sync it, and rename it over the destination. They preserve an existing
file's permission bits and use 0600 for a new file. They do not promise portable
power-loss durability of the directory entry or platform-independent replacement
of open files. Encoding failures occur before the destination is changed.

Loaded comment text, including inline comments, is preserved as a preamble. Hashes
inside strings are not comments. Formatting and comment positions are standardized,
and unknown/unregistered keys are omitted. Saves do not merge concurrent edits made
outside Config after the last successful load. Save does not switch the watched path.

Durations and atomic time/network values are saved as strings and decoded back to
their registered types. time.Time uses RFC3339Nano; native TOML datetime syntax is
unsupported. Integers in files have signed 64-bit range. Full uint64 values can be
held in memory but cannot be saved above MaxInt64. Unsupported TOML syntax and
numeric key restrictions follow the pinned [toml package](https://github.com/lixenwraith/toml).

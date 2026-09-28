# Command-line arguments

`LoadCLI` accepts `--server.port=9000`, `--server.port 9000`, and bare boolean flags
such as `--debug`. Arguments after `--` are positional and are not parsed. Other
positional arguments are ignored. There is no short-flag parser; use Go's flag package
when that interface is needed. Paths must obey registration's dot-separated key syntax.

Values remain strings and are validated against registered types before publication.
Each successful call replaces the complete CLI source, including an empty argument
list. Omitted flags lose their previous overrides. Unknown paths are recorded in
`UnknownCLIKeys` in sorted order and are not registered. The returned list is a copy.

`GenerateFlags` creates a flag.FlagSet from defaults. `BindFlags` atomically replaces
the CLI source with explicitly visited flags, rejects unregistered flags, and uses
the same checked conversions. Repeated flags resolve to the last value. Invalid
values leave the prior CLI source and unknown-key list intact.

The Builder reads os.Args by default. Use `WithArgs(nil)` to disable CLI loading for
that build or supply an explicit argument slice for deterministic initialization.

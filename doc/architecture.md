# Architecture

Registered items hold a default, source overrides and an effective value selected
by precedence. A Config RWMutex protects these maps and runtime options. Values
are copied on admission and never mutated internally afterward; replacement removes
or swaps references. Public reads return copies. Nested snapshots can therefore be
assembled under a read lock and decoded after releasing it.

The decoder handles the limited configuration object model: TOML tags, standard
containers and atomic duration/time/network values. It delegates numeric conversion
to lixenwraith/toml and uses explicit decimal parsing for environment/CLI strings.
No mapstructure or external file parser is used in the runtime dependency graph.

Typed cache metadata uses a second mutex, always acquired after the Config read
lock. Version changes invalidate its immutable decoded snapshot. Every AsStruct call
returns a detached copy, so publication cannot race with existing readers. Build
writes the supplied target once. Clone copies configuration state and options but
does not copy active watchers or subscriptions.

File reads and parsing precede source publication. All candidate values are checked
against the current schema, then the complete source is replaced under one lock.
The watcher additionally verifies cancellation, timeout and generation at commit.
One worker owns fingerprint/debounce state. Subscriber send/close operations share a
mutex; stop does not wait for that worker while holding the Config mutex.

Save normalizes atomic application values to TOML strings, marshals before touching
the target file, then writes/syncs/renames a temporary file. Comment text is captured
by lexer tokens and emitted as a preamble. The codec owns syntax, while Config owns
source precedence, snapshot isolation and live reload publication.

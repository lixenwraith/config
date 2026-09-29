# Config audit and TOML migration

This branch updates only lixenwraith/config. It uses Go 1.27.1 and the user's merged
TOML main commit `b59d068ff6a48b86365a65e1121343f672f77a67`, pinned as
`v0.0.0-20260928180512-b59d068ff6a4` because no version tag existed at integration.
Logwisp adoption and other TOML consumers are separate follow-up work.

## Confirmed defects and fixes

| Finding | Result | Regression evidence |
| --- | --- | --- |
| Fractions/overflow could pass through weak numeric conversion | Checked TOML conversion and explicit destination-width parsing | `TestCheckedConversionsAndRollback`, `FuzzConfigNumericConversions` |
| Options, input containers and returned values could alias caller state | Owned options/values and detached reads/clones | `TestOptionsAreOwned`, `TestOwnedValuesAndSnapshots` |
| Updating the cached target changed old AsStruct results | Immutable cached result and a fresh snapshot per call | `TestOwnedValuesAndSnapshots` |
| ExportEnv compared uncomparable slices/maps directly | Deep equality, with callbacks outside Config locks | `TestExportEnvContainerValues` |
| Invalid sources could be published before decoding failed | Whole-source validation before mutation | `TestSourceReplacementIsTransactional` |
| Removed env/CLI values survived subsequent loads | Successful loads replace their complete source | `TestSourceReplacementIsTransactional` |
| A timed-out reload kept running and could publish after stop | Context, watcher identity, path and generation checked at commit | `TestReloadCannotPublishAfterStopTimeoutOrNewLoad` |
| A timeout could suppress retries of unchanged content | Retry timeouts while deduplicating their notifications | `TestWatcherRetriesTimedOutUnchangedContent` |
| Debounce did not cover errors from partial in-place writes | Errors and successful updates wait for a quiet interval | `TestWatcherDebouncesErrorsAndRecovers` |
| Metadata-only polling missed some replacements/edits | Content fingerprints on every poll; deletion/recreation recovery | `TestWatcherSameMetadataReplacementAndRecreation` |
| Source saving lost comments and widened permissions | Comment preamble, existing modes retained, new files 0600 | `TestSaveCommentsPermissionsAndFailure` |
| Atomic types were inconsistently registered/serialized | Exact type matching for duration/time/IP/CIDR/URL and pointer forms | `TestAtomicValuesAndFileRoundTrip` |
| A missing file could hide a joined invalid CLI/environment error | Missing-file fallback only when other sources succeed | `TestBuilderDoesNotHideInvalidSourceBehindMissingFile` |
| Required registration was a no-op; overlapping/duplicate fields were ambiguous | Required paths validated at Build; staged registration rejects conflicts | `TestRegistrationAndRequiredContracts` |
| Default source selection and narrow target schemas could be inconsistent | Default precedence honored; explicit defaults must match the typed target | `TestRegistrationAndRequiredContracts` |

Reproduction against original main `ccd280b`: fractional-to-integer conversion
silently produced 1 from 1.5; modifying an input slice changed its registered
default; an out-of-range CLI batch returned success; and requesting a refreshed
AsStruct changed an earlier result under its reader. All four were reproduced with
public-API probes; committed regressions cover the corrected behavior.

## Dependency and scope decision

Mapstructure is replaceable for the package's actual object model. The replacement
supports TOML field names, ordinary containers, pointers and the existing standard
atomic types. It does not reproduce mapstructure's broad weak coercions or add a
custom hook API. Numeric correctness is shared with lixenwraith/toml.

`go list -deps ./...` reports only the standard library, lixenwraith/toml and this
module. BurntSushi TOML, mapstructure and YAML are removed from runtime code.
Tests use the standard library; Testify and its transitive dependencies have been
removed from go.mod and go.sum. JSON/YAML files and tags are unsupported, with explicit rejection tests.

No global reflection cache or lock was introduced. Known container sizes are used
for allocation. Stored values are immutable after admission. One watcher worker
owns observation/debounce state, and subscription cleanup has no per-subscriber
goroutine. File I/O and environment callbacks execute outside Config locks.

## Verification and limits

The full unit and race suites and go vet pass with Go 1.27.1. Numeric fuzzing ran
for 10 seconds and passed 234,155 executions, covering full uint64 preservation,
signed boundaries and narrowing. The 386 and Windows test binaries cross-compile
locally. This host cannot execute 386 binaries; CI executes that suite and also
runs race tests, vet, fuzzing and Windows compilation. Final checks are on the PR.

TOML's supported subset is unchanged. Native datetime syntax is not added; time.Time
is represented by a quoted RFC3339Nano string. Full uint64 works in memory but values
above MaxInt64 cannot be saved as TOML integers. Comment text survives as a preamble;
locations and formatting do not. Unknown file keys are not saved. Depth is bounded
at 128 for configuration graphs. Live application validators remain an application
responsibility; Builder validators run only during initialization. Notifications
are best effort, and file timeouts prevent late publication without forcibly
interrupting kernel I/O. See the linked topic guides for migration details.

# Live reconfiguration

`AutoUpdate` starts polling the loaded file. `Watch` starts it if necessary and
subscribes to notifications. Option variants configure the polling interval
(minimum 100ms), debounce, subscriber limit, reload timeout and permission checks.
Options for an already-running watcher stay unchanged; stop/restart to change them.

One worker reads and fingerprints the regular file per poll, including same-size,
same-timestamp edits and atomic replacements. Both successful and erroneous reads
must settle for the debounce interval. Stable errors are reported once per observed
state. Deletion preserves the last valid state, and recreation can reload normally.
This trades one file read per poll for reliable detection of metadata-preserving edits.

Before publishing, the worker checks its context, current watcher identity, watched
path and source generation under the Config lock. Type checks and allocation finish
before the final timeout check. Expired, stopped or superseded operations cannot
publish. ReloadTimeout limits publication eligibility; it cannot forcibly interrupt
a kernel filesystem operation. Reads never hold the Config lock.

Events are changed registered paths, or `file_deleted`, `permissions_changed`,
`reload_timeout`, `reload_error:<detail>`, and `precedence:<path>`. A path event means
the effective value changed; a file value masked by CLI need not notify. Group/world
permission changes are blocked when VerifyPermissions is enabled; correcting the
permissions allows loading again. Default options enable that check.

Notifications are buffered (10 entries) and best effort; a slow subscriber may miss
events and should read the latest full state. MaxWatchers bounds retained channels.
StopAutoUpdate cancels publication and closes channels synchronously without waiting
under the Config lock. There is no goroutine per subscriber or per timed-out reload.
A new Watch call after stopping creates a new channel and worker.

`WatchFile` loads a replacement path before replacing the watcher and preserves its
options. Failure leaves the prior watcher and values active. Loading a different
file directly stops the previous watcher; call Watch to watch the new path.

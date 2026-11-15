`syscalls.json.gz` is the snapshot embedded in the CLI. `provenance.json` has its source and SHA-256, which the binary checks on load. It includes man page summaries under the licenses in NOTICE.md.

`make prepare` downloads a fresh copy from api.syscall.sh. Commit it before a release so `go install` users get the same data as the archives.

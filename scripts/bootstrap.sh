#!/bin/sh
# Prepares a development environment: checks the Go toolchain, tidies modules,
# builds jitr, and registers this repository's own profile. Safe to run
# repeatedly.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

need=$(go list -m -f '{{.GoVersion}}')
have=$(go env GOVERSION | sed 's/^go//')
if [ "$(printf '%s\n%s\n' "$need" "$have" | sort -V | head -n 1)" != "$need" ]; then
	echo "bootstrap: Go $need or later is required, found $have" >&2
	exit 1
fi
echo "bootstrap: Go $have"

go mod tidy
make build
jitr="$root/bin/jitr"

profile="$root/.jitr"
if "$jitr" profile list --json | grep -qF "\"path\":\"$profile\""; then
	echo "bootstrap: profile $profile already registered"
else
	"$jitr" profile register "$profile"
fi

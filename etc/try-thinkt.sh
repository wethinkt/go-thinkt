#!/bin/sh
# Run the embedded viewer against unpublished sibling renderer commits.
set -eu
thinkt_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
nthtml_root="$thinkt_root/../nthtml"
nttypeset_root="$thinkt_root/../nttypeset"
if [ ! -f "$nthtml_root/go.mod" ] || [ ! -f "$nttypeset_root/go.mod" ]; then
    echo "Expected sibling nthtml and nttypeset checkouts next to go-thinkt." >&2
    exit 1
fi
thinkt_workspace=$(mktemp -d "${TMPDIR:-/tmp}/thinkt-viewer.XXXXXX")
trap 'rm -f "$thinkt_workspace/go.work" "$thinkt_workspace/go.work.sum"; rmdir "$thinkt_workspace"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
GOWORK=off go -C "$thinkt_workspace" work init "$thinkt_root" "$nthtml_root" "$nttypeset_root"
# A version-specific workspace override also supplies the unpublished module's
# go.mod during module-graph loading; no repository module files are rewritten.
nthtml_version=$(awk '$1 == "nimbleterminal.dev/nthtml" { print $2; exit }' "$thinkt_root/go.mod")
nttypeset_version=$(awk '$1 == "nimbleterminal.dev/nttypeset" { print $2; exit }' "$nthtml_root/go.mod")
GOWORK="$thinkt_workspace/go.work" go -C "$thinkt_workspace" work edit \
    "-replace=nimbleterminal.dev/nthtml@$nthtml_version=$nthtml_root" \
    "-replace=nimbleterminal.dev/nttypeset@$nttypeset_version=$nttypeset_root"
THINKT_VIEWER="${THINKT_VIEWER:-html}" GOWORK="$thinkt_workspace/go.work" \
    go -C "$thinkt_root" run ./cmd/thinkt "$@"

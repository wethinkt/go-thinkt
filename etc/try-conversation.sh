#!/bin/sh
# Exercise the conversation fixture against both local, possibly unpublished,
# Nimble Terminal modules without changing their go.mod files.
set -eu

thinkt_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
nthtml_root="$thinkt_root/../nthtml"
nttypeset_root="$thinkt_root/../nttypeset"
thinkt_html="$thinkt_root/internal/conversation/testdata/conversation.html"

case "${1:-}" in
    ""|-*) ;;
    *) thinkt_html="$1"; shift ;;
esac
case "$thinkt_html" in
    /*) ;;
    *) thinkt_html="$PWD/$thinkt_html" ;;
esac

if [ ! -f "$nthtml_root/go.mod" ] || [ ! -f "$nttypeset_root/go.mod" ]; then
    echo "Expected sibling nthtml and nttypeset checkouts next to go-thinkt." >&2
    exit 1
fi

thinkt_workspace=$(mktemp -d "${TMPDIR:-/tmp}/thinkt-render.XXXXXX")
trap 'rm -f "$thinkt_workspace/go.work" "$thinkt_workspace/go.work.sum"; rmdir "$thinkt_workspace"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
GOWORK=off go -C "$thinkt_workspace" work init "$nthtml_root" "$nttypeset_root"

if [ "$#" -eq 0 ]; then
    set -- -mode view
fi
GOWORK="$thinkt_workspace/go.work" go -C "$nthtml_root" run ./cmd/nthtml "$@" "$thinkt_html"

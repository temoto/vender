#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
: ${app=""}
cmd="$1"
set -eu

dodir() {
    echo "- $1" >&2
    (
		cd "$base/$1"
		if ls ./*.go >/dev/null 2>&1; then
			do_go
		else
            echo "skip unknown target: $1" >&2
        fi
    )
}

do_go() {
    go get
    go build
    if ls ./*_test.go >/dev/null 2>&1; then
        go test
        go test -bench=.
    fi
}

main() {
	cd $base
    if [[ -n "$app" ]] ; then
        dodir "$app"
    else
        for d in $(find . -type d ! -path '.' ! -path './.*' ! -path './archive*') ; do
            dodir "$d"
        done
    fi
}

main

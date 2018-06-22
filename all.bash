#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
set -eu

main() {
	cd $base

    go get ./...
    go build ./...

    # begin workaround cannot use test profile flag with multiple packages
    for d in $(go list ./... |egrep -v '(vendor|archive)/') ; do
        go test -race -coverprofile=$base/coverage-$(basename $d).txt -covermode=atomic $d
    done
    cat $base/coverage-*.txt >$base/coverage.txt
    rm $base/coverage-*.txt
    # end workaround cannot use test profile flag with multiple packages

    go test -bench=. ./...

    for d in $(find . -type d ! -path '.' ! -path './.*' ! -path './archive*') ; do
        if ! ls $base/$d/*.go >/dev/null 2>&1 ; then
            echo "no build defined for $d" >&2
            exit 1
        fi
    done
}

main

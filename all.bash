#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
: ${build_go:=1}
set -eu

main() {
	cd $base
	rm -f $base/coverage.{info,txt} $base/*.gcda $base/*.gcno

	if [[ "$build_go" != "0" ]] ; then
		export GO111MODULE=on
		GO111MODULE=off go get -v github.com/xlab/c-for-go
		GO111MODULE=off go get -v golang.org/x/tools/cmd/stringer
		go get -v github.com/golang/protobuf/protoc-gen-go
		ensure_golangci_lint
		go generate ./...
		go build ./...
		go test -timeout 3m -race -coverprofile=$base/coverage.txt -covermode=atomic ./...
		go test -bench=. -vet= ./...
		golangci-lint run || true
	fi

	paths=$(find . -type d ! -path '.' ! -path '*/.*' ! -path './script*' ! -path './target*' ! -path './vendor*')
	for d in $paths ; do
		# skip directories without files
		if [[ -z "$(find $base/$d -depth 1 ! -type d ! -path '*/.*')" ]] ; then continue ; fi
		# skip directories with .go files, already built them
		if ls "$base/$d"/*.go >/dev/null 2>&1 ; then continue ; fi
		echo "no build defined for $d" >&2
		exit 1
	done
}

ensure_golangci_lint() {
	golangci-lint -h &>/dev/null && return 0

	echo "$0: golangci-lint is not installed; CI should just work" >&2
	confirm "Local dev? Install golangci-lint? [yN] " || return 1
	(
		set -eux
		cd $(mktemp -d)
		export GO111MODULE=on
		go mod init tmp
		go get github.com/golangci/golangci-lint/cmd/golangci-lint
		rm -rf $PWD
	)
}

confirm() {
    local reply
    local prompt="$1"
    read -n1 -p "$prompt" -t31 reply >&2
    echo "" >&2
    local rc=0
    local default_y=" \[Yn\] $"
    if [[ -z "$reply" ]] && [[ "$prompt" =~ $default_y ]] ; then
        reply="y"
    fi
    [[ "$reply" != "y" ]] && rc=1
    return $rc
}

main

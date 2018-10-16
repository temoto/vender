#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
set -eu

main() {
	cd $base

	git submodule update --init
	cc=$(which cc)
	if which ccache &>/dev/null ; then
		cc="$(which ccache) cc"
	fi
	( cd hardware/pigpio ; make CC="$cc" )

	paths=$(find . -type d ! -path '.' ! -path '*/.*' ! -path './script*' ! -path './vendor*')

	cargo build
	cargo test

	go get -t -v ./...
	go get -v github.com/golang/protobuf/protoc-gen-go
	go get -v golang.org/x/tools/cmd/stringer
	go generate ./...
	go build ./...

	# begin workaround cannot use test profile flag with multiple packages
	for d in $(go list ./...) ; do
		go test -timeout 3m -race -coverprofile=$base/coverage-$(basename $d).txt -covermode=atomic $d
	done
	cat $base/coverage-*.txt >$base/coverage.txt
	rm $base/coverage-*.txt
	# end workaround cannot use test profile flag with multiple packages

	go test -bench=. ./...

	for d in $paths ; do
		# skip directories without files
		if [[ -z "$(find $base/$d -depth 1 ! -type d ! -path '*/.*')" ]] ; then continue ; fi
		# skip directories with .go files, already built them
		if ls "$base/$d"/*.go >/dev/null 2>&1 ; then continue ; fi
		echo "no build defined for $d" >&2
		exit 1
	done
}

main

#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
set -eu

main() {
	cd $base

	paths=$(find . -type d ! -path '.' ! -path './.*' ! -path './archive*' ! -path './script*' ! -path './vendor*')

	go get -v github.com/golang/protobuf/protoc-gen-go
	for d in $paths ; do
		if ls $base/$d/*.proto >/dev/null 2>&1 ; then
			protoc -I=$base/$d --go_out=plugins=grpc:$base/$d $base/$d/*.proto
		fi
	done

	gopathbase=$(dirname $(go list ./helpers)) # path to any go package
	gopaths=$(go list ./... |egrep -v "$gopathbase/(archive|script|vendor)/")

	go get -t -v $gopaths
	go generate $gopaths
	go build $gopaths

	# begin workaround cannot use test profile flag with multiple packages
	for d in $gopaths ; do
		go test -race -coverprofile=$base/coverage-$(basename $d).txt -covermode=atomic $d
	done
	cat $base/coverage-*.txt >$base/coverage.txt
	rm $base/coverage-*.txt
	# end workaround cannot use test profile flag with multiple packages

	go test -bench=. $gopaths

	for d in $paths ; do
		# skip directories without files
		if ! ls -FAl "$base/$d/" |egrep -vq '^total|^d.+/$' && continue; then continue ; fi
		# skip directories with .go files, already built them
		if ls "$base/$d"/*.go >/dev/null 2>&1 ; then return ; fi
		echo "no build defined for $d" >&2
		exit 1
	done
}

main

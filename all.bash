#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
app="$1"
set -e

build_go() {
	if [[ -z "$app" ]] || [[ "$app" == "$1" ]]; then
		cd "$base/$1"
		go get .
		go build
		if ls ./*_test.go >/dev/null 2>&1; then
			go test
			go test -bench=.
		fi
	fi
}

main() {
	if [[ -z "$app" ]] || [[ "$app" == "avr-mdb" ]]; then
		cd "$base/avr-mdb"
		echo -n "CC version: "
		${CC-cc} --version
		echo -n "avr-gcc version: "
		avr-gcc --version
		make clean all
	fi

	build_go crc
	build_go display
	build_go display/lcd-test
	build_go head
	build_go i2c
	build_go i2c/i2c-test
}

main

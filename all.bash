#!/bin/bash
base="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
( cd "$base/avr-spi-uart" ; make clean all )
for d in display display/lcd-test head ; do
  ( cd "$base/$d" ; go get ; go build )
done

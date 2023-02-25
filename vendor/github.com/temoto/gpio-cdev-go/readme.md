# What

gpio-cdev-go is pure Go library to access Linux 4.8+ GPIO chardev interface. [![GoDoc](https://godoc.org/github.com/temoto/gpio-cdev-go?status.svg)](https://godoc.org/github.com/temoto/gpio-cdev-go)

Tested on:
- ARMv6,7 RaspberryPi 1, OrangePi Lite
- ARM64 RaspberryPi 3 B+

If you know how to make this work useful for more people, please take a minute to communicate:
- https://github.com/temoto/gpio-cdev-go/issues/new
- temotor@gmail.com

Ultimate success would be to merge this functionality into periph.io lib.


# Usage

Low level, bare ioctl API is provided by set of `Raw*` functions.

High-level wrapper (see api.go) is recommended way to use library.

```
// Open GPIO chip device.
// Default consumer tag will be used if one is not provided to line management functions.
chip, err := gpio.Open("/dev/gpiochip0", "default-consumer")
defer chip.Close()

// Set lines via either `SetBulk(values ...byte)`
// or create setter closure with `SetFunc(line) -> func(bool)`
// Either way you should call `Flush()` to commit changes to hardware.
pins, err := chip.OpenLines(
  gpio.GPIOHANDLE_REQUEST_OUTPUT, "hd44780",
  nRS, nRW, nE, nD4, nD5, nD6, nD7,
)
defer pins.Close()

pins.SetBulk(1, 0, 0, 1, 1, 1, 1)
err := pins.Flush()

set_pin_d4 := pins.SetFunc(nD4)
set_pin_d4(true)
err := pins.Flush()

// Reading current lines state
pins, err := chip.OpenLines(gpio.GPIOHANDLE_REQUEST_INPUT, "consumer", uint32(line))
defer pins.Close()
data, err := pins.Read()
lineActive := data.Values[0] == 1

// Waiting for edge. REQUEST_INPUT flag is implied.
lineEvent, err := chip.GetLineEvent(uint32(notifyLine), /*RequestFlag*/ 0,
  gpio.GPIOEVENT_REQUEST_RISING_EDGE, "consumer")
defer lineEvent.Close()
currentValue, err := lineEvent.Read()
eventData, err := lineEvent.Wait(timeout) // 0 to block forever
ok := eventData.ID == GPIOEVENT_EVENT_RISING_EDGE
```


# Possible issues

- may leak `req.fd` descriptors, TODO test


# Testing

* get 2 free GPIO pins
* jumper them
* set environment variables and run tests
```
export GPIO_TEST_DEV="/dev/gpiochip0"
export GPIO_TEST_PIN="19"
export GPIO_TEST_PIN_LOOP="16"
go test ./...
```


# Flair

[![Build status](https://travis-ci.org/temoto/gpio-cdev-go.svg?branch=master)](https://travis-ci.org/temoto/gpio-cdev-go)
[![Coverage](https://codecov.io/gh/temoto/gpio-cdev-go/branch/master/graph/badge.svg)](https://codecov.io/gh/temoto/gpio-cdev-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/temoto/gpio-cdev-go)](https://goreportcard.com/report/github.com/temoto/gpio-cdev-go)

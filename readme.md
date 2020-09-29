# What
Vender is free open source VMC (Vending Machine Controller).

Status:
- MDB adapter hardware module - works
- VMC - in development
- Configuration editor - planned


# Hardware

Required for VMC:
- Works on RaspberryPI and OrangePi Lite (H3). Possibly anything with GPIO that runs Go/Linux.
- MDB signal level inverter and current limiter - required, see files in `hardware/schematic`
- MDB adapter, takes care of 9bit and timing, we use ATMega328p with `hardware/mega-firmware` It is not mandatory, software option is available: https://github.com/temoto/iodin

Supported peripherals:
- MDB coin acceptor, bill validator
- Evend MDB drink devices
- any MDB device via configuration scenarios (work in progress)
- MT16S2R HD44780-like text display
- TWI(I2C) numpad keyboard
- SSD1306-compatible graphic display (planned)


# Design

VMC overall structure:
- engine (see internal/engine packages) executes actions, handles concurrency and errors
- device/feature drivers provide actions to engine
- configuration scenario specifies action groups and when to execute them


# Build

- Install Go 1.15 from https://golang.org/dl/
- Set target environment, default is `GOARCH=arm GOOS=linux`
- Run `script/build`
- Deploy file `build/vender` to your hardware

## Supported Go versions: 1.13 and 1.15

Vender compiled with Go 1.13 was successfully running in production until release v0.200630.0.
Go 1.14 introduced async preemtible runtime by interrupting syscalls. Go 1.15 os and net packages automatically retry on EINTR.

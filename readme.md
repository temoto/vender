# What
Vender is free open source VMC (Vending Machine Controller).

Status:
- MDB adapter hardware module - works
- VMC - in development
- Configuration editor - planned


# Hardware

Required for VMC:
- Works on RaspberryPI. We study OrangePi-{2,3}G-IOT for mobile cell connectivity. Possibly anything with GPIO that runs Go/Linux.
- MDB signal level inverter and current limiter - required, see files in `hardware/schematic`
- MDB adapter, takes care of 9bit and timing, we use ATMega328p with `hardware/mega-firmware` It is not mandatory, software option is available: https://github.com/temoto/iodin

Supported peripherals:
- MDB coin acceptor, bill validator
- Evend MDB drink devices (work in progress)
- any MDB device via configuration scenarios (work in progress)
- MT16S2R-like display (work in progress)
- TWI(I2C) numpad keyboard (work in progress)


# Design

VMC overall structure:
- engine (see engine packages) executes actions, handles concurrency and errors
- device/feature drivers provide actions to engine
- configuration scenario specifies action groups and when to execute them

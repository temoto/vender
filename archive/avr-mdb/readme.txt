What
====

AVR (ATMega) firmware implementing TWI(i2c) slave - MDB master bridge.
Used to set up 9-bit serial communication with vending devices.

A lot of vending machine components speak MDB protocol, which requires 9-bit UART.
We use RaspberryPi to control vending devices, but it's hard to make it speak 9-bit serial.

This firmware:
- accepts commands from TWI
- sends data to MDB
- parses response into buffer
- notifies master to initiate another TWI session
- sends MDB response

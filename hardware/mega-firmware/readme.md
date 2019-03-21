# What

Firmware for MDB adapter. Later called mega because ATMega328p MCU was used.

Usage patterns:
- VMC requests: MDB bus reset
- VMC requests: MDB send bytes xx, read response
- I2C keyboard: key pressed/released

VMC to mega communication happens over SPI bus. Since SPI communication must be initiated by master, a configured pin is raised high to indicate mega has something to say.


# Wire protocol v4

VMC and mega exchange frames in half-duplex mode.
Frame structure: `header length [payload...] checksum errcode padding...`

- `header` contains flags in high 4 bits, protocol version in low 4 bits
- `length` includes payload only
- `checksum` is CRC8 poly93, covers only length and payload
- `padding` is either PAD_OK or PAD_ERROR, clock SPI enough to get padding 4+ bytes.

Frame header values from master to mega:
- `04` used to receive any incoming data without sending
- `44` tries to send request; will be ignored if mega responds with `PROTOCOL_FLAG_REQUEST_BUSY` flag
- `84` Acknowledges successful read of `length` bytes with `checksum`. Complete frame looks like `84 ?? ack_length ack_crc`

Frame header values from mega to master:
- `04` ready to store incoming request, no response ready. If master sends payload, mega will reply with zeros until end of incoming payload (by declared length), then send checksum confirmation `00 FF crc_local crc_remote crc_payload errcode padding...`
- `44` will not store incoming request, sends response
- `84` will not store incoming request, no response, previous request is not processed yet
- `c4` will not store incoming request, sends response, previous request is not processed yet
To handle `PROTOCOL_FLAG_PAYLOAD`, perform

Command (from master) payload: `command [args...]`

Response (from mega) payload: `response field...`

`field` uses binary tagged encoding, see `protocol.h` `protocol.go`

Frame with `packet.header=RESET` sent by mega on reboot.


# Issues

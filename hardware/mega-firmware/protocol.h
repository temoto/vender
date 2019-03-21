#ifndef INCLUDE_PROTOCOL_H
#define INCLUDE_PROTOCOL_H
#include "config.h"

#include <inttypes.h>
#include <stdbool.h>
#include "buffer.h"

#define PROTOCOL_VERSION 4
#define PROTOCOL_HEADER_FLAG_MASK 0xf0
#define PROTOCOL_HEADER_VERSION_MASK 0x0f
#define PROTOCOL_FLAG_REQUEST_BUSY 0x80
#define PROTOCOL_FLAG_PAYLOAD 0x40
#define PROTOCOL_PAD_OK 0x01
#define PROTOCOL_PAD_ERROR 0xfe

#define PACKET_FIELDS_MAX_LENGTH (BUFFER_SIZE - 1)
#define FIELD_MAX_LENGTH (BUFFER_SIZE - 10)

#define TWI_LISTEN_MAX_LENGTH 36

typedef uint8_t command_t;
command_t const COMMAND_STATUS = 0x01;
command_t const COMMAND_CONFIG = 0x02;
command_t const COMMAND_RESET = 0x03;
command_t const COMMAND_DEBUG = 0x04;
command_t const COMMAND_FLASH = 0x05;
command_t const COMMAND_MDB_BUS_RESET = 0x07;
command_t const COMMAND_MDB_TRANSACTION_SIMPLE = 0x08;
command_t const COMMAND_MDB_TRANSACTION_CUSTOM = 0x09;

typedef uint8_t response_t;
response_t const RESPONSE_OK = 0x01;
response_t const RESPONSE_RESET = 0x02;
response_t const RESPONSE_TWI_LISTEN = 0x03;
response_t const RESPONSE_ERROR = 0x80;

typedef struct packet {
  bool filled;
  union {
    command_t command;
    response_t response;
  } h;
  buffer_t b;
} packet_t;

static void packet_clear_fast(packet_t* const p) __attribute__((nonnull));
static inline void packet_clear_fast(packet_t* const p) {
  p->h.command = 0;
  buffer_clear_fast(&p->b);
  p->filled = false;
}

typedef uint8_t errcode_t;
errcode_t const ERROR_FRAME_HEADER = 0x01;
errcode_t const ERROR_FRAME_LENGTH = 0x02;
errcode_t const ERROR_INVALID_CRC = 0x02;
errcode_t const ERROR_REQUEST_OVERWRITE = 0x03;
errcode_t const ERROR_INVALID_ACK = 0x04;
errcode_t const ERROR_BUFFER_OVERFLOW = 0x05;
errcode_t const ERROR_UNKNOWN_COMMAND = 0x10;
errcode_t const ERROR_INVALID_DATA = 0x11;
errcode_t const ERROR_NOT_IMPLEMENTED = 0x12;

// Protobuf-like encoding for response data.
typedef uint8_t field_t;
field_t const FIELD_INVALID = 0;
field_t const FIELD_FIRMWARE_VERSION = 0x01;  // len=2
field_t const FIELD_CLOCK10U = 0x02;          // len=2, uint16 by 10us
field_t const FIELD_MCUSR = 0x03;             // len=1
field_t const FIELD_ERRORN = 0x08;            // len=N
field_t const FIELD_ERROR2 = 0x09;            // len=2
field_t const FIELD_MDB_RESULT = 0x10;        // len=2: result_t, error-data
field_t const FIELD_MDB_DATA = 0x11;          // len=N, without checksum
field_t const FIELD_MDB_DURATION10U = 0x12;   // len=2, uint16 by 10us
field_t const FIELD_TWI_ADDR = 0x20;          // len=1
field_t const FIELD_TWI_DATA = 0x21;          // len=N

#define MDB_BLOCK_SIZE 36
#define MDB_ACK 0x00
#define MDB_RET 0xaa
#define MDB_NAK 0xff

typedef uint8_t mdb_state_t;
mdb_state_t const MDB_STATE_IDLE = 0;
mdb_state_t const MDB_STATE_ERROR = 1;
mdb_state_t const MDB_STATE_SEND = 2;
mdb_state_t const MDB_STATE_RECV = 3;
mdb_state_t const MDB_STATE_RECV_END = 4;
mdb_state_t const MDB_STATE_BUS_RESET = 5;
mdb_state_t const MDB_STATE_DONE = 6;

typedef uint8_t mdb_result_t;
mdb_result_t const MDB_RESULT_SUCCESS = 0x01;
mdb_result_t const MDB_RESULT_BUSY = 0x08;
mdb_result_t const MDB_RESULT_INVALID_CHK = 0x09;
mdb_result_t const MDB_RESULT_NAK = 0x0a;
mdb_result_t const MDB_RESULT_TIMEOUT = 0x0b;
mdb_result_t const MDB_RESULT_INVALID_END = 0x0c;
mdb_result_t const MDB_RESULT_RECEIVE_OVERFLOW = 0x0d;
mdb_result_t const MDB_RESULT_SEND_OVERFLOW = 0x0e;
mdb_result_t const MDB_RESULT_CODE_ERROR = 0x0f;
mdb_result_t const MDB_RESULT_UART_READ_UNEXPECTED = 0x10;
mdb_result_t const MDB_RESULT_UART_READ_ERROR = 0x11;
mdb_result_t const MDB_RESULT_UART_READ_OVERFLOW = 0x12;
mdb_result_t const MDB_RESULT_UART_READ_PARITY = 0x13;
mdb_result_t const MDB_RESULT_UART_SEND_BUSY = 0x14;
mdb_result_t const MDB_RESULT_UART_TXC_UNEXPECTED = 0x15;
mdb_result_t const MDB_RESULT_TIMER_CODE_ERROR = 0x18;

#endif  // INCLUDE_PROTOCOL_H

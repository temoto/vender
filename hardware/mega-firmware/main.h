#ifndef INCLUDE_MAIN_H
#define INCLUDE_MAIN_H
#include <inttypes.h>
#include <stdbool.h>

#define bit_mask_test(x, m) (((x) & (m)) == (m))
#define bit_mask_clear(x, m) ((x) &= ~(m))
#define bit_mask_set(x, m) ((x) |= (m))

#define MASTER_NOTIFY_DDR DDRB
#define MASTER_NOTIFY_PORT PORTB
#define MASTER_NOTIFY_PIN PINB2

uint16_t const FIRMWARE_VERSION = 0x0102;
uint8_t const PROTOCOL_VERSION = 2;

#define MDB_PACKET_SIZE 36
#define MDB_TIMEOUT 6  // ms
static uint8_t const MDB_ACK = 0x00;
static uint8_t const MDB_RET = 0xaa;
static uint8_t const MDB_NAK = 0xff;
typedef uint8_t mdb_state_t;
mdb_state_t const MDB_STATE_IDLE = 0;
mdb_state_t const MDB_STATE_ERROR = 1;
mdb_state_t const MDB_STATE_SEND = 2;
mdb_state_t const MDB_STATE_RECV = 3;
mdb_state_t const MDB_STATE_RECV_END = 4;
mdb_state_t const MDB_STATE_BUS_RESET = 5;

#define COMMAND_MAX_LENGTH 93
typedef uint8_t command_t;
command_t const COMMAND_STATUS = 0x01;
command_t const COMMAND_CONFIG = 0x02;
command_t const COMMAND_RESET = 0x03;
command_t const COMMAND_DEBUG = 0x04;
command_t const COMMAND_FLASH = 0x05;
command_t const COMMAND_MDB_BUS_RESET = 0x07;
command_t const COMMAND_MDB_TRANSACTION_SIMPLE = 0x08;
command_t const COMMAND_MDB_TRANSACTION_CUSTOM = 0x09;

#define RESPONSE_MAX_LENGTH 80
typedef uint8_t response_t;
#define RESPONSE_MASK_ERROR 0x80
response_t const RESPONSE_STATUS = 0x01;
// response_t const RESPONSE_CONFIG = 0x02;
response_t const RESPONSE_JUST_RESET = 0x03;
response_t const RESPONSE_DEBUG = 0x04;
response_t const RESPONSE_TWI = 0x06;
response_t const RESPONSE_MDB_SUCCESS = 0x08;
response_t const RESPONSE_BAD_PACKET = 0x0 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_INVALID_CRC = 0x1 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_INVALID_ID = 0x2 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_UNKNOWN_COMMAND = 0x3 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_INVALID_DATA = 0x4 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_BUFFER_OVERFLOW = 0x5 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_NOT_IMPLEMENTED = 0x6 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_BUSY = 0x8 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_INVALID_CHK = 0x9 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_NAK = 0xa + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_TIMEOUT = 0xb + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_INVALID_END = 0xc + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_RECEIVE_OVERFLOW = 0xd + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_SEND_OVERFLOW = 0xe + RESPONSE_MASK_ERROR;
response_t const RESPONSE_MDB_CODE_ERROR = 0xf + RESPONSE_MASK_ERROR;
response_t const RESPONSE_UART_READ_UNEXPECTED = 0x10 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_UART_READ_ERROR = 0x11 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_UART_READ_OVERFLOW = 0x12 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_UART_READ_PARITY = 0x13 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_UART_SEND_BUSY = 0x14 + RESPONSE_MASK_ERROR;
response_t const RESPONSE_TIMER_CODE_ERROR = 0x18 + RESPONSE_MASK_ERROR;

// Protobuf-like encoding in RESPONSE_STATUS, JUST_RESET, DEBUG, etc
typedef uint8_t field_t;
field_t const FIELD_INVALID = 0;
field_t const FIELD_PROTOCOL = 1;
field_t const FIELD_MCUSR = 2;
field_t const FIELD_BEEBEE = 3;
field_t const FIELD_MDB_STAT = 4;
field_t const FIELD_TWI_STAT = 5;
field_t const FIELD_UART_STAT = 6;
field_t const FIELD_QUEUE_MASTER = 7;
field_t const FIELD_QUEUE_TWI = 8;
field_t const FIELD_MDB_PROTOTCOL_STATE = 9;
field_t const FIELD_FIRMWARE_VERSION = 10;

static bool uart_send_ready(void);
static void mdb_init(void);
static bool mdb_step(void);
static void mdb_tx_begin(uint8_t const command_id);
static void mdb_bus_reset_begin(uint8_t const command_id,
                                uint16_t const duration);
static void mdb_bus_reset_finish(void);

static uint8_t master_command(uint8_t const *const bs,
                              uint8_t const max_length);
static void master_out_0(uint8_t const command_id, response_t const header);
static void master_out_1(uint8_t const command_id, response_t const header,
                         uint8_t const data);
static void master_out_2(uint8_t const command_id, response_t const header,
                         uint8_t const data1, uint8_t const data2);
static void master_out_n(uint8_t const command_id, response_t const header,
                         uint8_t const *const data, uint8_t const length);
static void twi_out_set_1(uint8_t const command_id, uint8_t const header,
                          uint8_t const data);
static void twi_init_slave(uint8_t const address);
static bool twi_step(void);

static void master_notify_init(void);
static void master_notify_set(bool const on);

static void timer1_set(uint16_t const ms);
static void timer1_stop(void);

static uint8_t memsum(uint8_t const *const src, uint8_t const length)
    __attribute__((pure));

#endif  // INCLUDE_MAIN_H

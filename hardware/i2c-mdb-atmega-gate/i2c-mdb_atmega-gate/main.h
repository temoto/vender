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

#define MDB_PACKET_SIZE 36
#define MDB_TIMEOUT 6  // ms
static uint8_t const MDB_ACK = 0x00;
static uint8_t const MDB_RET = 0xaa;
static uint8_t const MDB_NAK = 0xff;
typedef uint8_t MDB_State_t;
MDB_State_t const MDB_State_Idle = 0;
MDB_State_t const MDB_State_Error = 1;
MDB_State_t const MDB_State_Send = 2;
MDB_State_t const MDB_State_Recv = 3;
MDB_State_t const MDB_State_Recv_End = 4;
MDB_State_t const MDB_State_Bus_Reset = 5;

// master command
#define COMMAND_MAX_LENGTH 93
typedef uint8_t Command_t;
Command_t const Command_Poll = 0x01;
Command_t const Command_Config = 0x02;
Command_t const Command_Reset = 0x03;
Command_t const Command_Debug = 0x04;
Command_t const Command_Flash = 0x05;
Command_t const Command_MDB_Bus_Reset = 0x07;
Command_t const Command_MDB_Transaction_Simple = 0x08;
Command_t const Command_MDB_Transaction_Custom = 0x09;

#define RESPONSE_MAX_LENGTH 80
typedef uint8_t Response_t;
// slave ok
Response_t const Response_BeeBee[3] = {0xbe, 0xeb, 0xee};
Response_t const Response_Status = 0x01;
Response_t const Response_Debug = 0x04;
Response_t const Response_TWI = 0x05;
Response_t const Response_MDB_Started = 0x08;
Response_t const Response_MDB_Success = 0x09;
// slave error
Response_t const Response_Error = 0x80;
Response_t const Response_Bad_Packet = 0x81;
Response_t const Response_Invalid_CRC = 0x82;
Response_t const Response_Buffer_Overflow = 0x83;
Response_t const Response_Unknown_Command = 0x84;
Response_t const Response_Corruption = 0x85;
Response_t const Response_Not_Implemented = 0x86;
Response_t const Response_MDB_Busy = 0x88;
Response_t const Response_MDB_Invalid_CHK = 0x89;
Response_t const Response_MDB_NAK = 0x8a;
Response_t const Response_MDB_Timeout = 0x8b;
Response_t const Response_MDB_Invalid_End = 0x8c;
Response_t const Response_MDB_Receive_Overflow = 0x8d;
Response_t const Response_MDB_Send_Overflow = 0x8e;
Response_t const Response_UART_Read_Unexpected = 0x90;
Response_t const Response_UART_Read_Error = 0x91;
Response_t const Response_UART_Send_Busy = 0x92;

static bool uart_send_ready(void);
static void mdb_init(void);
static void mdb_reset(void);
static void mdb_start_send(void);
static bool mdb_step(void);

static uint8_t master_command(uint8_t const *const bs,
                              uint8_t const max_length);
static void master_out_1(Response_t const header);
static void master_out_2(Response_t const header, uint8_t const data);
static void master_out_n(Response_t const header, uint8_t const *const data,
                         uint8_t const data_length);
static void master_notify_init(void);
static void master_notify_set(bool const on);
static void twi_out_set_2(uint8_t const header, uint8_t const data);
static void twi_init_slave(uint8_t const address);
static bool twi_step(void);

static void timer0_set(uint8_t const ms);
static void timer0_stop(void);

static uint8_t memsum(uint8_t const *const src, uint8_t const length);

#endif  // INCLUDE_MAIN_H

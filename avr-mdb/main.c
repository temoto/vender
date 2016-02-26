#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/sleep.h>
#include <avr/wdt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <util/delay.h>
#include <util/twi.h>

#include "buffer.c"
#include "crc.h"
#include "ring.h"

/*
Glossary:
- TWI: two wire interface, exactly same as i2c.
  https://en.wikipedia.org/wiki/I%C2%B2C
- MDB: MultiDropBus, UART 9600-N-9 used in vending
  https://en.wikipedia.org/wiki/Multidrop_bus#MDB_in_Vending_Machines
- (The) master: device controlling this board (Raspberry Pi in our case)
- (The) slave: board with firmware from source code you read now

This ATMega device speaks:
- TWI slave to the master
- sends simple edge trigger to master to indicate data is ready
- TWI slave to keyboard hardware
- MDB master to vending devices

The master does:
- ask slave to write to MDB
- watch for high level on special pin
- ask slave to read MDB or keyboard data

TWI transaction may contain one or more packets. Packet always starts with
length and ends with CRC. Minimal length is 3 bytes.
CRC8 parameters: poly=0x93 init=0x00 xorout=0x00 refin=false refout=false.

Packet structure:
- length, total including all fields
- header, command/status
- N data bytes, N = length - 3, short packets have no data
- CRC8

Master header:
- 01 status poll, no data
- 02 update config, slave returns full config in response
- 03 reset, no data, wait 100ms then expect 0600beebee(CRC) from slave
- 04 read debug info
- 07 MDB bus reset (hold TX high for 100ms)
- 08-0f MDB transaction
  bit0 add auto CHK
  bit1 verify response CHK
  bit2 repeat on timeout
  useful values:
  08 (debug) your CHK, ignore response CHK, no repeat
  0f (release) auto add and verify CHK, repeat on timeout

Config data consists of 2 byte pairs, key-value. Keys:
- 01 respect master-slave TWI protocol CRC8, 1=verify (default) 0=ignore
- 02 MDB timeout, ms, default: 5
- 03 send MDB ACK, 1=after-data (default) 0=never 2=always
- 04 MDB max retries, default: 1
- 05 enable debug log (be ready to accept a lot of packets from slave)

Slave header, bit7=error:
- 01 OK, data[0] = master queue length
- 02 config
- 04 debug log
- 05 TWI incoming data from another master
- 08 MDB success - either received ACK or data with proper checksum
- 80 extended error in data bytes
- 81 bad packet (unknown command, etc) - reset or bug
- 82 invalid CRC8 - reset or bug
- 83 buffer overflow - retry after 1ms
- 88 MDB busy - retry
- 89 MDB invalid CHK - bug
- 8a MDB received NACK - retry
- 8b MDB timeout NACK - retry
- 90 UART chatterbox - retry or bug
- 91 UART read error - check UART wiring, retry or bug

Example talk:
M: 03 01 c8            03 length, 01 status poll, CRC
S: 06 00 be eb ee 75   06 length, 00 nothing, bee-bee, CRC

M: 04 0f 30 f7         0f MDB add and verify CHK repeat on timeout,
                       data = 30, CRC
S: 03 00 5b            nothing useful, check later
S: MDB send: 130 30 (first byte has 9 bit set), slave add MDB checksum
S: MDB read: 100 (ACK, 9 bit set)
S: notifies master

M: 03 01 c8            status poll
S: 03 08 3a            03 length, 08 MDB just ACK, CRC
*/

// master command
static uint8_t const Command_Poll = 0x01;
static uint8_t const Command_Config = 0x02;
static uint8_t const Command_Reset = 0x03;
static uint8_t const Command_Debug = 0x04;
static uint8_t const Command_MDB_Bus_Reset = 0x07;
static uint8_t const Command_MDB_Transaction_Low = 0x08;
static uint8_t const Command_MDB_Transaction_High = 0x0f;

// slave ok
static uint8_t const Response_BeeBee[3] = {0xbe, 0xeb, 0xee};
static uint8_t const Response_OK = 0x01;
static uint8_t const Response_Config = 0x02;
static uint8_t const Response_Debug = 0x04;
static uint8_t const Response_TWI = 0x05;
static uint8_t const Response_MDB_Started = 0x08;
static uint8_t const Response_MDB_Success = 0x09;
// slave error
static uint8_t const Response_Error = 0x80;
static uint8_t const Response_Bad_Packet = 0x81;
static uint8_t const Response_Invalid_CRC = 0x82;
static uint8_t const Response_Buffer_Overflow = 0x83;
static uint8_t const Response_Unknown_Command = 0x84;
static uint8_t const Response_Corruption = 0x85;
static uint8_t const Response_MDB_Busy = 0x88;
static uint8_t const Response_MDB_Protocol_Error = 0x89;
static uint8_t const Response_MDB_Invalid_CHK = 0x8a;
static uint8_t const Response_MDB_NACK = 0x8b;
static uint8_t const Response_MDB_Timeout = 0x8c;
static uint8_t const Response_UART_Chatterbox = 0x90;
static uint8_t const Response_UART_Read_Error = 0x91;

// forward
static void TWI_Out_Set_Short(uint8_t const);
static void Timer0_Set();
static void Timer0_Stop();

static uint8_t volatile mcu_status;

void early_init(void) __attribute__((naked)) __attribute__((section(".init3")));
void early_init(void) {
  wdt_disable();
  mcu_status = MCUSR;
  MCUSR = 0;
}

// Watchdog for software reset
static void soft_reset() __attribute__((noreturn));
static void soft_reset() {
  wdt_enable(WDTO_60MS);
  for (;;)
    ;
}

static bool bit_test(uint8_t const x, uint8_t const mask) {
  return (x & mask) == mask;
}

static uint8_t memsum(uint8_t const *const src, uint8_t const length) {
  uint8_t sum = 0;
  for (uint8_t i = 0; i < length; i++) {
    sum += src[i];
  }
  return sum;
}

static RingBuffer_t volatile buf_master_out;

static void Master_Out_1(uint8_t const header) {
  uint8_t const packet_length = 3;
  uint8_t const crc = crc8_p93_2b(packet_length, header);
  uint8_t const packet[3] = {packet_length, header, crc};
  if (!Ring_PushTailN(&buf_master_out, packet, sizeof(packet))) {
    return TWI_Out_Set_Short(Response_Buffer_Overflow);
  }
}

static void Master_Out_2(uint8_t const header, uint8_t const data) {
  uint8_t const packet_length = 4;
  if (buf_master_out.free < packet_length) {
    return TWI_Out_Set_Short(Response_Buffer_Overflow);
  }
  uint8_t const crc = crc8_p93_next(crc8_p93_2b(packet_length, header), data);
  uint8_t const packet[4] = {packet_length, header, data, crc};
  Ring_PushTailN(&buf_master_out, packet, packet_length);
}

static void Master_Out_N(uint8_t const header, uint8_t const *const data,
                         uint8_t const data_length) {
  uint8_t const packet_length = 3 + data_length;
  if (buf_master_out.free < packet_length) {
    return TWI_Out_Set_Short(Response_Buffer_Overflow);
  }
  Ring_Unsafe_Push(&buf_master_out, 0, packet_length);
  Ring_Unsafe_Push(&buf_master_out, 1, header);
  Ring_MoveTail(&buf_master_out, 2);
  uint8_t crc = crc8_p93_2b(packet_length, header);
  for (uint8_t i = 0; i < data_length; i++) {
    Ring_Unsafe_Push(&buf_master_out, i, data[i]);
    crc = crc8_p93_next(crc, data[i]);
  }
  Ring_MoveTail(&buf_master_out, data_length);
  Ring_PushTail(&buf_master_out, crc);
}

static void Master_Out_Printf(uint8_t const header, char const *s, ...) {
  static char strbuf[101];
  va_list ap;
  va_start(ap, s);
  int16_t const length = vsnprintf(strbuf, sizeof(strbuf), s, ap);
  va_end(ap);
  if ((length < 0) || (length >= sizeof(strbuf))) {
    return TWI_Out_Set_Short(Response_Buffer_Overflow);
  }
  return Master_Out_N(header, (uint8_t const *)strbuf, length);
}

static void LED_Set(bool const on) {
  if (on) {
    PORTB |= _BV(PINB5);
  } else {
    PORTB &= ~_BV(PINB5);
  }
}

static inline void LED_Init() {
  DDRB |= _BV(PINB5);
  LED_Set(false);
}

static void Master_Notify_Set(bool const on) {
  if (on) {
    PORTB |= _BV(PINB1);
  } else {
    PORTB &= ~_BV(PINB1);
  }
  LED_Set(on);
}

static void Master_Notify_Init() {
  DDRB |= _BV(PINB1);
  Master_Notify_Set(false);
}

// Begin MDB driver
static uint8_t const MDB_STATE_IDLE = 0x00;
static uint8_t const MDB_STATE_TIMEOUT = 0x01;
static uint8_t const MDB_STATE_TX_BEGIN = 0x08;
static uint8_t const MDB_STATE_TX_DATA = 0x09;
static uint8_t const MDB_STATE_TX_ACK = 0x0a;
static uint8_t const MDB_STATE_TX_NACK = 0x0b;
static uint8_t const MDB_STATE_TX_RET = 0x0c;
static uint8_t const MDB_STATE_RX = 0x10;
static uint8_t const MDB_STATE_RX_END = 0x11;
static uint8_t volatile mdb_state;

static uint8_t const MDB_ACK = 0x00;
static uint8_t const MDB_RET = 0x55;
static uint8_t const MDB_NACK = 0xff;

static Buffer_t volatile mdb_in;
static uint8_t volatile mdb_in_data[39];
static Buffer_t volatile mdb_out;
static uint8_t volatile mdb_out_data[39];

#ifdef _AVR_IOM128_H_
#define USART_RX_vect USART0_RX_vect
#define USART_TX_vect USART0_TX_vect
#define USART_UDRE_vect USART0_UDRE_vect
#endif

static void MDB_Init() {
  mdb_state = MDB_STATE_IDLE;
  Buffer_Init(&mdb_in, (uint8_t * const)mdb_in_data, sizeof(mdb_in_data));
  Buffer_Init(&mdb_out, (uint8_t * const)mdb_out_data, sizeof(mdb_out_data));

  DDRD |= _BV(PD1);
  DDRD &= ~_BV(PD0);

#define BAUD 9600
#include <util/setbaud.h>
  UBRR0H = UBRRH_VALUE;
  UBRR0L = UBRRL_VALUE;
#if USE_2X
  UCSR0A |= (1 << U2X0);
#else
  UCSR0A &= ~(1 << U2X0);
#endif

  // #define USART_PRESCALE (((F_CPU) / (BAUD * 16UL)) - 1)
  // UBRR0H = (uint8_t const)(USART_PRESCALE >> 8);
  // UBRR0L = (uint8_t const)(USART_PRESCALE);

  UCSR0B = 0
           // enable rx, tx
           | _BV(RXEN0) | _BV(TXEN0)
           // interrupts
           //
           // FIXME: doesn't work now
           //
           // | _BV(RXCIE0) | _BV(UDRIE0)
           // enable 8 bit
           | _BV(RXB80) | _BV(TXB80)
           // 9 data bits
           | _BV(UCSZ02);
  // 9 data bits
  UCSR0C |= _BV(UCSZ00) | _BV(UCSZ01);
}

static uint8_t MDB_Send(uint8_t const *const src, uint8_t const length,
                        bool const add_chk) {
  uint8_t const total_length = length + (add_chk ? 1 : 0);
  if (total_length > mdb_out.size) {
    return Response_Buffer_Overflow;
  }
  mdb_out.length = total_length;
  mdb_out.used = 0;
  memcpy(mdb_out.data, src, length);
  if (add_chk) {
    mdb_out.data[total_length - 1] = memsum(src, length);
  }
  mdb_state = MDB_STATE_TX_BEGIN;
  Master_Out_Printf(Response_Debug, "M:st=TXB");
  return Response_MDB_Started;
}

static void MDB_Step() {
  if (mdb_state == MDB_STATE_RX_END) {
    if (mdb_in.length == 1) {
      uint8_t const data = mdb_in.data[0];
      if (data == MDB_ACK) {
        Master_Out_1(Response_MDB_Success);
      } else if (data == MDB_NACK) {
        Master_Out_1(Response_MDB_NACK);
      } else {
        Master_Out_2(Response_MDB_Protocol_Error, data);
      }
      mdb_state = MDB_STATE_IDLE;
      Master_Out_Printf(Response_Debug, "M:Step:RXE/1-I");
    } else {
      if (/*config verify chk*/ true) {
        uint8_t const chk = memsum(mdb_in.data, mdb_in.length - 1);
        uint8_t const chk_in = mdb_in.data[mdb_in.length - 1];
        if (chk_in != chk) {
          // Use RET? - Not yet.
          // mdb_state = MDB_STATE_TX_RET;
          // mdb_out.used = 0;
          // return;

          mdb_state = MDB_STATE_IDLE;
          Master_Out_Printf(Response_Debug, "M:Step:RXE/C!-I");
          Master_Out_N(Response_MDB_Invalid_CHK, mdb_in.data, mdb_in.length);
          return;
        }
      }
      mdb_state = MDB_STATE_TX_ACK;
      Master_Out_Printf(Response_Debug, "M:Step:RXE/Cv-TA");
      Master_Out_N(Response_MDB_Success, mdb_in.data, mdb_in.length - 1);
    }
  } else if (mdb_state == MDB_STATE_TIMEOUT) {
    mdb_state = MDB_STATE_IDLE;
    Master_Out_Printf(Response_Debug, "M:Step:TO-I");
    Master_Out_1(Response_MDB_Timeout);
  }
}

static bool UART_Recv_Ready() { return bit_test(UCSR0A, _BV(RXC0)); }

static void UART_Recv() {
  uint8_t const sra = UCSR0A;
  uint8_t const srb = UCSR0B;
  uint8_t const data = UDR0;
  bool const bit9 = bit_test(srb, _BV(RXB80));
  uint8_t const debug[5] = {bit9 ? 0x80 : 0, data, sra, srb, mdb_state};
  if ((sra & (_BV(FE0) | _BV(DOR0) | _BV(UPE0))) != 0) {
    // uart_error = Response_UART_Read_Error;
    // memcpy((void *)uart_debug, debug, sizeof(debug));
    Master_Out_N(Response_UART_Read_Error, (uint8_t const *const)debug,
                 sizeof(debug));

    mdb_state = MDB_STATE_TX_NACK;
    Master_Out_Printf(Response_Debug, "UR:err-TXN");
    return;
  }
  if (mdb_state == MDB_STATE_RX) {
    if (!Buffer_Append(&mdb_in, data)) {
      Master_Out_N(Response_Buffer_Overflow, (uint8_t const *const)debug,
                   sizeof(debug));
      mdb_state = MDB_STATE_IDLE;
      Master_Out_Printf(Response_Debug, "UR:RX/ap!-I");
      return;
    }
    if (bit9) {
      Timer0_Stop();
      mdb_state = MDB_STATE_RX_END;
      Master_Out_Printf(Response_Debug, "UR:RX/9-RXE");
    }
  } else {
    // uart_error = Response_UART_Chatterbox;
    // memcpy((void *)uart_debug, debug, sizeof(debug));
    Master_Out_N(Response_UART_Chatterbox, (uint8_t const *const)debug,
                 sizeof(debug));
    mdb_state = MDB_STATE_TX_NACK;
    Master_Out_Printf(Response_Debug, "UR:RX!?-TXN");
  }
}

static bool UART_Recv_Check() {
  if (!UART_Recv_Ready()) {
    return false;
  }
  UART_Recv();
  return true;
}

static void UART_Send_Byte(uint8_t const b, bool const bit9) {
  if (bit9) {
    UCSR0B |= _BV(TXB80);
  } else {
    UCSR0B &= ~_BV(TXB80);
  }
  UDR0 = b;
}

static bool UART_Send_Ready() { return bit_test(UCSR0A, _BV(UDRE0)); }

static void UART_Send() {
  if (mdb_out.used >= mdb_out.length) {
    return;
  }
  uint8_t const data = mdb_out.data[mdb_out.used];
  if (mdb_state == MDB_STATE_TX_BEGIN) {
    UART_Send_Byte(data, true);
    mdb_out.used++;
    mdb_state = MDB_STATE_TX_DATA;
    Master_Out_Printf(Response_Debug, "US:TXB-TXD");
    return;
  } else if (mdb_state == MDB_STATE_TX_DATA) {
    UART_Send_Byte(data, false);
    mdb_out.used++;
    if (mdb_out.used >= mdb_out.length) {
      // I finished, what have you to say?
      mdb_state = MDB_STATE_RX;
      Master_Out_Printf(Response_Debug, "US:TXD/used-RX");
      Timer0_Set();
    }
    return;
  } else if (mdb_state == MDB_STATE_TX_ACK) {
    Timer0_Stop();
    UART_Send_Byte(MDB_ACK, false);
    mdb_state = MDB_STATE_IDLE;
    Master_Out_Printf(Response_Debug, "US:TXA-I");
    return;
  } else if (mdb_state == MDB_STATE_TX_RET) {
    UART_Send_Byte(MDB_RET, false);
    Timer0_Set();
    mdb_state = MDB_STATE_RX;
    Master_Out_Printf(Response_Debug, "US:TXR-RX");
    mdb_in.length = mdb_in.used = 0;
    return;
  } else if (mdb_state == MDB_STATE_TX_NACK) {
    Timer0_Stop();
    UART_Send_Byte(MDB_NACK, false);
    mdb_state = MDB_STATE_IDLE;
    Master_Out_Printf(Response_Debug, "US:TXN-I");
    return;
  }
}

static bool UART_Send_Check() {
  if (!UART_Send_Ready()) {
    return false;
  }
  UART_Send();
  return true;
}
// End MDB driver

// Begin Timer0 driver
#define timer0_ocra (F_CPU / 1024UL * 5000UL / 1000000UL)

static void Timer0_Set() {
  // TCCR0A = _BV(WGM01);
  TCCR0A = 0;
  TCNT0 = 0;
  OCR0A = timer0_ocra;
  TIMSK0 |= _BV(OCIE0A);
  // CTC, F_CPU/1024
  TCCR0B = _BV(WGM02) | _BV(CS02) | _BV(CS00);
}

static void Timer0_Stop() {
  TCCR0B = 0;
  TCCR0A = 0;
  TIMSK0 &= ~_BV(OCIE0A);
}

ISR(TIMER0_COMPA_vect) {
  Timer0_Stop();
  Master_Out_Printf(Response_Error, "/D:T0");
  if (mdb_state == MDB_STATE_RX) {
    mdb_state = MDB_STATE_TIMEOUT;
    Master_Out_Printf(Response_Debug, "Tim:RX-TO");
  } else if (mdb_state != MDB_STATE_IDLE) {
    Master_Out_Printf(Response_Error, "/T:M=R");
    // debug, invalid state
  }
}
// End Timer0 driver

// Begin TWI driver
static bool volatile twi_idle = true;
static uint8_t volatile twi_in_data[128];
static uint8_t volatile twi_out_data[128];
static Buffer_t volatile twi_in;
static Buffer_t volatile twi_out;

static void TWI_Init_Slave(uint8_t const address) {
  TWCR = 0;
  TWBR = 0x0c;
  TWAR = address << 1;
  TWSR = 0;
  twi_idle = true;
  Buffer_Init(&twi_in, (uint8_t * const)twi_in_data, sizeof(twi_in_data));
  Buffer_Init(&twi_out, (uint8_t * const)twi_out_data, sizeof(twi_out_data));
  TWCR = _BV(TWINT) | _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
}

static void TWI_Out_Set_Short(uint8_t const header) {
  uint8_t const length = 3;
  twi_out.length = length;
  twi_out.used = 0;
  twi_out.data[0] = length;
  twi_out.data[1] = header;
  twi_out.data[2] = crc8_p93_2b(length, header);
  twi_out.data[3] = 0;
}

static void TWI_Out_Set_Long1(uint8_t const header, uint8_t const data) {
  uint8_t const length = 4;
  twi_out.length = length;
  twi_out.used = 0;
  twi_out.data[0] = length;
  twi_out.data[1] = header;
  twi_out.data[2] = data;
  twi_out.data[3] = crc8_p93_next(crc8_p93_2b(length, header), data);
  twi_out.data[4] = 0;
}

ISR(TWI_vect) {
  bool ack = false;
  switch (TW_STATUS) {
    case TW_NO_INFO:
      return;

    case TW_BUS_ERROR:
      twi_in.length = twi_in.used = 0;
      twi_out.length = twi_out.used = 0;
      TWCR = _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      twi_idle = false;
      twi_in.length = twi_in.used = 0;
      ack = true;
      break;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      twi_idle = false;
      twi_in.data[twi_in.length] = TWDR;
      if (twi_in.length < twi_in.size) {
        twi_in.length++;
      }
      ack = true;
      break;

    // data received, NACK returned
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      twi_idle = false;
      ack = false;
      break;

    // Receive Stop or ReStart
    case TW_SR_STOP:
      twi_idle = true;
      twi_in.used = twi_in.length;
      ack = true;
      break;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      twi_idle = false;
      if (twi_out.length == 0) {
        TWI_Out_Set_Long1(Response_OK, buf_master_out.length);
      } else {
        twi_out.used = 0;
      }
      ack = (twi_out.used < twi_out.length);
      if (ack) {
        TWDR = twi_out.data[twi_out.used];
      } else {
        TWDR = 0;
      }
      break;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      twi_idle = false;
      ack = (twi_out.used < twi_out.length);
      if (ack) {
        twi_out.used++;
        TWDR = twi_out.data[twi_out.used];
      } else {
        TWDR = 0;
      }
      break;

    // Send Last Byte Receive ACK
    case TW_ST_LAST_DATA:
    // Send Last Byte Receive NACK
    case TW_ST_DATA_NACK:
      twi_idle = true;
      twi_out.length = twi_out.used = 0;
      ack = true;
      break;
  }
  TWCR = _BV(TWINT) | (ack ? _BV(TWEA) : 0) | _BV(TWEN) | _BV(TWIE);
}
// End TWI driver

static void Init() {
  // LED_Init();
  Ring_Init(&buf_master_out);
  MDB_Init();
  TWI_Init_Slave(0x78);
  set_sleep_mode(SLEEP_MODE_IDLE);
  Master_Notify_Init();
  Timer0_Stop();

  // disable ADC
  ADCSRA &= ~_BV(ADEN);
  // power reduction
  PRR |= _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);

  // hello after reset
  Master_Out_N(Response_Debug, Response_BeeBee, sizeof(Response_BeeBee));
}

static uint8_t Master_Command(uint8_t const *bs, uint8_t const max_length) {
  if (max_length < 3) {
    Master_Out_1(Response_Bad_Packet);
    return max_length;
  }
  uint8_t const length = bs[0];
  if (length > max_length) {
    Master_Out_1(Response_Bad_Packet);
    return length;
  }
  uint8_t const crc_in = bs[length - 1];
  uint8_t const crc_local = crc8_p93_n(0, bs, length - 1);
  if (crc_in != crc_local) {
    Master_Out_2(Response_Invalid_CRC, crc_in);
    return length;
  }

  uint8_t const header = bs[1];
  uint8_t const data_length = length - 3;
  uint8_t const *const data = bs + 2;
  if (header == Command_Poll) {
    if (length == 3) {
      Master_Out_2(Response_OK, buf_master_out.length);
    } else {
      Master_Out_1(Response_Bad_Packet);
    }
    // TODO
    //} else if (cmd == Command_Config) {
  } else if (header == Command_Reset) {
    Init();
    // soft_reset();  // noreturn
    return length;
  } else if (header == Command_Debug) {
    Master_Out_Printf(Response_Debug, "MDB:s=%c,in=%s,out=%s", mdb_state + '0',
                      mdb_in.data, mdb_out.data);

    // TODO:
    // } else if (header == Command_MDB_Bus_Reset) {
  } else if ((header >= Command_MDB_Transaction_Low) &&
             (header <= Command_MDB_Transaction_High)) {
    if (mdb_state != MDB_STATE_IDLE) {
      Master_Out_1(Response_MDB_Busy);
      return length;
    }
    // TODO: get from config
    bool const add_chk = true;
    uint8_t const response = MDB_Send(data, data_length, add_chk);
    Master_Out_2(response, data_length);
  } else {
    Master_Out_1(Response_Unknown_Command);
  }
  return length;
}

static bool Poll() {
  bool again = false;

  // TWI read is finished
  if (twi_idle && (twi_in.length > 0)) {
    uint8_t *src = twi_in.data;
    if (twi_in.length == 1) {
      // keyboard sends 1 byte
      Master_Out_2(Response_TWI, src[0]);
      again = true;
    } else {
      // master sends >= 3 bytes
      uint8_t i = 0;
      for (;;) {
        uint8_t const consumed = Master_Command(src, twi_in.length - i);
        if (consumed == 0) {
          break;
        }
        i += consumed;
        src += consumed;
        if (i >= twi_in.length) {
          break;
        }
      }
      again = true;
    }
    twi_in.length = twi_in.used = 0;
  }

  if (mdb_state != MDB_STATE_IDLE) {
    MDB_Step();
    again |= UART_Send_Check();
    again |= UART_Recv_Check();
    MDB_Step();
  }

  if (twi_idle && (twi_out.used >= twi_out.length) &&
      (buf_master_out.length >= 3)) {
    twi_out.used = 0;
    twi_out.length = buf_master_out.length;
    twi_out.data[twi_out.length] = 0;
    twi_out.data[twi_out.length + 1] = 0;
    if (!Ring_PopHeadN(&buf_master_out, twi_out.data, buf_master_out.length)) {
      TWI_Out_Set_Short(Response_Corruption);
    }
    again = true;
  }

  return again;
}

static void Poll_Loop(int8_t const max_repeats) {
  for (int8_t i = 0; i < max_repeats; i++) {
    if (!Poll()) {
      break;
    }
  }
}

#ifdef TEST
#include "main_test.c"
#else
int main(void) __attribute__((naked));
int main(void) {
  cli();
  Init();
  Master_Out_Printf(Response_Debug, "RST:%d", mcu_status);

  for (;;) {
    sei();
    _delay_us(5);

    while (!twi_idle) {
      _delay_us(5);
    }
    // sleep_mode();
    // while (!twi_idle) {
    //   sleep_mode();
    // }

    cli();

    Poll_Loop(2);

    Master_Notify_Set((!twi_idle) || (twi_out.used < twi_out.length) ||
                      (buf_master_out.length >= 3));
  }
}
#endif

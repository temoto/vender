#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/sleep.h>
#include <avr/wdt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
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

To recover from unexpected situation, master sends 3 null bytes: 00 00 00
then waits for 100ms before checking status. Slave must perform full reset
and send special bee-bee packet: 06 00 be eb ee CRC.

Master header:
- 01 status poll, no data
- 02 read config
- 03 update config
- 07 MDB bus reset (hold TX high for 100ms)
- 08-0f MDB transaction
  bit0 add auto CHK
  bit1 check response CHK
  bit2 repeat on timeout
  useful values:
  08 (debug) your CHK, ignore response CHK, no repeat
  0f (release) auto add and verify CHK, repeat on timeout

Config data consists of 2 byte pairs, key-value. Keys:
- 01 respect master-slave TWI protocol CRC8, 1=check (default) 0=ignore
- 02 MDB timeout, ms, default: 5
- 03 send MDB ACK, 1=after-data (default) 0=never 2=always
- 04 MDB max retries, default: 1
- 05 enable debug log (be ready to accept a lot of packets from slave)

Slave header, bit7=error:
- 00 nothing useful, may contain debug info in data bytes
- 01 config
- 02 TWI incoming data from another master
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
static uint8_t const Command_Reset[3] = {0, 0, 0};
static uint8_t const Command_Poll = 0x01;
static uint8_t const Command_Config_Read = 0x02;
static uint8_t const Command_Config_Update = 0x03;
static uint8_t const Command_MDB_Bus_Reset = 0x07;
static uint8_t const Command_MDB_Transaction_Low = 0x08;
static uint8_t const Command_MDB_Transaction_High = 0x0f;

// slave ok
static uint8_t const Response_BeeBee[3] = {0xbe, 0xeb, 0xee};
static uint8_t const Response_Nothing = 0x00;
static uint8_t const Response_Config = 0x01;
static uint8_t const Response_TWI = 0x02;
static uint8_t const Response_MDB_Success = 0x08;
// slave error
static uint8_t const Response_Error = 0x80;
static uint8_t const Response_Bad_Packet = 0x81;
static uint8_t const Response_Invalid_CRC = 0x82;
static uint8_t const Response_Buffer_Overflow = 0x83;
static uint8_t const Response_Unknown_Command = 0x84;
static uint8_t const Response_Corruption = 0x85;
static uint8_t const Response_MDB_Busy = 0x88;
static uint8_t const Response_MDB_Invalid_CHK = 0x89;
static uint8_t const Response_MDB_NACK = 0x8a;
static uint8_t const Response_MDB_Timeout = 0x8b;
static uint8_t const Response_UART_Chatterbox = 0x90;
static uint8_t const Response_UART_Read_Error = 0x91;

// forward
static void TWI_Out_Set_Short(uint8_t const);
static void Timer0_Set();

// Watchdog for software reset
void wdt_init(void) __attribute__((naked)) __attribute__((section(".init3")));
void wdt_init(void) {
  // MCUSR = 0;
  wdt_disable();
  return;
}
void soft_reset() {
  wdt_enable(WDTO_30MS);
  for (;;)
    ;
}

static bool bit_test(uint8_t const x, uint8_t const mask) {
  return (x & mask) == mask;
}

static RingBuffer_t volatile buf_master_out;

static void Master_Out_Append_Short(uint8_t const header) {
  uint8_t const packet_length = 3;
  uint8_t const crc = crc8_p93_2b(packet_length, header);
  uint8_t const packet[3] = {packet_length, header, crc};
  if (!Ring_PushTailN(&buf_master_out, packet, sizeof(packet))) {
    TWI_Out_Set_Short(Response_Buffer_Overflow);
  }
}

static void Master_Out_Append_Long(uint8_t const header,
                                   uint8_t const *const data,
                                   uint8_t const data_length) {
  uint8_t const packet_length = 3 + data_length;
  if (buf_master_out.free < packet_length) {
    TWI_Out_Set_Short(Response_Buffer_Overflow);
  }
  Ring_PushTail2(&buf_master_out, packet_length, header);
  Ring_PushTailN(&buf_master_out, data, data_length);
  uint8_t crc = crc8_p93_2b(packet_length, header);
  crc = crc8_p93_n(crc, data, data_length);
  Ring_PushTail(&buf_master_out, crc);
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
static uint8_t const MDB_STATE_IDLE = 0;
static uint8_t const MDB_STATE_TIMEOUT = 1;
static uint8_t const MDB_STATE_TX_BEGIN = 10;
static uint8_t const MDB_STATE_TX_DATA = 11;
static uint8_t const MDB_STATE_TX_ACK = 12;
static uint8_t const MDB_STATE_TX_NACK = 13;
static uint8_t const MDB_STATE_TX_RET = 14;
static uint8_t const MDB_STATE_RX = 20;
static uint8_t const MDB_STATE_RX_END = 21;
static uint8_t volatile mdb_state;
#define MDB_State_Idle (mdb_state == MDB_STATE_IDLE)
static uint8_t volatile mdb_chk;
static uint8_t volatile uart_error;
static uint8_t volatile uart_debug[4];

static uint8_t volatile mdb_in_data[39];
static uint8_t volatile mdb_out_data[39];
static Buffer_t volatile mdb_in;
static Buffer_t volatile mdb_out;

#ifdef _AVR_IOM128_H_
#define USART_RX_vect USART0_RX_vect
#define USART_TX_vect USART0_TX_vect
#define USART_UDRE_vect USART0_UDRE_vect
#endif

static void MDB_Init() {
  Buffer_Init(&mdb_in, (uint8_t * const)mdb_in_data, sizeof(mdb_in_data));
  Buffer_Init(&mdb_out, (uint8_t * const)mdb_out_data, sizeof(mdb_out_data));

// DDRD |= _BV(PD1);
// DDRD &= ~_BV(PD0);

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
           // enable rx, tx and interrupts
           | _BV(RXEN0) | _BV(TXEN0) | _BV(RXCIE0) | _BV(TXCIE0)
           // enable 8 bit
           | _BV(RXB80) | _BV(TXB80)
           // 9 data bits
           | _BV(UCSZ02);
  // 9 data bits
  UCSR0C |= _BV(UCSZ00) | _BV(UCSZ01);
}

static bool UART_Recv_Ready() { return bit_test(UCSR0A, _BV(RXC0)); }

static void UART_Recv() {
  uint8_t const srlow = UCSR0A;
  uint8_t const srhigh = UCSR0B;
  uint8_t const data = UDR0;
  bool const bit9 = bit_test(srhigh, _BV(1));
  uint8_t const debug[4] = {bit9 ? 0x80 : 0, data, srhigh, srlow};
  if ((srlow & (_BV(FE0) | _BV(DOR0) | _BV(UPE0))) != 0) {
    uart_error = Response_UART_Read_Error;
    memcpy((void *)uart_debug, debug, sizeof(debug));
    return;
  }
  if (mdb_state == MDB_STATE_RX) {
    if (!Buffer_Append(&mdb_in, data)) {
      uart_error = Response_Buffer_Overflow;
      memcpy((void *)uart_debug, debug, sizeof(debug));
      return;
    }
    if (bit9) {
      mdb_state = MDB_STATE_RX_END;
    }
  } else {
    uart_error = Response_UART_Chatterbox;
    memcpy((void *)uart_debug, debug, sizeof(debug));
  }
}

static bool UART_Recv_Loop(int8_t const max_repeats) {
  bool activity = false;
  for (int8_t i = max_repeats; i >= 0; i--) {
    if (buf_master_out.free < 3) {
      break;
    }
    if (!UART_Recv_Ready()) {
      break;
    }
    UART_Recv();
    activity = true;
  }
  return activity;
}

ISR(USART_RX_vect) { UART_Recv_Loop(2); }

static void UART_Send_Byte(uint8_t const b, bool const bit9) {
  if (bit9) {
    UCSR0B |= _BV(TXB80);
  } else {
    UCSR0B &= ~_BV(TXB80);
  }
  UDR0 = b;
}

static bool UART_Send_Ready() { return bit_test(UCSR0A, _BV(UDRE0)); }
// static bool UART_Send_Done() { return bit_test(UCSR0A, _BV(TXC0)); }

static bool UART_Send_Loop(int8_t const max_repeats) {
  bool activity = false;
  for (int8_t i = max_repeats; i >= 0; i--) {
    if (mdb_out.used >= mdb_out.length) {
      break;
    }
    if (!UART_Send_Ready()) {
      break;
    }
    uint8_t const data = mdb_out.data[mdb_out.used];
    if (mdb_state == MDB_STATE_TX_BEGIN) {
      UART_Send_Byte(data, true);
      mdb_out.used++;
      mdb_state = MDB_STATE_TX_DATA;
    } else if (mdb_state == MDB_STATE_TX_DATA) {
      UART_Send_Byte(data, false);
      mdb_out.used++;
      if (mdb_out.used >= mdb_out.length) {
        // I finished, what have you to say?
        mdb_state = MDB_STATE_RX;
        Timer0_Set();
      }
    } else if (mdb_state == MDB_STATE_TX_ACK) {
      UART_Send_Byte(0x00, true);
    } else if (mdb_state == MDB_STATE_TX_RET) {
      UART_Send_Byte(0xaa, true);
    } else if (mdb_state == MDB_STATE_TX_NACK) {
      UART_Send_Byte(0xff, true);
    }
    activity = true;
  }
  return activity;
}

ISR(USART_UDRE_vect) { UART_Send_Loop(2); }

// ISR(USART_TX_vect) {}
// End MDB driver

// Begin Timer0 driver
#define timer0_ocra (5000UL / F_CPU * 1024UL)

static void Timer0_Set() {
  TCCR0A = _BV(WGM01);
  OCR0A = timer0_ocra;
  // CTC, F_CPU/1024
  TCCR0B = _BV(CS02) | _BV(CS00);
  TIMSK0 |= _BV(OCIE0A);
}

static void Timer0_Stop() {
  TCCR0A = 0;
  TCCR0B = 0;
  TIMSK0 &= ~_BV(OCIE0A);
}

ISR(TIMER0_COMPA_vect) {
  Timer0_Stop();
  if (mdb_state == MDB_STATE_RX) {
    mdb_state = MDB_STATE_TIMEOUT;
  } else if (mdb_state == MDB_STATE_RX) {
    // debug, invalid state
  }
}
// End Timer0 driver

// Begin TWI driver
static bool volatile twi_idle = 0;
static uint8_t volatile twi_in_data[253];
static uint8_t volatile twi_out_data[253];
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
  // uint8_t const packet[3] = {3, header, crc8_p93_2b(3, header)};
  // Buffer_Set(&twi_out, packet, sizeof(packet));
  twi_out.length = 3;
  twi_out.used = 0;
  twi_out.data[0] = 3;
  twi_out.data[1] = header;
  twi_out.data[2] = crc8_p93_2b(3, header);
}

static bool TWI_Out_Append_From_Master_Out() {
  uint8_t length;
  if (!Ring_PeekHead(&buf_master_out, &length)) {
    return false;
  }
  if (buf_master_out.length < length) {
    TWI_Out_Set_Short(Response_Corruption);
    return false;
  }
  if (length > twi_out.size) {
    TWI_Out_Set_Short(Response_Buffer_Overflow);
    return false;
  }
  twi_out.length = length;
  twi_out.used = 0;
  if (!Ring_PopHeadN(&buf_master_out, twi_out.data, length)) {
    TWI_Out_Set_Short(Response_Corruption);
    return false;
  }

  return true;
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
      twi_out.used = 0;
      if (twi_out.length == 0) {
        TWI_Out_Set_Short(Response_Nothing);
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
  twi_idle = true;

  // disable ADC
  ADCSRA &= ~_BV(ADEN);
  // power reduction
  PRR |= _BV(PRTIM1) | _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);

  // hello after reset
  Master_Out_Append_Long(Response_Nothing, Response_BeeBee,
                         sizeof(Response_BeeBee));
}

static uint8_t const Master_Command(uint8_t const *bs,
                                    uint8_t const max_length) {
  if (max_length < 3) {
    Master_Out_Append_Short(Response_Bad_Packet);
    return 0;
  }
  uint8_t const length = bs[0];
  if (length > max_length) {
    Master_Out_Append_Short(Response_Bad_Packet);
    return 0;
  }

  if ((length == 3) &&
      memcmp((uint8_t const *)bs, Command_Reset, sizeof(Command_Reset))) {
    soft_reset();
    return length;
  }

  uint8_t const crc_in = bs[length - 1];
  uint8_t crc_local = 0;
  for (uint8_t i = 0; i < length; i++) {
    crc_local = crc8_p93_next(crc_local, bs[i]);
  }
  if (crc_in != crc_local) {
    Master_Out_Append_Short(Response_Invalid_CRC);
    return 0;
  }

  uint8_t const header = bs[1];
  uint8_t const data_length = length - 3;
  if (header == Command_Poll) {
    if (length == 3) {
      Master_Out_Append_Short(Response_Nothing);
    } else if (length == 4) {
      uint8_t const data[1] = {bs[2]};
      Master_Out_Append_Long(Response_Nothing, data, sizeof(data));
    } else {
      Master_Out_Append_Short(Response_Bad_Packet);
    }

    // TODO:
    // } else if (header == Command_MDB_Bus_Reset) {
  } else if ((header >= Command_MDB_Transaction_Low) &&
             (header <= Command_MDB_Transaction_High)) {
    if (!MDB_State_Idle) {
      Master_Out_Append_Short(Response_MDB_Busy);
      return length;
    }
    Master_Out_Append_Long(Response_Nothing, (uint8_t *)(&bs[2]), data_length);
    mdb_state = MDB_STATE_TX_DATA;

    // TODO:
    //} else if (cmd == Command_Config_Write) {
    // TODO
    //} else if (cmd == Command_Config_Read) {
    // TODO
  } else {
    Master_Out_Append_Short(Response_Unknown_Command);
  }
  return length;
}

static bool Step() {
  bool again = false;

  // TWI read is finished
  if (twi_idle && (twi_in.length > 0)) {
    if (twi_in.length == 1) {
      // keyboard sends 1 byte
      uint8_t const data[1] = {twi_in.data[2]};
      Master_Out_Append_Long(Response_TWI, data, sizeof(data));
      again = true;
    } else {
      // master sends >= 3 bytes
      uint8_t i = 0;
      uint8_t const *src = twi_in.data;
      for (;;) {
        uint8_t const consumed = Master_Command(src, twi_in.length - i);
        if (consumed == 0) {
          break;
        }
        i += consumed;
        src += consumed * sizeof(uint8_t);
        if (i >= twi_in.length) {
          break;
        }
      }
      again = true;
    }
    twi_in.length = twi_in.used = 0;
  }

  if (uart_error != 0) {
    Master_Out_Append_Long(uart_error, (uint8_t const *const)uart_debug,
                           sizeof(uart_debug));
    uart_error = 0;
  }
  again |= UART_Send_Loop(3);
  again |= UART_Recv_Loop(3);

  if (twi_idle) {
    while (TWI_Out_Append_From_Master_Out())
      ;
    again = true;
  }

  return again;
}

static void Step_Loop(int8_t const max_repeats) {
  for (int8_t i = 0; i < max_repeats; i++) {
    if (!Step()) {
      break;
    }
  }
}

#ifdef TEST
#include "main_test.c"
#else
int main(void) {
  cli();
  wdt_disable();
  Init();

  for (;;) {
    sei();

    while (!twi_idle)
      ;

    // sleep_mode();
    // while (!twi_idle) {
    //   sleep_mode();
    // }

    cli();

    Step_Loop(/*max =*/10);

    Master_Notify_Set((!twi_idle) || (twi_out.used < twi_out.length) ||
                      (buf_master_out.length >= 3));
  }
  return 0;
}
#endif

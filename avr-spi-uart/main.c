#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/sleep.h>
#include <avr/wdt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
#include <util/twi.h>

#include "crc.h"
#include "ring.h"

/*
TWI = two wire interface, exactly same as i2c.

This ATMega device speaks:
- TWI slave to the master (Raspberry Pi in our case)
- sends simple edge trigger to master to indicate data is ready
- TWI slave to keyboard hardware, stores data in buffer
- UART (9600-N-9), stores data in buffer

The master does:
- TWI write transaction to write to UART
- watch for edge on special pin
- TWI read transaction to read UART or keyboard data

Any TWI write or read transmits exactly 3 bytes:
- header
- data
- CRC8 poly=0x93 init=0x00 xorout=0x00 refin=false refout=false

Header bits: OK C1 C2 9B . . . .
- OK (1) error (0), master->slave error = reset
- C1,C2 status poll (00), UART data (01), TWI data (10), TWI set address (11)
- 9B data 9th bit for UART

Master sequences:
- 00 00 00 - flush buffers, reset state, all zeros intentional
- 80 00 74 - status poll
- 20 .. .. - send data byte to UART, 9bit 0
- 30 .. .. - send data byte to UART, 9bit 1
- 60 .. .. - set TWI slave address from data byte

Slave sequences:
- 01 00 00 - nothing to say, repeat later
- 00 .. .. - status error, details in data byte
- 80 00 74 - status OK, no data is buffered for reading
- 81 01 5f - hello after reset
- a0 .. .. - received byte from UART, 9bit 0
- b0 .. .. - received byte from UART, 9bit 1
- c0 .. .. - received byte from TWI
*/

static uint8_t const Header_OK = 0x80;           // bit 7 =1
static uint8_t const Header_Status = 0;          // bit 7 =0
static uint8_t const Header_TWI_Data = 0x40;     // bit 6
static uint8_t const Header_TWI_Address = 0x60;  // bit 6+5
static uint8_t const Header_UART_Data = 0x20;    // bit 5
static uint8_t const Header_9bit = 0x10;         // bit 4

static uint8_t const Error_CRC = 0x93;

static RingBuffer_t buf_twi_in;
static RingBuffer_t buf_twi_out;
static RingBuffer_t buf_uart_in;
static RingBuffer_t buf_uart_out;
static uint8_t volatile error_code = 0;

// Watchdog for software reset
void wdt_init(void) __attribute__((naked)) __attribute__((section(".init3")));
void wdt_init(void) {
  MCUSR = 0;
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

static uint8_t min_uint8(uint8_t const a, uint8_t const b) {
  return (a < b) ? a : b;
}

static bool ring_push3_with_crc(RingBuffer_t volatile* b, uint8_t const b1,
                                uint8_t const b2) {
  uint8_t const b3 = crc8_p93_2b(b1, b2);
  return Ring_PushTail3(b, b1, b2, b3);
}

static void LED_Set(bool const on) {
  if (on) {
    PORTB |= _BV(PINB5);
  } else {
    PORTB &= ~_BV(PINB5);
  }
}

static void LED_Init() {
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

// Begin TWI driver
static uint8_t const TWI_STATE_IDLE = 0;
static uint8_t const TWI_STATE_ST = 2;
static uint8_t const TWI_STATE_SR = 3;
static volatile uint8_t twi_state;
#define TWI_State_Idle (twi_state == TWI_STATE_IDLE)
#define TWI_State_Reading (twi_state == TWI_STATE_SR)
#define TWI_State_Writing (twi_state == TWI_STATE_ST)

static volatile uint8_t twi_ses_in[93];
static volatile uint8_t twi_ses_out[93];
#define TWI_In_Read twi_ses_in[1]
#define TWI_In_Done twi_ses_in[0]
#define TWI_In_Size (sizeof(twi_ses_in) - 2)
#define TWI_In_Next twi_ses_in[TWI_In_Read + 2]
#define TWI_Out_Have twi_ses_out[0]
#define TWI_Out_Sent twi_ses_out[1]
#define TWI_Out_Size (sizeof(twi_ses_out) - 2)
#define TWI_Out_Next twi_ses_out[TWI_Out_Sent + 2]

static void TWI_Init_Slave(uint8_t const address) {
  TWCR = 0;
  TWBR = 0x0c;
  TWAR = address << 1;
  TWSR = 0;
  twi_state = TWI_STATE_IDLE;
  TWI_In_Read = 0;
  TWI_In_Done = 0;
  TWI_Out_Have = 0;
  TWI_Out_Sent = 0;
  TWCR = _BV(TWINT) | _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
}

ISR(TWI_vect) {
  bool ack = false;
  switch (TW_STATUS) {
    case TW_NO_INFO:
      return;

    case TW_BUS_ERROR:
      TWI_In_Read = 0;
      TWI_In_Done = 0;
      TWI_Out_Sent = 0;
      TWCR = _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      twi_state = TWI_STATE_SR;
      TWI_In_Read = 0;
      TWI_In_Done = 0;
      ack = true;
      break;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      twi_state = TWI_STATE_SR;
      TWI_In_Next = TWDR;
      if (TWI_In_Read < TWI_In_Size) {
        TWI_In_Read++;
      }
      ack = true;
      break;

    // data received, NACK returned
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      twi_state = TWI_STATE_SR;
      ack = false;
      break;

    // Receive Stop or ReStart
    case TW_SR_STOP:
      twi_state = TWI_STATE_IDLE;
      TWI_In_Done = TWI_In_Read;
      ack = true;
      break;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      twi_state = TWI_STATE_ST;
      TWI_Out_Sent = 0;
      if (TWI_Out_Have == 0) {
        TWI_Out_Have = 3;
        twi_ses_out[2] = 1;
        twi_ses_out[3] = 0;
        twi_ses_out[4] = 0;
      }
      ack = (TWI_Out_Sent < TWI_Out_Have);
      if (ack) {
        TWDR = TWI_Out_Next;
      } else {
        TWDR = 0;
      }
      break;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      twi_state = TWI_STATE_ST;
      ack = (TWI_Out_Sent < TWI_Out_Have);
      if (ack) {
        TWI_Out_Sent++;
        TWDR = TWI_Out_Next;
      } else {
        TWDR = 0;
      }
      break;

    // Send Last Byte Receive ACK
    case TW_ST_LAST_DATA:
    // Send Last Byte Receive NACK
    case TW_ST_DATA_NACK:
      twi_state = TWI_STATE_IDLE;
      TWI_Out_Have = 0;
      TWI_Out_Sent = 0;
      ack = true;
      break;
  }
  TWCR = _BV(TWINT) | (ack ? _BV(TWEA) : 0) | _BV(TWEN) | _BV(TWIE);
}
// End TWI driver

// Begin UART driver
static void UART_Init() {
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
  // 1 stop bit
  UCSR0C |= _BV(USBS0);
  // 9 data bits
  UCSR0C |= _BV(UCSZ00) | _BV(UCSZ01);
}

static bool UART_Recv_Ready() { return bit_test(UCSR0A, _BV(RXC0)); }

static void UART_Recv() {
  uint8_t const srlow = UCSR0A;
  uint8_t const srhigh = UCSR0B;
  uint8_t const data = UDR0;
  uint8_t status = Header_UART_Data;
  if ((srlow & (_BV(FE0) | _BV(DOR0) | _BV(UPE0))) == 0) {
    status |= Header_OK;
    if ((srhigh & _BV(1)) != 0) {
      status |= Header_9bit;
    }
  }
  // TODO: check buffer push errors
  ring_push3_with_crc(&buf_twi_out, status, data);
}

static bool UART_Recv_Loop(int8_t const max_repeats) {
  bool activity = false;
  for (int8_t i = max_repeats; i >= 0; i--) {
    if (buf_twi_out.free < 3) {
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

ISR(USART_RX_vect) { UART_Recv_Loop(5); }

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
    if (!UART_Send_Ready()) {
      break;
    }
    if (buf_uart_out.length < 2) {
      break;
    }
    uint8_t header, data;
    if (!Ring_PopHead2(&buf_uart_out, &header, &data)) {
      break;
    }
    UART_Send_Byte(data, bit_test(header, Header_9bit));
    activity = true;
  }
  return activity;
}

ISR(USART_UDRE_vect) { UART_Send_Loop(5); }

ISR(USART_TX_vect) { UART_Send_Loop(5); }
// End UART driver

static void init() {
  LED_Init();
  Ring_Init(&buf_uart_in);
  Ring_Init(&buf_uart_out);
  Ring_Init(&buf_twi_in);
  Ring_Init(&buf_twi_out);
  TWI_Init_Slave(0x78);
  UART_Init();
  set_sleep_mode(SLEEP_MODE_IDLE);
  Master_Notify_Init();
  memset((void*)twi_ses_in, 0, sizeof(twi_ses_in));
  memset((void*)twi_ses_out, 0, sizeof(twi_ses_out));
  twi_state = TWI_STATE_IDLE;

  // disable ADC
  ADCSRA &= ~_BV(ADEN);
  // power reduction
  PRR |= _BV(PRTIM1) | _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);

  // hello after reset
  ring_push3_with_crc(&buf_twi_out, Header_OK | 0x01, 0x01);
}

static bool step() {
  bool again = false;

  // TWI read is finished
  if (TWI_State_Idle && (TWI_In_Read > 0)) {
    if (TWI_In_Read == 1) {
      // keyboard
      if (ring_push3_with_crc(&buf_twi_out, Header_OK | Header_TWI_Data,
                              twi_ses_in[2])) {
        memset((void*)twi_ses_in, 0, sizeof(twi_ses_in));
      }
      again = true;
    } else if (buf_twi_in.free >= TWI_In_Read) {
      // master
      for (uint8_t i = 0; i < TWI_In_Read; i++) {
        uint8_t const b = twi_ses_in[i + 2];
        Ring_PushTail(&buf_twi_in, b);
      }
      memset((void*)twi_ses_in, 0, sizeof(twi_ses_in));
      again = true;
    }
  }

  while (buf_twi_in.length >= 3) {
    again = true;
    uint8_t header, data, crc_in;
    Ring_PopHead3(&buf_twi_in, &header, &data, &crc_in);
    uint8_t const crc_local = crc8_p93_2b(header, data);
    if (crc_in != crc_local) {
      ring_push3_with_crc(&buf_twi_out, Header_TWI_Data, Error_CRC);
      continue;
    }
    if ((header == 0) && (data == 0) && (crc_in == 0)) {
      soft_reset();
      // init();
      return false;
    }
    uint8_t const cmd = header & (_BV(6) | _BV(5));
    if (cmd == Header_Status) {
      // TODO: handle error
      ring_push3_with_crc(&buf_twi_out, Header_OK, data + 1);
    } else if (cmd == Header_UART_Data) {
      // TODO: handle error
      Ring_PushTail2(&buf_uart_out, header, data);
      // while (!UART_Send_Ready()) ;
      // UART_Send_Byte(data, (header & Header_9bit) != 0);
      ring_push3_with_crc(&buf_twi_out, Header_OK | (header & Header_9bit),
                          data);
    } else if (cmd == Header_TWI_Address) {
      TWI_Init_Slave(data);
      ring_push3_with_crc(&buf_twi_out, Header_OK, data);
    } else {
      // Error: unknown command
      ring_push3_with_crc(&buf_twi_out, Header_TWI_Data | 1, header);
    }
  }

  again |= UART_Send_Loop(10);
  again |= UART_Recv_Loop(10);

  if (TWI_State_Idle && (buf_twi_out.length >= 3)) {
    uint8_t i, b = 0;
    uint8_t const len = min_uint8(buf_twi_out.length, TWI_Out_Size);
    memset((void*)twi_ses_out, 0, sizeof(twi_ses_out));
    for (i = 0; i < len; i++) {
      Ring_PopHead(&buf_twi_out, &b);
      twi_ses_out[i + 2] = b;
    }
    TWI_Out_Have = len;
    again = true;
  }

  return again;
}

int main(void) {
  wdt_disable();
  cli();
  init();

  for (;;) {
    sei();
    // sleep_mode();
    while (!TWI_State_Idle) {
      // sleep_mode();
    }
    cli();
    while (step())
      ;
    Master_Notify_Set((buf_twi_out.length >= 3) ||
                      (TWI_Out_Sent < TWI_Out_Have));
  }
  return 0;
}

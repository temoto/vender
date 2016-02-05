#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/sleep.h>
#include <compat/twi.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
#include <util/atomic.h>
#include <util/delay.h>

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
- TWI read transaction to read UART or TWI/I2C data

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
- a0 .. .. - send data byte to UART, 9bit 0
- b0 .. .. - send data byte to UART, 9bit 1
- e0 .. .. - set TWI/I2C slave address from data byte

Slave sequences:
- 00 .. .. - status error, details in data byte
- 80 00 74 - status OK, no data is buffered for reading
- a0 .. .. - received byte from UART, 9bit 0
- b0 .. .. - received byte from UART, 9bit 1
- c0 .. .. - received byte from TWI/I2C
*/

static uint8_t const Header_OK = _BV(7);
static uint8_t const Header_Status = 0 | 0;
static uint8_t const Header_UART_Data = 0 | _BV(5);
static uint8_t const Header_TWI_Data = _BV(6) | 0;
static uint8_t const Header_TWI_Address = _BV(6) | _BV(5);
static uint8_t const Header_9bit = _BV(4);

static uint8_t const Error_CRC = 0x93;

#define USART_BAUD 9600
#define USART_PRESCALE (((F_CPU) / (USART_BAUD * 16UL)) - 1)

static RingBuffer_t buf_twi_in;
static RingBuffer_t buf_twi_out;
static RingBuffer_t buf_uart_in;
static RingBuffer_t buf_uart_out;
static volatile uint8_t error_code = 0;

static uint8_t twi_ses_in[6];
static uint8_t twi_ses_out[6];
#define TWI_In_Read twi_ses_in[1]
#define TWI_In_Ack twi_ses_in[0]
#define TWI_In_Size (sizeof(twi_ses_in) - 2)
#define TWI_In_Next twi_ses_in[TWI_In_Read + 2]
#define TWI_Out_Have twi_ses_out[0]
#define TWI_Out_Sent twi_ses_out[1]
#define TWI_Out_Size (sizeof(twi_ses_out) - 2)
#define TWI_Out_Next twi_ses_out[TWI_Out_Sent + 2]

bool bit_test(uint8_t const x, uint8_t const mask) {
  return (x & mask) == mask;
}

bool ring_push3_with_crc(RingBuffer_t* const b, uint8_t const b1,
                         uint8_t const b2) {
  uint8_t const b3 = crc8_p93_2b(b1, b2);
  return Ring_PushTail3(b, b1, b2, b3);
}

void Master_Notify_Set(bool const on) {
  if (on) {
    PORTB |= _BV(PINB1);

    // led
    PORTB |= _BV(PINB5);
  } else {
    PORTB &= ~_BV(PINB1);

    // led
    PORTB &= ~_BV(PINB5);
  }
}

void Master_Notify_Init() {
  DDRB |= _BV(PINB1);
  Master_Notify_Set(false);
}

void TWI_Init_Slave(uint8_t address) {
  TWBR = 0x0c;
  TWCR =
      // _BV(TWINT) |
      _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
  TWAR = address << 1;
  TWSR = 0;
}

ISR(TWI_vect) {
  bool ack = false;
  switch (TW_STATUS) {
    case TW_NO_INFO:
      return;

    case TW_BUS_ERROR:
      TWCR = _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      memset(twi_ses_in, 0, sizeof(twi_ses_in));
      ack = true;
      break;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      TWI_In_Next = TWDR;
      if (TWI_In_Read < TWI_In_Size) {
        TWI_In_Read++;
      }
      ack = true;
      break;

    // data received, NACK returned
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      TWI_In_Next = TWDR;
      if (TWI_In_Read < TWI_In_Size) {
        TWI_In_Read++;
      }
      // memset(twi_ses_in, 0, sizeof(twi_ses_in));
      // TWI_In_Read = 0;
      ack = true;
      break;

    // Receive Stop or ReStart
    case TW_SR_STOP:
      TWI_In_Ack = 1;
      if (TWI_In_Read == 1) {
        // keyboard
        if (ring_push3_with_crc(&buf_twi_out, Header_OK | Header_TWI_Data,
                                twi_ses_in[2])) {
          memset(twi_ses_in, 0, sizeof(twi_ses_in));
        }
      }
      if (TWI_Out_Sent >= TWI_Out_Have) {
        memset(twi_ses_out, 0, sizeof(twi_ses_out));
      }
      ack = true;
      break;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      TWI_Out_Sent = 0;
      ack = (TWI_Out_Sent < TWI_Out_Have);
      if (ack) {
        TWDR = TWI_Out_Next;
      } else {
        TWDR = 0;
      }
      break;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
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
      if (TWI_Out_Sent < TWI_Out_Have) {
        TWI_Out_Sent++;
      }
      ack = true;
      break;

    // Send Last Byte Receive NACK
    case TW_ST_DATA_NACK:
      if (TWI_Out_Sent < TWI_Out_Have) {
        TWI_Out_Sent++;
      }
      ack = true;
      break;
  }
  TWCR = _BV(TWINT) | (ack ? _BV(TWEA) : 0) | _BV(TWEN) | _BV(TWIE);
}

void UART_Init() {
  UBRR0H = (uint8_t const)(USART_PRESCALE >> 8);
  UBRR0L = (uint8_t const)(USART_PRESCALE);

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

void UART_Send_Byte(uint8_t const b, bool const bit9) {
  if (bit9) {
    UCSR0B |= _BV(TXB80);
  } else {
    UCSR0B &= ~_BV(TXB80);
  }
  UDR0 = b;
}

bool UART_Send_Ready() { return bit_test(UCSR0A, _BV(UDRE0)); }
bool UART_Send_Done() { return bit_test(UCSR0A, _BV(TXC0)); }

void UART_Recv() {
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
  uint8_t const crc = crc8_p93_2b(status, data);
  // TODO: check buffer push errors
  Ring_PushTail3(&buf_uart_in, status, data, crc);
}

bool UART_Recv_Ready() { return bit_test(UCSR0A, _BV(RXC0)); }

bool step() {
  bool activity = false;

  while (buf_twi_in.length >= 3) {
    activity = true;
    uint8_t header, data, crc_in;
    Ring_PopHead3(&buf_twi_in, &header, &data, &crc_in);
    uint8_t const crc_check = crc8_p93_2b(header, data);
    if (crc_in != crc_check) {
      ring_push3_with_crc(&buf_twi_out, Header_TWI_Data, Error_CRC);
      continue;
    }
    // if (!bit_test(header, Header_OK)) {
    //   // TODO: reset everything
    //   break;
    // }
    uint8_t const cmd = header & (_BV(6) | _BV(5));
    uint8_t const crc_ok_0 = crc8_p93_2b(Header_OK, 0);
    if (cmd == Header_Status) {
      // TODO: handle error
      Ring_PushTail3(&buf_twi_out, Header_OK, data + 1, crc_ok_0);
    } else if (cmd == Header_UART_Data) {
      // TODO: handle error
      Ring_PushTail2(&buf_uart_out, header, data);
    } else if (cmd == Header_TWI_Address) {
      TWI_Init_Slave(data);
      // TODO: handle error
      Ring_PushTail3(&buf_twi_out, Header_OK, 0, crc_ok_0);
    }
  }

  while (UART_Send_Ready()) {
    uint8_t header, data;
    if (!Ring_PopHead2(&buf_uart_out, &header, &data)) {
      break;
    }
    UART_Send_Byte(data, bit_test(header, Header_9bit));
    activity = true;
  }
  while (UART_Recv_Ready() && (buf_uart_in.free >= 3)) {
    UART_Recv();
    activity = true;
  }
  while ((buf_uart_in.length >= 3) && (buf_twi_out.free >= 3)) {
    uint8_t b1, b2, b3;
    Ring_PopHead3(&buf_uart_in, &b1, &b2, &b3);
    Ring_PushTail3(&buf_twi_out, b1, b2, b3);
    activity = true;
  }

  if ((buf_twi_out.length >= 3) &&
      ((TWI_Out_Have == 0) || (TWI_Out_Sent >= TWI_Out_Have))) {
    uint8_t b1, b2, b3;
    if (Ring_PopHead3(&buf_twi_out, &b1, &b2, &b3)) {
      memset(twi_ses_out, 0, sizeof(twi_ses_out));
      TWI_Out_Have = 3;
      twi_ses_out[2] = b1;
      twi_ses_out[3] = b2;
      twi_ses_out[4] = b3;
    }
    activity = true;
  }
  Master_Notify_Set((buf_twi_out.length >= 3) || (TWI_Out_Sent < TWI_Out_Have));

  // If TWI session is finished and something was received, move data to
  // buf_twi_in
  if ((TWI_In_Read > 0) && (TWI_In_Ack == 1) &&
      buf_twi_in.free >= TWI_In_Read) {
    for (uint8_t i = 0; i < TWI_In_Read; i++) {
      uint8_t const b = twi_ses_in[i + 2];
      Ring_PushTail(&buf_twi_in, b);
    }
    memset(twi_ses_in, 0, sizeof(twi_ses_in));
    activity = true;
  }

  return activity;
}

int main(void) {
  cli();
  DDRB |= _BV(PORTB5);
  Ring_Init(&buf_uart_in);
  Ring_Init(&buf_uart_out);
  Ring_Init(&buf_twi_in);
  Ring_Init(&buf_twi_out);
  // TWI_Init_Slave(0x69);
  TWI_Init_Slave(0x78);
  UART_Init();
  set_sleep_mode(SLEEP_MODE_IDLE);
  sleep_enable();
  bool activity = false;
  Master_Notify_Init();
  memset(twi_ses_in, 0, sizeof(twi_ses_in));
  memset(twi_ses_out, 0, sizeof(twi_ses_out));
  sei();

  for (;;) {
    if (!activity) {
      sleep_mode();
    }
    ATOMIC_BLOCK(ATOMIC_RESTORESTATE) { activity = step(); }
  }
  return 0;
}

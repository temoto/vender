#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/sleep.h>
#include <compat/twi.h>
#include <inttypes.h>
#include <stdbool.h>
#include <util/atomic.h>
#include <util/delay.h>

#include "crc.h"
#include "ring.h"

/*
This ATMega device speaks:
- SPI slave to the master (Raspberry Pi in our case)
- sends simple edge trigger to master to indicate data is ready
- TWI/I2C slave to keyboard hardware, stores data in buffer
- UART (9600-N-9), stores data in buffer

The master does:
- SPI write transaction to write to UART
- watch for edge on special pin
- SPI read transaction to read UART or TWI/I2C data

Any SPI write or read transmits exactly 3 bytes:
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

#define USART_BAUD 9600
#define USART_PRESCALE (((F_CPU) / (USART_BAUD * 16UL)) - 1)

static RingBuffer_t buf_spi_in;
static RingBuffer_t buf_spi_out;
static RingBuffer_t buf_twi_in;
static RingBuffer_t buf_uart_in;
static RingBuffer_t buf_uart_out;
static volatile bool event_spi = false;
static volatile bool event_twi = false;
static volatile uint8_t error_code = 0;

bool bit_test(uint8_t const x, uint8_t const mask) {
  return (x & mask) == mask;
}

void Master_Notify_Init(){
  DDRB |= _BV(PINB2);
}

void Master_Notify_Set(bool const on) {
  if (on) {
    PORTB |= _BV(PINB2);
  } else {
    PORTB &= ~_BV(PINB2);
  }
}

void TWI_Init_Slave(uint8_t address) {
  DDRB |= _BV(PINB1);
  TWBR = 0x0c;
  TWCR = _BV(TWINT) | _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
  TWAR = address;
  TWSR = 0;
}

void TWI_Run(bool const ack) {
  if (ack) {
    TWCR = 1 << TWINT | 1 << TWEA | 1 << TWEN | 1 << TWIE;
  } else {
    TWCR = 1 << TWINT | 1 << TWEN | 1 << TWIE;
  }
}

ISR(TWI_vect) {
  bool ack = false;
  switch (TW_STATUS) {
    case TW_BUS_ERROR:
      TWCR = _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
      break;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    // Receive SLA+W broadcast
    case TW_SR_GCALL_ACK:
      ack = buf_spi_out.free >= 3;
      break;

    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      ack = Ring_PushTail(&buf_twi_in, TWDR);
      event_twi = true;
      break;

    // Receive Stop or ReStart
    case TW_SR_STOP:
      ack = true;
      break;
    // Receive SLA+R LP
    case TW_ST_ARB_LOST_SLA_ACK:
    // Receive SLA+R
    case TW_ST_SLA_ACK:
      ack = buf_spi_out.free >= 3;
      break;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      // have something to send?
      if (0) {
      } else {
        TWDR = 0;
        ack = false;
      }
      break;

    // Send Last Byte Receive NACK
    case TW_ST_DATA_NACK:
      break;
    // Send Last Byte Receive ACK
    case TW_ST_LAST_DATA:
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
  ATOMIC_BLOCK(ATOMIC_RESTORESTATE) {
    uint8_t const crc = crc8_p93_2b(status, data);
    // TODO: check buffer push errors
    Ring_PushTail3(&buf_uart_in, status, data, crc);
  }
}

bool UART_Recv_Ready() { return bit_test(UCSR0A, _BV(RXC0)); }

void SPI_Init_Slave(void) {
  DDRB |= _BV(PINB4);
  SPCR = _BV(SPE) | _BV(SPIE);
  SPDR = 0;
}

bool SPI_Ready(void) { return event_spi || bit_test(SPSR, _BV(SPIF)); }

void SPI_Send(uint8_t const b) {
  SPDR = b;
  Master_Notify_Set(true);
}

ISR(SPI_STC_vect) { event_spi = true; }

int main(void) {
  cli();

  uint8_t const crc_ok_0 = crc8_p93_2b(Header_OK, 0);

  Ring_Init(&buf_spi_in);
  Ring_Init(&buf_spi_out);
  Ring_Init(&buf_uart_in);
  Ring_Init(&buf_uart_out);
  Ring_Init(&buf_twi_in);
  SPI_Init_Slave();
  TWI_Init_Slave(0x69);
  UART_Init();
  set_sleep_mode(SLEEP_MODE_IDLE);
  sleep_enable();
  bool should_sleep = true;
  Master_Notify_Init();
  sei();

  Ring_PushTail3(&buf_spi_out, Header_OK, 0, crc_ok_0);
  for (;;) {
    if (should_sleep) {
      sleep_mode();
    }

    should_sleep = true;
    while (SPI_Ready()) {
      event_spi = false;
      uint8_t const in = SPDR;
      Ring_PushTail(&buf_spi_in, in);
      uint8_t out = 0;
      bool const again = Ring_PopHead(&buf_spi_out, &out);

      SPI_Send(out);
      Master_Notify_Set(again);
      should_sleep = false;
    }
    while (buf_spi_in.length >= 3) {
      uint8_t header, data, crc_in;
      Ring_PopHead3(&buf_spi_in, &header, &data, &crc_in);
      uint8_t const crc_check = crc8_p93_2b(header, data);
      if (crc_in != crc_check) {
        // TODO: handle CRC error
        continue;
      }
      if (!bit_test(header, Header_OK)) {
        // TODO: reset everything
        break;
      }
      uint8_t const cmd = header & (_BV(6) | _BV(5));
      if (cmd == Header_Status) {
        // TODO: handle error
        Ring_PushTail3(&buf_spi_out, Header_OK, 0, crc_ok_0);
      } else if (cmd == Header_UART_Data) {
        // TODO: handle error
        Ring_PushTail2(&buf_uart_out, header, data);
      } else if (cmd == Header_TWI_Address) {
        TWI_Init_Slave(data);
        // TODO: handle error
        Ring_PushTail3(&buf_spi_out, Header_OK, 0, crc_ok_0);
      }
      should_sleep = false;
    }

    while (buf_twi_in.length > 0) {
      event_twi = false;
      should_sleep = false;
      uint8_t data;
      if (!Ring_PopHead(&buf_twi_in, &data)) {
        break;
      }
      uint8_t const header = Header_OK | Header_TWI_Data;
      uint8_t const crc = crc8_p93_2b(header, data);
      if (!Ring_PushTail3(&buf_spi_out, header, data, crc)) {
        break;
      }
      Master_Notify_Set(true);
    }

    while (UART_Send_Ready()) {
      uint8_t header, data;
      if (!Ring_PopHead2(&buf_uart_out, &header, &data)) {
        break;
      }
      UART_Send_Byte(data, bit_test(header, Header_9bit));
      should_sleep = false;
    }
    while (UART_Recv_Ready() && (buf_uart_in.free >= 3)) {
      UART_Recv();
      Master_Notify_Set(true);
      should_sleep = false;
    }
    while ((buf_uart_in.length >= 3) && (buf_spi_out.free >= 3)) {
      uint8_t b;
      for (uint8_t i = 0; i < 3; i++) {
        // TODO: fatal error
        if (!Ring_PopHead(&buf_uart_in, &b)) {
          break;
        }
        if (!Ring_PushTail(&buf_spi_out, b)) {
          break;
        }
      }
      Master_Notify_Set(true);
      should_sleep = false;
    }
  }
  return 0;
}

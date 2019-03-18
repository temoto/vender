#ifndef _INCLUDE_FROM_MAIN
#error this file looks like standalone C source, but actually must be included in main.c
#endif
#ifndef INCLUDE_TWI_C
#define INCLUDE_TWI_C
#include <avr/interrupt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <util/twi.h>
#include "buffer.h"
#include "common.h"
#include "crc.h"

static uint8_t const TWCR_ACK = _BV(TWINT) | _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
static uint8_t const TWCR_NACK = _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
static uint8_t const TWCR_STOP =
    _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);

static bool volatile twi_idle = true;
static buffer_t volatile twi_in;
static uint8_t volatile _drop;

static void twi_init_slave(uint8_t const address) {
  static uint8_t twi_in_data[REQUEST_MAX_LENGTH];
  static uint8_t twi_out_data[RESPONSE_MAX_LENGTH];

  TWCR = 0;
  TWBR = 0;  // TWI slave does not control clock
  TWAR = address << 1;
  TWSR = 0;
  twi_idle = true;
  buffer_init(&twi_in, (uint8_t *)twi_in_data, sizeof(twi_in_data));
  buffer_init(&twi_out, (uint8_t *)twi_out_data, sizeof(twi_out_data));
  TWCR = TWCR_ACK;
}

static void twi_step(void) {
  if (!twi_idle) {
    return;
  }

  // TWI session is finished
  uint8_t const in_length = twi_in.length;  // anti-volatile
  if (in_length == 1) {
    if (response_empty()) {
      response_begin(0, RESPONSE_TWI_LISTEN);
    }
    // keyboard sends 1 byte, encode as 2 for future compatibility
    response_fn(FIELD_TWI_DATA, (uint8_t const[]){0, twi_in.data[0]}, 2);
    response_finish();
    buffer_clear_fast(&twi_in);
  } else if (in_length > 1) {
    // master sends >= 4 bytes
    // command is likely to generate response, wait until out buffer is empty
    if (response_empty()) {
      master_command(twi_in.data, in_length);
      buffer_clear_fast(&twi_in);
    }
  }
}

// Standard speed 100KHz = 160 clocks at F_CPU=16MHz.
// This is worst case interrupt handler CPU budget.
ISR(TWI_vect) {
  uint8_t data;
  uint8_t const st = TW_STATUS;
  static uint8_t out_idx;

  switch (st) {
    case TW_NO_INFO:
      return;

    case TW_BUS_ERROR:
      twi_idle = true;
      TWCR = TWCR_STOP;
      buffer_clear_fast(&twi_in);
      out_idx = 0;
      return;

    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      twi_idle = false;
      TWDR = 0;
      TWCR = TWCR_ACK;
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
      twi_idle = false;
      if (twi_in.length == 0) {
        TWCR = TWCR_ACK;
      } else {
        // unparsed request ready: deny write
        TWCR = TWCR_NACK;
      }
      return;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      twi_idle = false;
      data = TWDR;
      TWCR = TWCR_ACK;
      if (!buffer_append(&twi_in, data)) {
        // TODO respond error
      }
      return;

    // data received, NACK returned
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      twi_idle = false;
      _drop = TWDR;
      TWCR = TWCR_ACK;
      return;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      twi_idle = false;
      if (twi_out.length > 0) {
        TWDR = twi_out.data[0];
        TWCR = TWCR_ACK;
        out_idx = 1;
      } else {
        TWDR = 0;
        TWCR = TWCR_NACK;
        out_idx = 0;
      }
      return;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      twi_idle = false;
      if (out_idx < twi_out.length) {
        TWDR = twi_out.data[out_idx];
        TWCR = TWCR_ACK;
        out_idx++;
      } else {
        TWDR = 0;
        TWCR = TWCR_NACK;
        buffer_clear_fast(&twi_out);
      }
      return;

    // Send Last Byte Receive ACK
    // slave transmission completed successfully
    case TW_ST_LAST_DATA:
      twi_idle = true;
      TWCR = TWCR_ACK;
      buffer_clear_fast(&twi_out);
      return;

    // Send Last Byte Receive NACK
    // master buffer full, stop will not happen
    case TW_ST_DATA_NACK:
      twi_idle = true;
      TWCR = TWCR_ACK;
      return;

    // Receive Stop or ReStart
    case TW_SR_STOP:
      twi_idle = true;
      TWCR = TWCR_ACK;
      return;

    default:
      TWCR = TWCR_ACK;
  }
}

#endif  // INCLUDE_TWI_C

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

static buffer_t volatile twi_listen;

static void twi_init_slave(uint8_t const address) {
  TWCR = 0;
  TWBR = 0;  // TWI slave does not control clock
  TWAR = address << 1;
  TWSR = 0;
  buffer_init((buffer_t * const) & twi_listen);
  TWCR = TWCR_ACK;
}

static void twi_step(void) {
  uint8_t const length = twi_listen.length;  // anti-volatile
  if (length == 0) {
    return;
  }

  if (response_empty() && (mdb.state == MDB_STATE_IDLE)) {
    response_begin(RESPONSE_TWI_LISTEN);
    response_fn(FIELD_TWI_DATA, (uint8_t const* const)twi_listen.data, length);
    buffer_clear_fast((buffer_t * const) & twi_listen);
    response.filled = true;
  }
}

static inline void twi_flush_to_response(void) {
  uint8_t const length = twi_listen.length;  // anti-volatile
  if (length == 0) {
    return;
  }
  if (response.b.length + length + RESPONSE_RESERVED_FOR_ERROR <
      PACKET_FIELDS_MAX_LENGTH) {
    response_fn(FIELD_TWI_DATA, (uint8_t const* const)twi_listen.data, length);
    buffer_clear_fast((buffer_t * const) & twi_listen);
  }
}

// Standard speed 100KHz = 160 clocks at F_CPU=16MHz.
// This is worst case interrupt handler CPU budget.
ISR(TWI_vect) {
  uint8_t data;

  switch (TW_STATUS) {
    case TW_NO_INFO:
      return;

    case TW_BUS_ERROR:
      TWCR = TWCR_STOP;
      buffer_clear_fast((buffer_t * const) & twi_listen);
      return;

    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      TWDR = 0;
      TWCR = TWCR_ACK;
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
      TWCR = TWCR_ACK;
      return;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      data = TWDR;
      TWCR = TWCR_ACK;
      // keyboard sends 1 byte, encode as 2 for future compatibility
      buffer_append_2((buffer_t * const) & twi_listen, 0, data);
      return;

    // data received, NACK returned
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      (void)TWDR;
      TWCR = TWCR_ACK;
      return;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      TWDR = 0;
      TWCR = TWCR_ACK;
      return;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      TWDR = 0;
      TWCR = TWCR_NACK;
      return;

    // Send Last Byte Receive ACK
    // slave transmission completed successfully
    case TW_ST_DATA_NACK:
    // Send Last Byte Receive NACK
    // master buffer full, stop will not happen
    case TW_ST_LAST_DATA:
    // Receive Stop or ReStart
    case TW_SR_STOP:
      TWCR = TWCR_ACK;
      return;

    default:
      TWCR = TWCR_ACK;
  }
}

#endif  // INCLUDE_TWI_C

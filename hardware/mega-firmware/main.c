// Created: 18/06/2018 21:16:04
// Author : Alex, Temoto

#include "config.h"

#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/wdt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
#include <util/atomic.h>
#include <util/delay.h>
#include "bit.h"
#include "buffer.h"
#include "crc.h"
#include "protocol.h"

#define _INCLUDE_FROM_MAIN
#include "common.h"
#include "mdb.c"
#include "twi.c"

// fwd

static void cmd_status(uint8_t const request_id, uint8_t const *const data,
                       uint8_t const length) __attribute__((nonnull));
static void cmd_reset(uint8_t const request_id, uint8_t const *const data,
                      uint8_t const length) __attribute__((nonnull));
static void cmd_debug(uint8_t const request_id);
static void cmd_mdb_bus_reset(uint8_t const request_id,
                              uint8_t const *const data, uint8_t const length)
    __attribute__((nonnull));
static void cmd_mdb_tx_simple(uint8_t const request_id,
                              uint8_t const *const data, uint8_t const length)
    __attribute__((nonnull));
static void master_notify_init(void);
static void master_notify_set(bool const on);
static void clock_init(void);
static void clock_stop(void);
static void led_init(void);
static void led_toggle(void) __attribute__((used));
static void led_set(bool const on) __attribute__((used));

// Just .noinit is not enough for GCC 8.2
// https://github.com/technomancy/atreus/issues/34
uint8_t mcusr_saved __attribute__((section(".noinit,\"aw\",@nobits;")));
uint8_t reset_command_id __attribute__((section(".noinit,\"aw\",@nobits;")));
bool watchdog_expect __attribute__((section(".noinit,\"aw\",@nobits;")));

void early_init(void) __attribute__((naked, used, section(".init3")));
void early_init(void) {
  wdt_disable();
  cli();
  mcusr_saved = MCUSR;
  MCUSR = 0;
  if (bit_mask_test(mcusr_saved, _BV(WDRF)) && watchdog_expect) {
    bit_mask_clear(mcusr_saved, _BV(WDRF));
  } else {
    reset_command_id = 0;
  }
  watchdog_expect = false;
}

// Watchdog for software reset
static void soft_reset(void) __attribute__((noreturn));
static void soft_reset(void) {
  cli();
  watchdog_expect = true;
  for (;;)
    ;
}

static void nop(void) { __asm volatile("nop" ::); }

int main(void) {
  cli();
  wdt_enable(WDTO_30MS);
  wdt_reset();
  clock_stop();
  timer1_stop();

  clock_init();
  twi_init_slave(0x78);
  mdb_init();
  master_notify_init();
  led_init();
  // disable ADC
  bit_mask_clear(ADCSRA, _BV(ADEN));
  // power reduction
  PRR = _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);

  // hello after reset
  response_begin(reset_command_id, RESPONSE_RESET);
  response_f2(FIELD_FIRMWARE_VERSION, (FIRMWARE_VERSION >> 8),
              (FIRMWARE_VERSION & 0xff));
  response_f1(FIELD_MCUSR, mcusr_saved);
  response_finish();
  reset_command_id = 0;

  static uint8_t debugb_data[RESPONSE_MAX_LENGTH - 12];
  buffer_init(&debugb, debugb_data, sizeof(debugb_data));

  for (;;) {
    wdt_reset();

    cli();
    led_set(twi_out.length > 0);
    twi_step();
    master_notify_set(!response_empty());
    sei();
    nop();

    cli();
    if (mdb.state != MDB_STATE_IDLE) {
      mdb_step();
    }
    sei();
    nop();

    // TODO measure idle time
    _delay_us(300);
  }

  return 0;
}

static uint8_t master_command(uint8_t const *const bs,
                              uint8_t const max_length) {
  if (max_length < 4) {
    response_begin(0, RESPONSE_ERROR);
    response_f2(FIELD_ERROR2, ERROR_BAD_PACKET, max_length);
    response_fn(FIELD_ERRORN, bs, max_length);
    response_finish();
    return max_length;
  }
  uint8_t const length = bs[0];
  if (length > max_length) {
    response_begin(0, RESPONSE_ERROR);
    response_f2(FIELD_ERROR2, ERROR_BAD_PACKET, length);
    response_fn(FIELD_ERRORN, bs, length);
    response_finish();
    return length;
  }
  uint8_t const crc_in = bs[length - 1];
  uint8_t const crc_local = crc8_p93_n(0, bs, length - 1);
  if (crc_in != crc_local) {
    response_error2(0, ERROR_INVALID_CRC, crc_in);
    return length;
  }

  uint8_t const request_id = bs[1];
  if (request_id == 0) {
    response_error2(request_id, ERROR_INVALID_ID, 0);
    return length;
  }
  current_request_id = request_id;
  command_t const header = bs[2];
  uint8_t const data_length = length - 4;
  uint8_t const *const data = bs + 3;
  if (header == COMMAND_STATUS) {
    cmd_status(request_id, data, data_length);
  } else if (header == COMMAND_CONFIG) {
    mcusr_saved = 0;
    // TODO
    response_error2(request_id, ERROR_NOT_IMPLEMENTED, 0);
  } else if (header == COMMAND_RESET) {
    cmd_reset(request_id, data, data_length);
  } else if (header == COMMAND_DEBUG) {
    cmd_debug(request_id);
  } else if (header == COMMAND_FLASH) {
    // FIXME simulate bad watchdog event
    for (;;)
      ;
    // TODO
    response_error2(request_id, ERROR_NOT_IMPLEMENTED, 0);
  } else if (header == COMMAND_MDB_BUS_RESET) {
    cmd_mdb_bus_reset(request_id, data, data_length);
  } else if (header == COMMAND_MDB_TRANSACTION_SIMPLE) {
    cmd_mdb_tx_simple(request_id, data, data_length);
  } else if (header == COMMAND_MDB_TRANSACTION_CUSTOM) {
    // TODO read options from data: timeout, retry
    // then proceed as COMMAND_MDB_TRANSACTION_SIMPLE
    response_error2(request_id, ERROR_NOT_IMPLEMENTED, 0);
  } else {
    response_error2(request_id, ERROR_UNKNOWN_COMMAND, header);
  }
  return length;
}

static void cmd_status(uint8_t const request_id,
                       __attribute__((unused)) uint8_t const *const data,
                       uint8_t const length) {
  if (length != 0) {
    response_error2(request_id, ERROR_INVALID_DATA, 0);
    return;
  }

  response_begin(request_id, RESPONSE_OK);
  response_f2(FIELD_FIRMWARE_VERSION, (FIRMWARE_VERSION >> 8),
              (FIRMWARE_VERSION & 0xff));
  response_f1(FIELD_MCUSR, mcusr_saved);
  response_f1(FIELD_MDB_LENGTH, mdb_in.length);
  response_finish();
}

static void cmd_reset(uint8_t const request_id, uint8_t const *const data,
                      uint8_t const length) {
  if (length != 1) {
    response_error2(request_id, ERROR_INVALID_DATA, 0);
    return;
  }
  switch (data[0]) {
    case 0x01:
      mdb_reset();
      response_begin(request_id, RESPONSE_OK);
      response_f1(FIELD_MCUSR, mcusr_saved);
      response_finish();
      break;
    case 0xff:
      reset_command_id = request_id;
      soft_reset();  // noreturn
      break;
    default:
      response_error2(request_id, ERROR_INVALID_DATA, 1);
      break;
  }
}

static void cmd_debug(uint8_t const request_id) {
  response_begin(request_id, RESPONSE_OK);
  response_fn(FIELD_ERRORN, (void *)debugb.data, debugb.length);
  response_finish();
  buffer_clear_fast(&debugb);
}

static void cmd_mdb_bus_reset(uint8_t const request_id,
                              uint8_t const *const data, uint8_t const length) {
  if (length != 2) {
    response_error2(request_id, ERROR_INVALID_DATA, 0);
    return;
  }
  uint16_t const duration = (data[0] << 8) | data[1];
  mdb_bus_reset_begin(request_id, duration);
}

static void cmd_mdb_tx_simple(uint8_t const request_id,
                              uint8_t const *const data, uint8_t const length) {
  mdb_tx_begin(request_id, data, length);
}

static void master_notify_init(void) {
  bit_mask_set(MASTER_NOTIFY_DDR, _BV(MASTER_NOTIFY_PIN));
  master_notify_set(false);
}
static void master_notify_set(bool const on) {
  if (on) {
    bit_mask_set(MASTER_NOTIFY_PORT, _BV(MASTER_NOTIFY_PIN));
  } else {
    bit_mask_clear(MASTER_NOTIFY_PORT, _BV(MASTER_NOTIFY_PIN));
  }
}

static void led_init(void) { bit_mask_set(LED_DDR, _BV(LED_PIN)); }
static void led_toggle(void) { LED_PORT ^= _BV(LED_PIN); }
static void led_set(bool const on) {
  if (on) {
    bit_mask_set(LED_PORT, _BV(LED_PIN));
  } else {
    bit_mask_clear(LED_PORT, _BV(LED_PIN));
  }
}

static void clock_init(void) {
  _clock_10us = 0;
  // prescale 8, CTC, TOP=19 -> interrupt every 10us
  TCNT0 = 0;
  OCR0A = 19;
  TIMSK0 = _BV(OCIE0A);
  TCCR0A = _BV(WGM01);
  TCCR0B = _BV(CS01);
}
static void clock_stop(void) {
  TCCR0A = 0;
  TCCR0B = 0;
  TIMSK0 = 0;
}
ISR(TIMER0_COMPA_vect) {
  _clock_10us++;
  static uint8_t tmp10us = 0;
  static uint8_t tmp1ms = 0;
  if (++tmp10us == 100) {
    tmp10us = 0;
    if (++tmp1ms == 100) {
      tmp1ms = 0;
      _clock_100ms++;
      // cheap "every 1.6s"
      if ((_clock_100ms & 0xf) == 0) {
        // led_toggle();
      }
    }
  }
}

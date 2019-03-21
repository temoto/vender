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
#include "crc.c"
#include "mdb.c"
#include "spi.c"
#include "twi.c"

// fwd

static void request_exec(void);
static void cmd_status(void);
static void cmd_reset(void);
static void cmd_debug(void);
static void cmd_mdb_bus_reset(void);

// Just .noinit is not enough for GCC 8.2
// https://github.com/technomancy/atreus/issues/34
uint8_t mcusr_saved __attribute__((section(".noinit,\"aw\",@nobits;")));
bool watchdog_expect __attribute__((section(".noinit,\"aw\",@nobits;")));

void early_init(void) __attribute__((naked, used, section(".init3")));
void early_init(void) {
  wdt_disable();
  cli();
  mcusr_saved = MCUSR;
  MCUSR = 0;
  if (bit_mask_test(mcusr_saved, _BV(WDRF)) && watchdog_expect) {
    bit_mask_clear(mcusr_saved, _BV(WDRF));
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

int main(void) {
  cli();
  wdt_enable(WDTO_30MS);
  wdt_reset();
  clock_stop();
  timer1_stop();
  PCICR = 0;
  PCMSK0 = 0;

  clock_init();
  master_notify_init();
  led_init();
  buffer_init((buffer_t * const) & debugb);
  packet_clear_fast((packet_t * const) & request);
  packet_clear_fast((packet_t * const) & response);
  mdb_init();
  wdt_reset();
  twi_init_slave(0x78);
  spi_init_slave();
  // disable unused peripherals: ADC, Timer2
  bit_mask_clear(ADCSRA, _BV(ADEN));
  PRR = _BV(PRTIM2) | _BV(PRADC);

  // hello after reset
  response_begin(RESPONSE_RESET);
  response_f2(FIELD_FIRMWARE_VERSION, (FIRMWARE_VERSION >> 8),
              (FIRMWARE_VERSION & 0xff));
  response_f1(FIELD_MCUSR, mcusr_saved);
  response.filled = true;

  for (;;) {
    wdt_reset();

    cli();
    twi_step();
    sei();
    nop();

    cli();
    spi_step();
    sei();
    nop();

    cli();
    if (request.filled && response_empty()) {
      request_exec();
      if (!response_empty()) {
        response.filled = true;
      }
      packet_clear_fast((packet_t * const) & request);
    }
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

static void request_exec(void) {
  command_t const cmd = request.h.command;  // anti-volatile
  if (cmd == COMMAND_STATUS) {
    cmd_status();
  } else if (cmd == COMMAND_CONFIG) {
    mcusr_saved = 0;
    // TODO
    response_error2(ERROR_NOT_IMPLEMENTED, 0);
  } else if (cmd == COMMAND_RESET) {
    cmd_reset();
  } else if (cmd == COMMAND_DEBUG) {
    cmd_debug();
  } else if (cmd == COMMAND_FLASH) {
    // FIXME simulate bad watchdog event
    for (;;)
      ;
    // TODO
    response_error2(ERROR_NOT_IMPLEMENTED, 0);
  } else if (cmd == COMMAND_MDB_BUS_RESET) {
    cmd_mdb_bus_reset();
  } else if (cmd == COMMAND_MDB_TRANSACTION_SIMPLE) {
    mdb_tx_begin();
  } else if (cmd == COMMAND_MDB_TRANSACTION_CUSTOM) {
    // TODO read options from data: timeout, retry
    // then proceed as COMMAND_MDB_TRANSACTION_SIMPLE
    response_error2(ERROR_NOT_IMPLEMENTED, 0);
  } else {
    response_error2(ERROR_UNKNOWN_COMMAND, cmd);
  }
  return;
}

static void cmd_status(void) {
  if (request.b.length != 0) {
    response_error2(ERROR_INVALID_DATA, 0);
    return;
  }

  response_begin(RESPONSE_OK);
  response_f2(FIELD_FIRMWARE_VERSION, (FIRMWARE_VERSION >> 8),
              (FIRMWARE_VERSION & 0xff));
  response_f1(FIELD_MCUSR, mcusr_saved);
}

static void cmd_reset(void) {
  if (request.b.length != 1) {
    response_error2(ERROR_INVALID_DATA, 0);
    return;
  }
  switch (request.b.data[0]) {
    case 0x01:
      mdb_reset();
      response_begin(RESPONSE_OK);
      response_f1(FIELD_MCUSR, mcusr_saved);
      break;
    case 0xff:
      soft_reset();  // noreturn
      break;
    default:
      response_error2(ERROR_INVALID_DATA, 1);
      break;
  }
}

static void cmd_debug(void) {
  response_begin(RESPONSE_OK);
  response_fn(FIELD_ERRORN, (void *)debugb.data, debugb.length);
  buffer_clear_fast((buffer_t * const) & debugb);
}

static void cmd_mdb_bus_reset(void) {
  if (request.b.length != 2) {
    response_error2(ERROR_INVALID_DATA, 0);
    return;
  }
  uint16_t const duration = (request.b.data[0] << 8) | request.b.data[1];
  mdb_bus_reset_begin(duration);
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

static void led_init(void) {
#ifdef LED_CONFIGURED
  bit_mask_set(LED_DDR, _BV(LED_PIN));
#endif
}
static void led_toggle(void) {
#ifdef LED_CONFIGURED
  LED_PORT ^= _BV(LED_PIN);
#endif
}
static void led_set(bool const on) {
  if (on) {
#ifdef LED_CONFIGURED
    bit_mask_set(LED_PORT, _BV(LED_PIN));
#endif
  } else {
#ifdef LED_CONFIGURED
    bit_mask_clear(LED_PORT, _BV(LED_PIN));
#endif
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
#ifdef LED_CONFIGURED
  static uint8_t tmp10us = 0;
  static uint8_t tmp1ms = 0;
  if (++tmp10us == 100) {
    tmp10us = 0;
    if (++tmp1ms == 100) {
      tmp1ms = 0;
      _clock_100ms++;
      // cheap "every 1.6s"
      if ((_clock_100ms & 0xf) == 0) {
        led_toggle();
      }
    }
  }
#endif
}

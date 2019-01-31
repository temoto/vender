/*
 * Created: 18/06/2018 21:16:04
 * Author : Alex
 */

#define F_CPU 16000000UL  // Clock Speed

#include "main.h"
#include <avr/interrupt.h>
#include <avr/io.h>
#include <avr/wdt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
#include <util/atomic.h>
#include <util/delay.h>
#include "crc.h"

// must include .c after main.h
#include "buffer.c"
#include "mdb.c"
#include "twi.c"

// Just .noinit is not enough for GCC 8.2
// https://github.com/technomancy/atreus/issues/34 uint8_t mcu_status
// __attribute__((section(".noinit")));
uint8_t mcusr_saved __attribute__((section(".noinit,\"aw\",@nobits;")));
bool watchdog_expect __attribute__((section(".noinit,\"aw\",@nobits;")));

void early_init(void) __attribute__((naked, used, section(".init3")));
void early_init(void) {
  wdt_disable();
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
  timer0_stop();
  twi_init_slave(0x78);
  mdb_init();
  master_notify_init();
  // disable ADC
  bit_mask_clear(ADCSRA, _BV(ADEN));
  // power reduction
  PRR = _BV(PRTIM1) | _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);
  // hello after reset
  master_out_n(Response_Debug, Response_BeeBee, sizeof(Response_BeeBee));

  sei();

  for (;;) {
    wdt_reset();
    bool again = false;
    if (twi_idle) {
      again |= twi_step();  // may take 130us on FCPU=16MHz
    }
    if (mdb_state != MDB_State_Idle) {
      // cli(); // TODO double check if mdb_step() is safe with interrupts
      again |= mdb_step();
      // sei();
    }
    master_notify_set((!twi_idle) || (twi_out.used < twi_out.length) ||
                      (master_out.length > 0));
    if (!again) {
      // TODO measure idle time
      _delay_us(300);
    }
  }

  return 0;
}

static uint8_t master_command(uint8_t const *const bs,
                              uint8_t const max_length) {
  if (max_length < 3) {
    master_out_2(Response_Bad_Packet, 0);
    return max_length;
  }
  uint8_t const length = bs[0];
  if (length > max_length) {
    master_out_2(Response_Bad_Packet, 0);
    return length;
  }
  uint8_t const crc_in = bs[length - 1];
  uint8_t const crc_local = crc8_p93_n(0, bs, length - 1);

  if (crc_in != crc_local) {
    master_out_2(Response_Invalid_CRC, crc_in);
    return length;
  }

  Command_t const header = bs[1];
  uint8_t const data_length = length - 3;
  uint8_t const *const data = bs + 2;
  if (header == Command_Poll) {
    if (length != 3) {
      master_out_2(Response_Bad_Packet, 1);
      return length;
    }
  } else if (header == Command_Config) {
    mcusr_saved = 0;
    // TODO
    master_out_1(Response_Not_Implemented);
  } else if (header == Command_Reset) {
    soft_reset();  // noreturn
  } else if (header == Command_Debug) {
    uint8_t buf[40];
    memset(buf, 0, sizeof(buf));
    uint8_t *bufp = buf;
    *bufp++ = 'M';
    *bufp++ = 1;  // length of MDB status
    *bufp++ = mdb_state;
    *bufp++ = 'T';
    uint8_t const twi_stat_len = sizeof(TwiStat_t);
    *bufp++ = twi_stat_len;  // length of TWI status
    memcpy(bufp, (void *)&twi_stat, twi_stat_len);
    bufp += twi_stat_len;
    *bufp++ = 'U';
    *bufp++ = mcusr_saved;
    master_out_n(Response_Debug, (uint8_t *)buf, sizeof(buf));
  } else if (header == Command_Flash) {
    // FIXME simulate bad watchdog event
    for (;;)
      ;
    // TODO
    master_out_1(Response_Not_Implemented);
  } else if (header == Command_MDB_Bus_Reset) {
    if (data_length != 2) {
      master_out_1(Response_Bad_Packet);
      return length;
    }
    if (mdb_state != MDB_State_Idle) {
      master_out_1(Response_MDB_Busy);
      return length;
    }
    mdb_state = MDB_State_Bus_Reset;
    uint16_t const duration = (data[0] << 8) | data[1];
    bit_mask_clear(UCSR0B, _BV(TXEN0));  // disable UART TX
    bit_mask_set(DDRD, _BV(1));          // set TX pin to output
    bit_mask_clear(PORTD, _BV(PORTD1));  // pull TX pin low
    timer0_set((uint8_t)duration);
    master_out_1(Response_MDB_Started);
  } else if (header == Command_MDB_Transaction_Simple) {
    if (mdb_state != MDB_State_Idle) {
      master_out_1(Response_MDB_Busy);
      return length;
    }
    // TODO Buffer_Clear_Fast(&mdb_out)  after thorough testing
    Buffer_Clear_Full(&mdb_out);
    Buffer_AppendN(&mdb_out, data, data_length);
    Buffer_Append(&mdb_out, memsum(data, data_length));
    mdb_start_send();
    master_out_1(Response_MDB_Started);
  } else if (header == Command_MDB_Transaction_Custom) {
    // TODO read options from data: timeout, retry
    // then proceed as Command_MDB_Transaction_Simple
    master_out_1(Response_Not_Implemented);
  } else {
    master_out_1(Response_Unknown_Command);
  }
  return length;
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

static uint8_t memsum(uint8_t const *const src, uint8_t const length) {
  uint8_t sum = 0;
  for (uint8_t i = 0; i < length; i++) {
    sum += src[i];
  }
  return sum;
}

static void timer0_set(uint8_t const ms) {
  timer0_stop();
  uint8_t const per_ms = (F_CPU / 1024000UL);
  uint8_t const cnt = 256 - (ms * per_ms);
  TCNT0 = cnt;
  TIMSK0 = _BV(TOIE0);
  TCCR0B = _BV(CS02) | _BV(CS00);  // prescale 1024, normal mode
}
static void timer0_stop(void) {
  TCCR0B = 0;
  TCCR0A = 0;
  TIMSK0 = 0;
}

ISR(TIMER0_OVF_vect) {
  timer0_stop();
  if (mdb_state == MDB_State_Idle) {
    // FIXME something is wrong?
  } else if (mdb_state == MDB_State_Bus_Reset) {
    // MDB BUS BREAK is finished, re-enable UART TX
    bit_mask_set(UCSR0B, _BV(TXEN0));
  } else {
    // MDB timeout while sending or receiving
    // silence max 5ms
    // VMC ---ADD*---CHK------------ADD*---CHK------
    // Per --------------[silence]------------ACK*--
    uint8_t const time_passed = 5;  // FIXME get real value
    mdb_fast_error(Response_MDB_Timeout, time_passed);
  }
}

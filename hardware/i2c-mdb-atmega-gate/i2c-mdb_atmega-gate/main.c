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

static uint8_t volatile mcu_status;

void early_init(void) __attribute__((naked)) __attribute__((section(".init3")));
void early_init(void) {
  wdt_disable();
  mcu_status = MCUSR;
  MCUSR = 0;
}

// Watchdog for software reset
static void soft_reset(void) __attribute__((noreturn));
static void soft_reset(void) {
  cli();
  wdt_enable(WDTO_60MS);
  for (;;)
    ;
}

int main(void) {
  cli();
  wdt_disable();
  timer0_stop();
  twi_init_slave(0x78);
  mdb_init();
  // disable ADC
  bit_mask_clear(ADCSRA, _BV(ADEN));
  // power reduction
  PRR = _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);
  // hello after reset
  master_out_n(Response_Debug, Response_BeeBee, sizeof(Response_BeeBee));

  sei();

  for (;;) {
    bool again = false;
    if (twi_idle) {
      again |= twi_step();  // may take 130us on FCPU=16MHz
    }
    if (mdb_state != MDB_STATE_IDLE) {
      // cli(); // TODO double check if mdb_step() is safe with interrupts
      again |= mdb_step();
      // sei();
    }
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
    if (length == 3) {
      master_out_2(Response_Queue_Size, master_out.length);
    } else {
      master_out_2(Response_Bad_Packet, 1);
    }
  } else if (header == Command_Config) {
    // TODO
    master_out_1(Response_Not_Implemented);
  } else if (header == Command_Reset) {
    soft_reset();  // noreturn
  } else if (header == Command_Debug) {
    char *buf = "M=xT=xxxxxxxxxxxxxx";
    buf[2] = '0' + mdb_state;
    for (uint8_t i = 0; i < sizeof(TwiStat_t); i++) {
      shex(buf + 5 + (i * 2), *((uint8_t *)&twi_stat + i));
    }
    master_out_n(Response_Debug, (uint8_t *)buf, sizeof(buf));
  } else if (header == Command_Flash) {
    // TODO
    master_out_1(Response_Not_Implemented);
  } else if (header == Command_MDB_Bus_Reset) {
    if (data_length != 2) {
      master_out_1(Response_Bad_Packet);
      return length;
    }
    if (mdb_state != MDB_STATE_IDLE) {
      master_out_1(Response_MDB_Busy);
      return length;
    }
    mdb_state = MDB_STATE_BUS_RESET;
    uint16_t const duration = (data[0] << 8) | data[1];
    bit_mask_clear(UCSR0B, _BV(TXEN0));  // disable UART TX
    bit_mask_set(DDRD, _BV(1));          // set TX pin to output
    bit_mask_clear(PORTD, _BV(PORTD1));  // pull TX pin low
    timer0_set((uint8_t)duration);
    master_out_1(Response_MDB_Started);
  } else if (header == Command_MDB_Transaction_Simple) {
    if (mdb_state != MDB_STATE_IDLE) {
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

static uint8_t memsum(uint8_t const *const src, uint8_t const length) {
  uint8_t sum = 0;
  for (uint8_t i = 0; i < length; i++) {
    sum += src[i];
  }
  return sum;
}

// static uint8_t uint8_min(uint8_t const a, uint8_t const b) {
//   return a < b ? a : b;
// }

static void shex(char *dst, uint8_t const value) {
  uint8_t const alpha[16] = "0123456789abcdef";
  dst[0] = alpha[value & 0xf];
  dst[1] = alpha[(value >> 4) & 0xf];
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
  if (mdb_state == MDB_STATE_IDLE) {
    // FIXME something is wrong?
  } else if (mdb_state == MDB_STATE_BUS_RESET) {
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

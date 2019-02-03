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

int main(void) {
  wdt_enable(WDTO_30MS);
  wdt_reset();
  timer1_stop();
  twi_init_slave(0x78);
  mdb_init();
  master_notify_init();
  // disable ADC
  bit_mask_clear(ADCSRA, _BV(ADEN));
  // power reduction
  PRR = _BV(PRTIM2) | _BV(PRSPI) | _BV(PRADC);
  // hello after reset
  uint8_t const jr_data[] = {
      FIELD_PROTOCOL,
      PROTOCOL_VERSION,
      FIELD_FIRMWARE_VERSION,
      (FIRMWARE_VERSION >> 8),
      (FIRMWARE_VERSION & 0xff),
      FIELD_BEEBEE,
      0xbe,
      0xeb,
      0xee,
      FIELD_MCUSR,
      mcusr_saved,
  };
  master_out_n(reset_command_id, RESPONSE_JUST_RESET, jr_data, sizeof(jr_data));
  reset_command_id = 0;

  sei();

  for (;;) {
    wdt_reset();
    bool again = false;
    if (twi_idle) {
      again |= twi_step();  // may take 130us on F_CPU=16MHz
      cli();
      master_notify_set((twi_out.used < twi_out.length) ||
                        (master_out.length > 0));
      sei();
    }
    if (mdb.state != MDB_STATE_IDLE) {
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

// fwd
static void cmd_status(uint8_t const command_id, uint8_t const *const data,
                       uint8_t const length);
static void cmd_reset(uint8_t const command_id, uint8_t const *const data,
                      uint8_t const length);
static void cmd_debug(uint8_t const command_id);
static void cmd_mdb_bus_reset(uint8_t const command_id,
                              uint8_t const *const data, uint8_t const length);
static void cmd_mdb_tx_simple(uint8_t const command_id,
                              uint8_t const *const data, uint8_t const length);

static uint8_t master_command(uint8_t const *const bs,
                              uint8_t const max_length) {
  if (max_length < 4) {
    master_out_1(0, RESPONSE_BAD_PACKET, 0);
    return max_length;
  }
  uint8_t const length = bs[0];
  if (length > max_length) {
    master_out_1(0, RESPONSE_BAD_PACKET, 0);
    return length;
  }
  uint8_t const crc_in = bs[length - 1];
  uint8_t const crc_local = crc8_p93_n(0, bs, length - 1);
  if (crc_in != crc_local) {
    master_out_1(0, RESPONSE_INVALID_CRC, crc_in);
    return length;
  }

  uint8_t const command_id = bs[1];
  if (command_id == 0) {
    master_out_1(command_id, RESPONSE_INVALID_ID, 0);
    return length;
  }
  command_t const header = bs[2];
  uint8_t const data_length = length - 4;
  uint8_t const *const data = bs + 3;
  if (header == COMMAND_STATUS) {
    cmd_status(command_id, data, data_length);
  } else if (header == COMMAND_CONFIG) {
    mcusr_saved = 0;
    // TODO
    master_out_0(command_id, RESPONSE_NOT_IMPLEMENTED);
  } else if (header == COMMAND_RESET) {
    cmd_reset(command_id, data, data_length);
  } else if (header == COMMAND_DEBUG) {
    cmd_debug(command_id);
  } else if (header == COMMAND_FLASH) {
    // FIXME simulate bad watchdog event
    for (;;)
      ;
    // TODO
    master_out_0(command_id, RESPONSE_NOT_IMPLEMENTED);
  } else if (header == COMMAND_MDB_BUS_RESET) {
    cmd_mdb_bus_reset(command_id, data, data_length);
  } else if (header == COMMAND_MDB_TRANSACTION_SIMPLE) {
    cmd_mdb_tx_simple(command_id, data, data_length);
  } else if (header == COMMAND_MDB_TRANSACTION_CUSTOM) {
    // TODO read options from data: timeout, retry
    // then proceed as COMMAND_MDB_TRANSACTION_SIMPLE
    master_out_0(command_id, RESPONSE_NOT_IMPLEMENTED);
  } else {
    master_out_0(command_id, RESPONSE_UNKNOWN_COMMAND);
  }
  return length;
}
static void cmd_status(uint8_t const command_id,
                       __attribute__((unused)) uint8_t const *const data,
                       uint8_t const length) {
  if (length != 0) {
    master_out_1(command_id, RESPONSE_INVALID_DATA, 0);
    return;
  }
  uint8_t const buf[] = {
      FIELD_PROTOCOL,
      PROTOCOL_VERSION,
      FIELD_FIRMWARE_VERSION,
      (FIRMWARE_VERSION >> 8),
      (FIRMWARE_VERSION & 0xff),
      FIELD_MCUSR,
      mcusr_saved,
      FIELD_QUEUE_TWI,
      twi_out.length,
      FIELD_QUEUE_MASTER,
      master_out.length,
  };
  master_out_n(command_id, RESPONSE_STATUS, buf, sizeof(buf));
}
static void cmd_reset(uint8_t const command_id, uint8_t const *const data,
                      uint8_t const length) {
  if (length != 1) {
    master_out_1(command_id, RESPONSE_INVALID_DATA, 0);
    return;
  }
  switch (data[0]) {
    case 0x01:
      mdb_reset();
      memset((void *)&twi_stat, 0, sizeof(twi_stat_t));
      uint8_t const jr_data[] = {
          FIELD_PROTOCOL,
          PROTOCOL_VERSION,
          FIELD_FIRMWARE_VERSION,
          (FIRMWARE_VERSION >> 8),
          (FIRMWARE_VERSION & 0xff),
          FIELD_BEEBEE,
          0xbe,
          0xeb,
          0xee,
          FIELD_MCUSR,
          mcusr_saved,
      };
      master_out_n(command_id, RESPONSE_JUST_RESET, jr_data, sizeof(jr_data));
      break;
    case 0xff:
      reset_command_id = command_id;
      soft_reset();  // noreturn
      break;
    default:
      master_out_1(command_id, RESPONSE_INVALID_DATA, 1);
      break;
  }
}
static void cmd_debug(uint8_t const command_id) {
  static uint8_t buf[RESPONSE_MAX_LENGTH];
  memset(buf, 0, sizeof(buf));
  uint8_t *bufp = buf;
  *bufp++ = FIELD_MDB_PROTOTCOL_STATE;
  *bufp++ = mdb.state;
  *bufp++ = FIELD_MDB_STAT;
  uint8_t const mdb_stat_len = sizeof(mdb_stat_t);
  *bufp++ = mdb_stat_len;
  memcpy(bufp, (void *)&mdb_stat, mdb_stat_len);
  bufp += mdb_stat_len;
  *bufp++ = FIELD_TWI_STAT;
  uint8_t const twi_stat_len = sizeof(twi_stat_t);
  *bufp++ = twi_stat_len;
  memcpy(bufp, (void *)&twi_stat, twi_stat_len);
  bufp += twi_stat_len;
  master_out_n(command_id, RESPONSE_DEBUG, (uint8_t *)buf, bufp - buf);
}
static void cmd_mdb_bus_reset(uint8_t const command_id,
                              uint8_t const *const data, uint8_t const length) {
  if (length != 2) {
    master_out_1(command_id, RESPONSE_INVALID_DATA, 0);
    return;
  }
  uint8_t const mst = mdb.state;
  if (mst != MDB_STATE_IDLE) {
    master_out_1(command_id, RESPONSE_MDB_BUSY, mst);
    return;
  }
  uint16_t const duration = (data[0] << 8) | data[1];
  mdb_bus_reset_begin(command_id, duration);
}
static void cmd_mdb_tx_simple(uint8_t const command_id,
                              uint8_t const *const data, uint8_t const length) {
  if (length == 0) {
    master_out_1(command_id, RESPONSE_INVALID_DATA, 0);
    return;
  }
  if (length + 1 > mdb_out.size) {
    master_out_1(command_id, RESPONSE_BUFFER_OVERFLOW, length + 1);
    return;
  }
  uint8_t const mst = mdb.state;
  if (mst != MDB_STATE_IDLE) {
    master_out_1(command_id, RESPONSE_MDB_BUSY, mst);
    return;
  }
  buffer_copy(&mdb_out, data, length);
  buffer_append(&mdb_out, memsum(data, length));
  mdb_tx_begin(command_id);
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

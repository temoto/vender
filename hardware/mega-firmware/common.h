#ifndef INCLUDE_COMMON_H
#define INCLUDE_COMMON_H
#include "config.h"

#include <inttypes.h>
#include <stdbool.h>
#include "buffer.h"
#include "crc.h"
#include "protocol.h"

#if defined(LED_DDR) && defined(LED_PORT) && defined(LED_PIN)
#define LED_CONFIGURED
#endif

static packet_t volatile request;
static packet_t volatile response;
static buffer_t volatile debugb;
static uint16_t volatile _clock_10us;
static uint8_t volatile _clock_100ms;

static void clock_init(void);
static void clock_stop(void);
static inline uint16_t clock_10us(void);

static void master_notify_init(void);
static void master_notify_set(bool const on);
static void led_init(void);
static void led_toggle(void) __attribute__((used));
static void led_set(bool const on) __attribute__((used));

static void mdb_init(void);
static void mdb_step(void);
static void mdb_tx_begin(void);
static void mdb_bus_reset_begin(uint16_t const duration);
static void mdb_reset(void);
static void timer1_set(uint16_t const ticks);
static void timer1_stop(void);

static void spi_init_slave(void);
static void spi_step(void);

static void twi_init_slave(uint8_t const address);
static void twi_step(void);
static void nop(void) __attribute__((used, always_inline));

// fwd

static bool response_empty(void);
static bool response_check_capacity(uint8_t const more);
static void response_begin(response_t const header);
static void response_error2(errcode_t const ec, uint8_t const arg)
    __attribute__((used));
static void response_f1(field_t const f, uint8_t const data)
    __attribute__((used));
static void response_f2(field_t const f, uint8_t const d1, uint8_t const d2)
    __attribute__((used));
static void response_fn(field_t const f, uint8_t const *const data,
                        uint8_t const length) __attribute__((nonnull, used));

// inline

static inline void debug2(uint8_t const b1, uint8_t const b2) {
  buffer_append((buffer_t *)&debugb, b1);
  buffer_append((buffer_t *)&debugb, b2);
}
static inline void debugn(uint8_t const *const data, uint8_t const length) {
  buffer_append_n((buffer_t *)&debugb, data, length);
}
static inline void debugs(char const *const s) {
  buffer_append_n((buffer_t *)&debugb, (uint8_t const *const)s, strlen(s));
}

static inline bool response_empty(void) { return response.h.response == 0; }

static bool response_check_capacity(uint8_t const more) {
  uint8_t const RESERVED_FOR_ERROR = 5;
  // FIXME length check in buffer_append_n is wasted
  if (response.b.length + more + RESERVED_FOR_ERROR >
      PACKET_FIELDS_MAX_LENGTH) {
    buffer_append((buffer_t *)&response.b, FIELD_ERROR2);
    buffer_append_2((buffer_t *)&response.b, ERROR_BUFFER_OVERFLOW, more);
    response.filled = true;
    return false;
  }
  return true;
}

static void response_begin(response_t const kind) {
  if (response.h.response == 0) {
    response.h.response = kind;
    uint16_t const clk = clock_10us();
    response_f2(FIELD_CLOCK10U, (clk >> 8), (clk & 0xff));
  }
}

static void response_error2(errcode_t const ec, uint8_t const arg) {
  response_begin(RESPONSE_ERROR);
  response_f2(FIELD_ERROR2, ec, arg);
}

static void response_f1(field_t const f, uint8_t const data) {
  if (!response_check_capacity(2)) {
    return;
  }
  buffer_append_2((buffer_t *)&response.b, f, data);
}
static void response_f2(field_t const f, uint8_t const d1, uint8_t const d2) {
  if (!response_check_capacity(3)) {
    return;
  }
  buffer_append((buffer_t *)&response.b, f);
  buffer_append_2((buffer_t *)&response.b, d1, d2);
}
static void response_fn(field_t const f, uint8_t const *const data,
                        uint8_t const length) {
  if (!response_check_capacity(2 + length)) {
    return;
  }
  buffer_append_2((buffer_t *)&response.b, f, length);
  buffer_append_n((buffer_t *)&response.b, data, length);
}

static inline uint16_t clock_10us(void) __attribute__((warn_unused_result));
static inline uint16_t clock_10us(void) { return _clock_10us; }

static inline void nop(void) { __asm volatile("nop" ::); }

#endif  // INCLUDE_COMMON_H

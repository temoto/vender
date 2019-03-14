#ifndef INCLUDE_COMMON_H
#define INCLUDE_COMMON_H
#include "config.h"

#include <inttypes.h>
#include <stdbool.h>
#include "buffer.h"
#include "crc.h"
#include "protocol.h"

static uint8_t volatile current_request_id;
static buffer_t volatile twi_listen;
static buffer_t volatile twi_out;
static buffer_t volatile debugb;
static uint16_t volatile _clock_10us;
static uint8_t volatile _clock_100ms;
static uint16_t volatile _clock_idle_total;
static uint16_t volatile clock_idle;

static uint8_t master_command(uint8_t const *const bs, uint8_t const max_length)
    __attribute__((hot, nonnull));
static inline uint16_t clock_10us(void) __attribute__((hot));

static void mdb_init(void);
static bool mdb_step(void) __attribute__((hot));
static void mdb_tx_begin(uint8_t const request_id, uint8_t const *const data,
                         uint8_t const length) __attribute__((nonnull));
static void mdb_bus_reset_begin(uint8_t const request_id,
                                uint16_t const duration);
static void mdb_reset(void);
static void timer1_stop(void) __attribute__((hot));

static void twi_init_slave(uint8_t const address);
static bool twi_step(void) __attribute__((hot));

// fwd

static void response_ensure_non_empty() __attribute__((hot));
static bool response_check_capacity(uint8_t const more) __attribute__((hot));
static bool response_empty(void) __attribute__((hot, warn_unused_result));
static void response_begin(uint8_t const request_id, response_t const header)
    __attribute__((hot));
static void response_finish(void) __attribute__((hot));
static void response_error2(uint8_t const request_id, errcode_t const ec,
                            uint8_t const arg) __attribute__((hot, used));
static void response_f0(field_t const f) __attribute__((hot, used));
static void response_f1(field_t const f, uint8_t const data)
    __attribute__((hot, used));
static void response_f2(field_t const f, uint8_t const d1, uint8_t const d2)
    __attribute__((hot, used));
static void response_fn(field_t const f, uint8_t const *const data,
                        uint8_t const length)
    __attribute__((hot, nonnull, used));

// inline

static inline void debug2(uint8_t const b1, uint8_t const b2) {
  buffer_append(&debugb, b1);
  buffer_append(&debugb, b2);
}
static inline void debugn(uint8_t const *const data, uint8_t const length) {
  buffer_append_n(&debugb, data, length);
}
static inline void debugs(char const *const s) {
  buffer_append_n(&debugb, (uint8_t const *const)s, strlen(s));
}

static void response_ensure_non_empty(void) {
  if (twi_out.length == 0) {
    uint8_t const b[] = {0,
                         current_request_id,
                         RESPONSE_ERROR,
                         FIELD_ERROR2,
                         ERROR_RESPONSE_EMPTY,
                         0};
    buffer_copy(&twi_out, b, sizeof(b));
  }
}
static bool response_check_capacity(uint8_t const more) {
  uint8_t const RESERVED_FOR_ERROR = 5;
  // FIXME length check in buffer_append_n is wasted
  if (twi_out.length + more + RESERVED_FOR_ERROR > twi_out.size) {
    uint8_t const b[] = {FIELD_ERROR2, ERROR_BUFFER_OVERFLOW, more};
    buffer_append_n(&twi_out, b, sizeof(b));
    response_finish();
    return false;
  }
  return true;
}

static inline bool response_empty(void) { return twi_out.length == 0; }

static void response_begin(uint8_t const request_id, response_t const header) {
  uint8_t const b[] = {0, request_id, header};
  uint16_t const clk = clock_10us();
  buffer_copy(&twi_out, b, sizeof(b));
  response_f1(FIELD_PROTOCOL, PROTOCOL_VERSION);
  response_f2(FIELD_CLOCK10U, (clk >> 8), (clk & 0xff));
}

static void response_finish(void) {
  response_ensure_non_empty();
  twi_out.data[0] = twi_out.length + 1;
  uint8_t const crc = crc8_p93_n(0, twi_out.data, twi_out.length);
  buffer_append(&twi_out, crc);
  current_request_id = 0;
}

static void response_error2(uint8_t const request_id, errcode_t const ec,
                            uint8_t const arg) {
  if (response_empty()) {
    response_begin(request_id, RESPONSE_ERROR);
  } else {
    twi_out.length = twi_out.data[0];
  }
  response_f2(FIELD_ERROR2, ec, arg);
  response_finish();
}

static void response_f0(field_t const f) {
  response_ensure_non_empty();
  if (!response_check_capacity(1)) {
    return;
  }
  buffer_append(&twi_out, f);
}
static void response_f1(field_t const f, uint8_t const data) {
  response_ensure_non_empty();
  if (!response_check_capacity(2)) {
    return;
  }
  buffer_append(&twi_out, f);
  buffer_append(&twi_out, data);
}
static void response_f2(field_t const f, uint8_t const d1, uint8_t const d2) {
  response_ensure_non_empty();
  if (!response_check_capacity(3)) {
    return;
  }
  buffer_append(&twi_out, f);
  buffer_append(&twi_out, d1);
  buffer_append(&twi_out, d2);
}
static void response_fn(field_t const f, uint8_t const *const data,
                        uint8_t const length) {
  response_ensure_non_empty();
  if (!response_check_capacity(2 + length)) {
    return;
  }
  buffer_append(&twi_out, f);
  buffer_append(&twi_out, length);
  buffer_append_n(&twi_out, data, length);
}

static inline uint8_t memsum(uint8_t const *const src, uint8_t const length)
    __attribute__((nonnull, pure));
static inline uint8_t memsum(uint8_t const *const src, uint8_t const length) {
  uint8_t sum = 0;
  for (uint8_t i = 0; i < length; i++) {
    sum += src[i];
  }
  return sum;
}

static inline uint16_t clock_10us(void) __attribute__((warn_unused_result));
static inline uint16_t clock_10us(void) { return _clock_10us; }

#endif  // INCLUDE_COMMON_H

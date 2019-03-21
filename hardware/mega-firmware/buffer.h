#ifndef INCLUDE_BUFFER_H
#define INCLUDE_BUFFER_H
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>

typedef struct buffer_t {
  uint8_t length;
  uint8_t data[BUFFER_SIZE];
} buffer_t;

static inline void buffer_clear_fast(buffer_t *const b) { b->length = 0; }

static void buffer_clear_full(buffer_t *const b) {
  memset(b->data, 0, BUFFER_SIZE);
  buffer_clear_fast(b);
}

static void buffer_init(buffer_t *const b) __attribute__((used));
static void buffer_init(buffer_t *const b) { buffer_clear_full(b); }

static inline bool buffer_append(buffer_t *const b, uint8_t const data) {
  uint8_t const len = b->length;
  if (len + 1 > BUFFER_SIZE) {
    return false;
  }
  b->data[len] = data;
  b->length = len + 1;
  return true;
}

static inline bool buffer_append_2(buffer_t *const b, uint8_t const data1,
                                   uint8_t const data2) {
  uint8_t const len = b->length;
  if (len + 2 > BUFFER_SIZE) {
    return false;
  }
  b->data[len] = data1;
  b->data[len + 1] = data2;
  b->length = len + 2;
  return true;
}

static inline bool buffer_append_n(buffer_t *const b, uint8_t const *const src,
                                   uint8_t const n) {
  uint8_t const len = b->length;
  if (len + n > BUFFER_SIZE) {
    return false;
  }
  memcpy(b->data + len, src, n);
  b->length = len + n;
  return true;
}

static inline bool buffer_copy(buffer_t *const b, uint8_t const *const src,
                               uint8_t const n) {
  if (n > BUFFER_SIZE) {
    return false;
  }
  memcpy(b->data, src, n);
  b->length = n;
  return true;
}

#endif  // INCLUDE_BUFFER_H

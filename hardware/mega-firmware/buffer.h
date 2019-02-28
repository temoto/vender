#ifndef INCLUDE_BUFFER_H
#define INCLUDE_BUFFER_H
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>

typedef struct buffer_t {
  uint8_t size;
  uint8_t length;  // stored
  uint8_t used;    // read/processed/sent/etc
  uint8_t *data;
} volatile buffer_t;

static inline void buffer_clear_fast(buffer_t *const b) {
  b->length = 0;
  b->used = 0;
}

static void buffer_clear_full(buffer_t *const b) {
  memset(b->data, 0, b->size);
  buffer_clear_fast(b);
}

static void buffer_init(buffer_t *const b, uint8_t *const storage,
                        uint8_t const size) __attribute__((used));
static void buffer_init(buffer_t *const b, uint8_t *const storage,
                        uint8_t const size) {
  b->size = size;
  b->data = storage;
  buffer_clear_full(b);
}

static inline bool buffer_append(buffer_t *const b, uint8_t const data) {
  uint8_t const len = b->length;
  if (len + 1 > b->size) {
    return false;
  }
  b->data[len] = data;
  b->length = len + 1;
  return true;
}

static inline bool buffer_append_n(buffer_t *const b, uint8_t const *const src,
                                   uint8_t const n) {
  uint8_t const len = b->length;
  if (len + n > b->size) {
    return false;
  }
  memcpy(b->data + len, src, n);
  b->length = len + n;
  return true;
}

static inline bool buffer_copy(buffer_t *const b, uint8_t const *const src,
                               uint8_t const n) {
  uint8_t const size = b->size;  // anti-volatile
  uint8_t const len = (n < size ? n : size);
  memcpy(b->data, src, len);
  b->length = len;
  return true;
}

#endif  // INCLUDE_BUFFER_H

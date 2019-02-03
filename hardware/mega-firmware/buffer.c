#ifndef INCLUDE_BUFFER_C
#define INCLUDE_BUFFER_C
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>

typedef struct {
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
                        uint8_t const size) {
  b->size = size;
  b->data = storage;
  buffer_clear_full(b);
}

static bool buffer_append(buffer_t *const b, uint8_t const data) {
  if (b->length + 1 > b->size) {
    return false;
  }
  b->data[b->length] = data;
  b->length++;
  return true;
}

static bool buffer_append_n(buffer_t *const b, uint8_t const *const src,
                            uint8_t const n) {
  if (b->length + n > b->size) {
    return false;
  }
  memcpy(b->data + b->length, src, n);
  b->length += n;
  return true;
}

static bool buffer_copy(buffer_t *const b, uint8_t const *const src,
                        uint8_t const n) {
  uint8_t const len = (n < b->size ? n : b->size);
  memcpy(b->data, src, len);
  b->length = len;
  return true;
}

static void buffer_swap(buffer_t *const b1, buffer_t *const b2) {
  buffer_t tmp;
  memcpy((void *)&tmp, (void const *)b1, sizeof(buffer_t));
  memcpy((void *)b1, (void const *)b2, sizeof(buffer_t));
  memcpy((void *)b2, (void const *)&tmp, sizeof(buffer_t));
}

#endif  // INCLUDE_BUFFER_C

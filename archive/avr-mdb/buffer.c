#ifndef INCLUDE_BUFFER_C
#define INCLUDE_BUFFER_C
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>

typedef struct {
  uint8_t size;
  uint8_t length;  // stored
  uint8_t used;    // read/processed/sent/etc
  uint8_t free;
  uint8_t *data;
} volatile Buffer_t;

static void Buffer_Clear(Buffer_t *const b) {
  uint8_t zero_length = b->length + 1;
  if (zero_length > b->size) {
    zero_length = b->size;
  }
  memset(b->data, 0, zero_length);
  b->length = 0;
  b->used = 0;
  b->free = b->size;
}

static void Buffer_Init(Buffer_t *const b, uint8_t *const storage,
                        uint8_t const size) {
  b->size = size;
  b->data = storage;
  Buffer_Clear(b);
}

static bool Buffer_Append(Buffer_t *const b, uint8_t const data) {
  if (b->free < 1) {
    return false;
  }
  b->data[b->length] = data;
  b->length++;
  b->free--;
  return true;
}

static bool Buffer_AppendN(Buffer_t *const b, uint8_t const *const src,
                           uint8_t const n) {
  if (b->free < n) {
    return false;
  }
  memcpy(b->data + b->length, src, n);
  b->length += n;
  b->free -= n;
  return true;
}

static void Buffer_Swap(Buffer_t *const b1, Buffer_t *const b2) {
  Buffer_t tmp;
  memcpy((void *)&tmp, (void const *)b1, sizeof(Buffer_t));
  memcpy((void *)b1, (void const *)b2, sizeof(Buffer_t));
  memcpy((void *)b2, (void const *)&tmp, sizeof(Buffer_t));
}

#endif  // INCLUDE_BUFFER_C

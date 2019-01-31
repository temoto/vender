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
} volatile Buffer_t;

static inline void Buffer_Clear_Fast(Buffer_t *const b) {
  b->length = 0;
  b->used = 0;
}

static void Buffer_Clear_Full(Buffer_t *const b) {
  memset(b->data, 0, b->size);
  Buffer_Clear_Fast(b);
}

static void Buffer_Init(Buffer_t *const b, uint8_t *const storage,
                        uint8_t const size) {
  b->size = size;
  b->data = storage;
  Buffer_Clear_Full(b);
}

static bool Buffer_Append(Buffer_t *const b, uint8_t const data) {
  if (b->length + 1 > b->size) {
    return false;
  }
  b->data[b->length] = data;
  b->length++;
  return true;
}

static bool Buffer_AppendN(Buffer_t *const b, uint8_t const *const src,
                           uint8_t const n) {
  if (b->length + n > b->size) {
    return false;
  }
  memcpy(b->data + b->length, src, n);
  b->length += n;
  return true;
}

static bool Buffer_Copy(Buffer_t *const b, uint8_t const *const src,
                        uint8_t const n) {
  uint8_t const len = (n < b->size ? n : b->size);
  memcpy(b->data, src, len);
  b->length = len;
  return true;
}

static void Buffer_Swap(Buffer_t *const b1, Buffer_t *const b2) {
  Buffer_t tmp;
  memcpy((void *)&tmp, (void const *)b1, sizeof(Buffer_t));
  memcpy((void *)b1, (void const *)b2, sizeof(Buffer_t));
  memcpy((void *)b2, (void const *)&tmp, sizeof(Buffer_t));
}

#endif  // INCLUDE_BUFFER_C

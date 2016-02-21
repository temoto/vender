#ifndef INCLUDE_BUFFER_C
#define INCLUDE_BUFFER_C
#include <inttypes.h>

typedef struct {
  uint8_t size;
  uint8_t length;
  uint8_t used;
  uint8_t *data;
} volatile Buffer_t;

static void Buffer_Init(Buffer_t *const b, uint8_t *const storage,
                        uint8_t const size) {
  b->size = size;
  b->length = 0;
  b->used = 0;
  b->data = storage;
}

static bool Buffer_Append(Buffer_t *const b, uint8_t data) {
  if (b->length >= b->size) {
    return false;
  }
  b->data[b->length] = data;
  b->length++;
  return true;
}

#endif  // INCLUDE_BUFFER_C

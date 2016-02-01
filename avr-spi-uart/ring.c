#include <stdio.h>   // printf, only for debug
#include <string.h>  // memset, required
#include "ring.h"

#ifdef DEBUG
#define debug_printf printf
#else
void debug_printf(char* const msg, ...) {}
#endif

void Ring_Init(RingBuffer_t* const b) {
  memset(b->data, 0, RING_BUFFER_SIZE);
  b->head = 0;
  b->tail = 0;
  b->length = 0;
  b->free = RING_BUFFER_SIZE;
}

bool Ring_PushTail(RingBuffer_t* const b, uint8_t const value) {
  if ((b->free == 0) && (b->tail == b->head)) {
    debug_printf("ring push(%c)  ERR full\n", value);
    return false;
  }
  uint8_t next = (b->tail + 1) % RING_BUFFER_SIZE;
  b->data[b->tail] = value;
  b->tail = next;
  b->length++;
  b->free--;
  debug_printf("ring push(%c)  head=%d tail=%d next=%d length=%d free=%d\n",
               value, b->head, b->tail, next, b->length, b->free);
  return true;
}

bool Ring_PushTail2(RingBuffer_t* const b, uint8_t const v1, uint8_t const v2) {
  if (b->free < 2) {
    return false;
  }
  if (!Ring_PushTail(b, v1)) {
    return false;
  }
  if (!Ring_PushTail(b, v2)) {
    return false;
  }
  return true;
}

bool Ring_PushTail3(RingBuffer_t* const b, uint8_t const v1, uint8_t const v2,
                    uint8_t const v3) {
  if (b->free < 3) {
    return false;
  }
  if (!Ring_PushTail(b, v1)) {
    return false;
  }
  if (!Ring_PushTail(b, v2)) {
    return false;
  }
  if (!Ring_PushTail(b, v3)) {
    return false;
  }
  return true;
}

bool Ring_PeekHead(RingBuffer_t* const b, uint8_t* const out) {
  if (b->length == 0) {
    debug_printf("ring peek     ERR empty head=%d tail=%d length=%d\n", b->head,
                 b->tail, b->length);
    return false;
  }
  if (out != NULL) {
    *out = b->data[b->head];
  }
  debug_printf("ring peek     out=%c head=%d length=%d\n", *out, b->head,
               b->length);
  return true;
}

bool Ring_PopHead(RingBuffer_t* const b, uint8_t* const out) {
  if (!Ring_PeekHead(b, out)) {
    return false;
  }
  // b->data[b->head] = 0;
  b->head = (b->head + 1) % RING_BUFFER_SIZE;
  b->length--;
  b->free++;
  debug_printf("ring pop      out=%c head=%d tail=%d length=%d free=%d\n", *out,
               b->head, b->tail, b->length, b->free);
  return true;
}

bool Ring_PopHead2(RingBuffer_t* const b, uint8_t* const out1,
                   uint8_t* const out2) {
  if (b->length < 2) {
    return false;
  }
  if (!Ring_PopHead(b, out1)) {
    return false;
  }
  if (!Ring_PopHead(b, out2)) {
    return false;
  }
  return true;
}

bool Ring_PopHead3(RingBuffer_t* const b, uint8_t* const out1,
                   uint8_t* const out2, uint8_t* const out3) {
  if (b->length < 3) {
    return false;
  }
  if (!Ring_PopHead(b, out1)) {
    return false;
  }
  if (!Ring_PopHead(b, out2)) {
    return false;
  }
  if (!Ring_PopHead(b, out3)) {
    return false;
  }
  return true;
}

void Ring_Debug(RingBuffer_t const* const b) {
  printf(
      "ring debug    data=[%c%c%c%c] head=%d tail=%d length=%d free=%d "
      "content=[",
      b->data[0] != 0 ? b->data[0] : '.', b->data[1] != 0 ? b->data[1] : '.',
      b->data[2] != 0 ? b->data[2] : '.', b->data[3] != 0 ? b->data[3] : '.',
      b->head, b->tail, b->length, b->free);
  uint8_t h = b->head, t = b->tail, n = t - h;
  if ((b->free == 0) && (h == t)) {
    n = RING_BUFFER_SIZE;
  } else if (h > t) {
    n = RING_BUFFER_SIZE - h + t;
  }
  for (uint8_t i = 0; i < n; i++) {
    uint8_t j = (h + i) % RING_BUFFER_SIZE;
    printf("%c", b->data[j]);
  }
  printf("]\n");
}

int Ring_Test() {
  RingBuffer_t rb;
  uint8_t v;
  Ring_Init(&rb);
  Ring_Debug(&rb);
  Ring_PushTail(&rb, '1');
  Ring_Debug(&rb);
  Ring_PushTail(&rb, '2');
  Ring_Debug(&rb);
  Ring_PushTail(&rb, 't');
  Ring_Debug(&rb);
  Ring_PopHead(&rb, &v);
  Ring_Debug(&rb);
  Ring_PushTail(&rb, '3');
  Ring_Debug(&rb);
  Ring_PushTail(&rb, '4');
  Ring_Debug(&rb);
  Ring_PushTail(&rb, 'e');
  Ring_Debug(&rb);
  Ring_PopHead(&rb, &v);
  Ring_Debug(&rb);
  Ring_PopHead(&rb, &v);
  Ring_Debug(&rb);
  Ring_PopHead(&rb, &v);
  Ring_Debug(&rb);
  Ring_PopHead(&rb, &v);
  Ring_Debug(&rb);
  Ring_PopHead(&rb, &v);
  Ring_Debug(&rb);
  return 0;
}

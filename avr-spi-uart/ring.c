#include <stdio.h>   // printf, only for debug
#include <string.h>  // memset, required
#include "ring.h"

#ifdef DEBUG
#define debug_printf printf
#undef RING_BUFFER_SIZE
#define RING_BUFFER_SIZE 5
#else
void debug_printf(char* const msg, ...) {}
#endif

inline void Ring_Init(RingBuffer_t* const b) {
  memset((void*)b->data, 0, RING_BUFFER_SIZE);
  b->head = 0;
  b->tail = 0;
  b->length = 0;
  b->free = RING_BUFFER_SIZE;
  debug_printf("ring init     b=%p free=%d\n", b, b->free);
}

inline bool Ring_MoveTail(RingBuffer_t* const b, int8_t const delta) {
  uint8_t next = (b->tail + delta) % RING_BUFFER_SIZE;
  if (delta >= 0) {
    if (delta > b->free) {
      return false;
    }
  } else {
    if (-delta > b->length) {
      return false;
    }
    if (b->tail < -delta) {
      next = RING_BUFFER_SIZE + delta;
    }
  }
  b->length += delta;
  b->free -= delta;
  b->tail = next;
  debug_printf(
      "ring movtail  delta=%d head=%d tail=%d length=%d "
      "free=%d\n",
      delta, b->head, b->tail, b->length, b->free);
  return true;
}

inline bool Ring_PushTail(RingBuffer_t* const b, uint8_t const value) {
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

inline bool Ring_PushTail2(RingBuffer_t* const b, uint8_t const v1,
                           uint8_t const v2) {
  if (b->free < 2) {
    return false;
  }
  b->data[(b->tail)] = v1;
  b->data[(b->tail + 1) % RING_BUFFER_SIZE] = v2;
  return Ring_MoveTail(b, 2);
}

inline bool Ring_PushTail3(RingBuffer_t* const b, uint8_t const v1,
                           uint8_t const v2, uint8_t const v3) {
  if (b->free < 3) {
    return false;
  }
  b->data[(b->tail)] = v1;
  b->data[(b->tail + 1) % RING_BUFFER_SIZE] = v2;
  b->data[(b->tail + 2) % RING_BUFFER_SIZE] = v3;
  return Ring_MoveTail(b, 3);
}

inline bool Ring_MoveHead(RingBuffer_t* const b, int8_t const delta) {
  if (delta >= 0) {
    if (delta > b->length) {
      return false;
    }
  } else {
    if (-delta > b->free) {
      return false;
    }
  }
  b->length -= delta;
  b->free += delta;
  b->head = (b->head + delta) % RING_BUFFER_SIZE;
  debug_printf(
      "ring movhead  delta=%d head=%d tail=%d length=%d "
      "free=%d\n",
      delta, b->head, b->tail, b->length, b->free);
  return true;
}

inline bool Ring_PeekHead(RingBuffer_t* const b, uint8_t* const out) {
  if (b->length == 0) {
    debug_printf("ring peek     ERR empty head=%d tail=%d length=%d\n", b->head,
                 b->tail, b->length);
    return false;
  }
  if (out != NULL) {
    *out = b->data[b->head];
  }
  debug_printf("ring peek     out=%c head=%d length=%d\n", b->data[b->head],
               b->head, b->length);
  return true;
}

inline bool Ring_PeekHead3(RingBuffer_t* const b, uint8_t* const out1,
                           uint8_t* const out2, uint8_t* const out3) {
  if (b->length < 3) {
    return false;
  }
  *out1 = b->data[b->head];
  *out2 = b->data[(b->head + 1) % RING_BUFFER_SIZE];
  *out3 = b->data[(b->head + 2) % RING_BUFFER_SIZE];
  debug_printf(
      "ring peek3    out1=%c out2=%c out3=%c head=%d tail=%d length=%d "
      "free=%d\n",
      *out1, *out2, *out3, b->head, b->tail, b->length, b->free);
  return true;
}

inline bool Ring_PopHead(RingBuffer_t* const b, uint8_t* const out) {
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

inline bool Ring_PopHead2(RingBuffer_t* const b, uint8_t* const out1,
                          uint8_t* const out2) {
  if (b->length < 2) {
    return false;
  }
  *out1 = b->data[b->head];
  *out2 = b->data[(b->head + 1) % RING_BUFFER_SIZE];
  return Ring_MoveHead(b, 2);
}

inline bool Ring_PopHead3(RingBuffer_t* const b, uint8_t* const out1,
                          uint8_t* const out2, uint8_t* const out3) {
  if (b->length < 3) {
    return false;
  }
  *out1 = b->data[b->head];
  *out2 = b->data[(b->head + 1) % RING_BUFFER_SIZE];
  *out3 = b->data[(b->head + 2) % RING_BUFFER_SIZE];
  return Ring_MoveHead(b, 3);
}

inline void Ring_Debug(RingBuffer_t* const b) {
  printf("ring debug    data=[");
  for (uint8_t i = 0; i < RING_BUFFER_SIZE; i++) {
    printf("%c", b->data[i] != 0 ? b->data[i] : '.');
  }
  printf("] head=%d tail=%d length=%d free=%d content=[", b->head, b->tail,
         b->length, b->free);
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

#ifdef TEST
#include <assert.h>
int main() {
  RingBuffer_t rb;
  uint8_t v1, v2, v3;
  Ring_Init(&rb);
  assert(rb.head == 0);
  assert(rb.tail == 0);
  assert(rb.length == 0);
  Ring_Debug(&rb);
  assert(Ring_PushTail(&rb, '1'));
  assert(rb.data[0] == '1');
  assert(rb.head == 0);
  assert(rb.tail == 1);
  assert(rb.length == 1);
  Ring_Debug(&rb);
  assert(Ring_PushTail(&rb, '2'));
  assert(rb.data[1] == '2');
  assert(rb.head == 0);
  assert(rb.tail == 2);
  assert(rb.length == 2);
  Ring_Debug(&rb);
  assert(Ring_PushTail(&rb, 't'));
  assert(rb.data[2] == 't');
  assert(rb.head == 0);
  assert(rb.tail == 3);
  assert(rb.length == 3);
  Ring_Debug(&rb);
  assert(Ring_PopHead(&rb, &v1));
  assert(v1 == '1');
  assert(rb.head == 1);
  assert(rb.tail == 3);
  assert(rb.length == 2);
  Ring_Debug(&rb);
  assert(Ring_PushTail(&rb, '3'));
  assert(rb.data[2] == 't');
  assert(rb.head == 1);
  assert(rb.tail == 4);
  assert(rb.length == 3);
  Ring_Debug(&rb);
  assert(Ring_PushTail(&rb, '4'));
  assert(rb.data[2] == 't');
  assert(rb.head == 1);
  assert(rb.tail == 0);
  assert(rb.length == 4);
  Ring_Debug(&rb);
  assert(Ring_PushTail(&rb, 'e'));
  assert(rb.data[2] == 't');
  assert(rb.head == 1);
  assert(rb.tail == 1);
  assert(rb.length == 5);
  Ring_Debug(&rb);
  assert(Ring_PopHead(&rb, &v2));
  assert(v2 == '2');
  assert(rb.head == 2);
  assert(rb.tail == 1);
  assert(rb.length == 4);
  Ring_Debug(&rb);
  assert(Ring_PopHead2(&rb, &v1, &v2));
  assert(v1 == 't');
  assert(v2 == '3');
  assert(rb.head == 4);
  assert(rb.tail == 1);
  assert(rb.length == 2);
  Ring_Debug(&rb);
  assert(Ring_PopHead(&rb, &v1));
  assert(v1 == '4');
  assert(rb.head == 0);
  assert(rb.tail == 1);
  assert(rb.length == 1);
  Ring_Debug(&rb);

  Ring_Init(&rb);
  assert(Ring_PushTail3(&rb, 'C', 'D', 'E'));
  assert(rb.head == 0);
  assert(rb.tail == 3);
  assert(rb.length == 3);
  assert(!Ring_PushTail3(&rb, 'x', 'y', 'z'));
  assert(rb.head == 0);
  assert(rb.tail == 3);
  assert(rb.length == 3);
  assert(Ring_PushTail2(&rb, '1', '2'));
  assert(rb.head == 0);
  assert(rb.tail == 0);
  assert(rb.length == 5);
  Ring_Debug(&rb);
  assert(Ring_PeekHead(&rb, &v1));
  assert(v1 == 'C');
  assert(rb.head == 0);
  assert(rb.tail == 0);
  assert(rb.length == 5);
  assert(Ring_MoveHead(&rb, 2));
  assert(rb.head == 2);
  assert(rb.tail == 0);
  assert(rb.length == 3);
  Ring_Debug(&rb);
  assert(Ring_MoveHead(&rb, -1));
  assert(rb.head == 1);
  assert(rb.tail == 0);
  assert(rb.length == 4);
  Ring_Debug(&rb);
  assert(Ring_MoveTail(&rb, -1));
  assert(rb.head == 1);
  assert(rb.tail == 4);
  assert(rb.length == 3);
  Ring_Debug(&rb);
  assert(!Ring_MoveTail(&rb, -4));
  assert(rb.head == 1);
  assert(rb.tail == 4);
  assert(rb.length == 3);
  Ring_Debug(&rb);
  assert(Ring_MoveTail(&rb, 1));
  assert(rb.head == 1);
  assert(rb.tail == 0);
  assert(rb.length == 4);
  Ring_Debug(&rb);
  assert(Ring_MoveHead(&rb, 2));
  assert(rb.head == 3);
  assert(rb.tail == 0);
  assert(rb.length == 2);
  Ring_Debug(&rb);
  assert(Ring_PushTail2(&rb, '3', '4'));
  assert(rb.head == 3);
  assert(rb.tail == 2);
  assert(rb.length == 4);
  Ring_Debug(&rb);
  assert(Ring_PopHead(&rb, &v1));
  assert(v1 == '1');
  assert(rb.head == 4);
  assert(rb.tail == 2);
  assert(rb.length == 3);
  Ring_Debug(&rb);
  assert(Ring_PopHead3(&rb, &v1, &v2, &v3));
  assert(v1 == '2');
  assert(v2 == '3');
  assert(v3 == '4');
  assert(rb.head == 2);
  assert(rb.tail == 2);
  assert(rb.length == 0);
  Ring_Debug(&rb);
  return 0;
}
#endif

#ifndef INCLUDE_RING_H
#define INCLUDE_RING_H

#include <stdbool.h>
#include <inttypes.h>

#define RING_BUFFER_SIZE 228
#if RING_BUFFER_SIZE > 256
#error "RING_BUFFER_SIZE must be <=256"
#endif

typedef struct {
  uint8_t data[RING_BUFFER_SIZE];
  uint8_t head;
  uint8_t tail;
  uint8_t length;
  uint8_t free;
} volatile RingBuffer_t;

void Ring_Init(RingBuffer_t* const b);
bool Ring_MoveTail(RingBuffer_t* const b, int8_t const delta);
bool Ring_PushTail(RingBuffer_t* const b, uint8_t const value);
bool Ring_PushTailN(RingBuffer_t* const b, uint8_t const* const src,
                    uint8_t const n);
bool Ring_PushTail2(RingBuffer_t* const b, uint8_t const v1, uint8_t const v2);
bool Ring_PeekHead(RingBuffer_t* const b, uint8_t* const out);
bool Ring_MoveHead(RingBuffer_t* const b, int8_t const delta);
bool Ring_PopHead(RingBuffer_t* const b, uint8_t* const out);
bool Ring_PopHeadN(RingBuffer_t* const b, uint8_t* const dst, uint8_t const n);
void Ring_Debug(RingBuffer_t* const b);

#endif  // INCLUDE_RING_H

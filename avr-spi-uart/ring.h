#ifndef INCLUDE_RING_H
#define INCLUDE_RING_H

#include <stdbool.h>
#include <inttypes.h>

#define RING_BUFFER_SIZE 128
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
bool Ring_PushTail2(RingBuffer_t* const b, uint8_t const v1, uint8_t const v2);
bool Ring_PushTail3(RingBuffer_t* const b, uint8_t const v1, uint8_t const v2,
                    uint8_t const v3);
bool Ring_PeekHead(RingBuffer_t* const b, uint8_t* const out);
bool Ring_PeekHead2(RingBuffer_t* const b, uint8_t* const out1,
                    uint8_t* const out2);
bool Ring_PeekHead3(RingBuffer_t* const b, uint8_t* const out1,
                    uint8_t* const out2, uint8_t* const out3);
bool Ring_MoveHead(RingBuffer_t* const b, int8_t const delta);
bool Ring_PopHead(RingBuffer_t* const b, uint8_t* const out);
bool Ring_PopHead2(RingBuffer_t* const b, uint8_t* const out1,
                   uint8_t* const out2);
bool Ring_PopHead3(RingBuffer_t* const b, uint8_t* const out1,
                   uint8_t* const out2, uint8_t* const out3);
void Ring_Debug(RingBuffer_t* const b);

#endif  // INCLUDE_RING_H

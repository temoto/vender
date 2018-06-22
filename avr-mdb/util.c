#ifndef INCLUDE_UTIL_C
#define INCLUDE_UTIL_C
#include <inttypes.h>
#include <stdbool.h>

static bool bit_test(uint8_t const x, uint8_t const mask) {
  return (x & mask) == mask;
}

static uint8_t memsum(uint8_t const *const src, uint8_t const length) {
  uint8_t sum = 0;
  for (uint8_t i = 0; i < length; i++) {
    sum += src[i];
  }
  return sum;
}

static uint8_t uint8_min(uint8_t const a, uint8_t const b) {
  return a < b ? a : b;
}

#endif  // INCLUDE_UTIL_C

#include "crc.h"
#include <assert.h>
#include <inttypes.h>
#include <stdio.h>

uint8_t test1(uint8_t const b) {
  uint8_t const crc = crc8_p93_next(0, b);
  printf("b1=%02x crc=%02x\n", b, crc);
  return crc;
}

uint8_t test2(uint8_t const b1, uint8_t const b2) {
  uint8_t const crc = crc8_p93_2b(b1, b2);
  printf("b2=%02x%02x crc=%02x\n", b1, b2, crc);
  return crc;
}

uint8_t test_str(uint8_t const bs[], uint8_t const len) {
  uint8_t crc = 0;
  printf("bs=");
  for (uint8_t i = 0; i < len; i++) {
    printf("%02x", bs[i]);
    crc = crc8_p93_next(crc, bs[i]);
  }
  printf(" crc=%02x\n", crc);
  return crc;
}

int main() {
  assert(test1(0x00) == 0x00);
  assert(test1(0x55) == 0x86);
  assert(test1(0xaa) == 0x9f);
  assert(test1(0xff) == 0x19);

  assert(test2(0x80, 0x00) == 0x74);
  assert(test2(0xe0, 0x78) == 0xc9);
  assert(test2(0x03, 0x01) == 0xc8);
  assert(test2(0x01, 0x03) == 0x9e);

  assert(test_str((uint8_t[]){0x04, 0x08, 0x30}, 3) == 0xf9);
  assert(test_str((uint8_t[]){0x04, 0x02, 0x01}, 3) == 0xf6);
  assert(test_str((uint8_t[]){0x05, 0x17, 0x08, 0xe1}, 4) == 0xc8);
  return 0;
}

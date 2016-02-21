#include <inttypes.h>
#include <stdio.h>
#include "crc.h"

void test1(uint8_t const b) {
  printf("b1=%02x crc=%02x\n", b, crc8_p93_next(0, b));
}

void test2(uint8_t const b1, uint8_t const b2) {
  printf("b2=%02x%02x crc=%02x\n", b1, b2, crc8_p93_2b(b1, b2));
}

void test_str(uint8_t const bs[], uint8_t const len) {
  uint8_t crc = 0;
  printf("bs=");
  for (uint8_t i = 0; i < len; i++) {
    printf("%02x", bs[i]);
    crc = crc8_p93_next(crc, bs[i]);
  }
  printf(" crc=%02x\n", crc);
}

int main() {
  test1(0x00);
  test1(0x55);
  test1(0xaa);
  test1(0xff);

  test2(0x80, 0x00);
  test2(0x30, 0x30);
  test2(0x20, 0x30);
  test2(0xe0, 0x69);
  test2(0xe0, 0x78);

  test2(0x03, 0x01);
  test_str((uint8_t[]){0x06, 0x00, 0xbe, 0xeb, 0xee}, 5);
  test_str((uint8_t[]){0x04, 0x0f, 0x30}, 3);
  test2(0x03, 0x00);
  test2(0x03, 0x08);
  return 0;
}

#include <inttypes.h>
#include <stdbool.h>
#include "crc.h"

uint8_t const CRC_POLY_93 = 0x93;

inline uint8_t crc8_p93_next(uint8_t crc, uint8_t data) {
  crc ^= data;
  for (int8_t i = 0; i < 8; i++) {
    if ((crc & 0x80) != 0) {
      crc <<= 1;
      crc ^= CRC_POLY_93;
    } else {
      crc <<= 1;
    }
  }
  return crc;
}

inline uint8_t crc8_p93_2b(uint8_t const data1, uint8_t const data2) {
  uint8_t crc = 0;
  crc = crc8_p93_next(crc, data1);
  crc = crc8_p93_next(crc, data2);
  return crc;
}

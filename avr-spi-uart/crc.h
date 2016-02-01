#ifndef INCLUDE_CRC_H
#define INCLUDE_CRC_H

#include <inttypes.h>

uint8_t crc8_p93_next(uint8_t crc, uint8_t data);
uint8_t crc8_p93_2b(uint8_t const data1, uint8_t const data2);

#endif // INCLUDE_CRC_H

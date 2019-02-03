#ifndef INCLUDE_TWI_C
#define INCLUDE_TWI_C
#include <inttypes.h>
#include <stdbool.h>
#include <util/twi.h>
#include "crc.h"

// Begin TWI driver

typedef struct {
  uint8_t no_info;
  uint8_t bus_error;
  uint8_t sr_data_nack;
  uint8_t sr_gcall_data_nack;
  uint8_t st_data_nack;
  uint8_t st_last_data;
  uint8_t sr_stop;
  uint8_t out_empty_set_length;
} twi_stat_t;

static bool volatile twi_idle = true;
static uint8_t volatile twi_in_data[COMMAND_MAX_LENGTH];
static buffer_t volatile twi_in;
static uint8_t volatile twi_out_data1[RESPONSE_MAX_LENGTH];
static uint8_t volatile twi_out_data2[RESPONSE_MAX_LENGTH];
static twi_stat_t volatile twi_stat;

// master_out/twi_out is double buffer
static buffer_t volatile master_out;
static buffer_t volatile twi_out;

static void twi_init_slave(uint8_t const address) {
  TWCR = 0;
  TWBR = 0x0c;
  TWAR = address << 1;
  TWSR = 0;
  twi_idle = true;
  buffer_init(&twi_in, (uint8_t *)twi_in_data, sizeof(twi_in_data));
  buffer_init(&master_out, (uint8_t *)twi_out_data1, sizeof(twi_out_data1));
  buffer_init(&twi_out, (uint8_t *)twi_out_data2, sizeof(twi_out_data2));
  memset((void *)&twi_stat, 0, sizeof(twi_stat_t));
  TWCR = _BV(TWINT) | _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
}

static bool twi_step(void) {
  bool again = false;

  // TWI read is finished
  uint8_t const in_length = twi_in.length;  // anti-volatile
  if (in_length > 0) {
    uint8_t *src = twi_in.data;
    if (in_length == 1) {
      // keyboard sends 1 byte
      master_out_1(0, RESPONSE_TWI, src[0]);
      again = true;
    } else {
      // master sends >= 4 bytes
      uint8_t i = 0;
      for (;;) {
        uint8_t const consumed = master_command(src, in_length - i);
        if (consumed == 0) {
          break;
        }
        i += consumed;
        src += consumed;
        if (i >= in_length) {
          break;
        }
      }
      again = true;
    }
    buffer_clear_fast(&twi_in);
  }
  if ((twi_out.used >= twi_out.length) && (master_out.length > 0)) {
    buffer_swap(&twi_out, &master_out);
    twi_out.used = 0;
    buffer_clear_full(&master_out);
    again = true;
  }

  return again;
}

static void twi_out_set_1(uint8_t const command_id, response_t const header,
                          uint8_t const data) {
  uint8_t const length = 4 + 1;
  uint8_t packet[] = {length, command_id, header, data, 0};
  packet[length - 1] = crc8_p93_n(0, packet, length - 1);
  buffer_copy(&twi_out, packet, length);
}

static void master_out_0(uint8_t const command_id, response_t const header) {
  uint8_t const length = 4 + 0;
  uint8_t const crc = crc8_p93_3b(length, command_id, header);
  uint8_t const packet[] = {length, command_id, header, crc};
  if (!buffer_append_n(&master_out, packet, length)) {
    twi_out_set_1(command_id, RESPONSE_BUFFER_OVERFLOW, length);
  }
}
static void master_out_1(uint8_t const command_id, response_t const header,
                         uint8_t const data) {
  uint8_t packet[] = {0, command_id, header, data, 0};
  uint8_t const length = packet[0] = sizeof(packet);
  packet[length - 1] = crc8_p93_n(0, packet, length - 1);
  if (!buffer_append_n(&master_out, packet, length)) {
    twi_out_set_1(command_id, RESPONSE_BUFFER_OVERFLOW, length);
  }
}
static void master_out_2(uint8_t const command_id, response_t const header,
                         uint8_t const data1, uint8_t const data2) {
  uint8_t packet[] = {0, command_id, header, data1, data2, 0};
  uint8_t const length = packet[0] = sizeof(packet);
  packet[length - 1] = crc8_p93_n(length, packet, length - 1);
  if (!buffer_append_n(&master_out, packet, length)) {
    twi_out_set_1(command_id, RESPONSE_BUFFER_OVERFLOW, length);
  }
}
static void master_out_n(uint8_t const command_id, response_t const header,
                         uint8_t const *const data, uint8_t const data_length) {
  uint8_t const packet_length = 4 + data_length;
  if (master_out.length + packet_length > master_out.size) {
    twi_out_set_1(command_id, RESPONSE_BUFFER_OVERFLOW, packet_length);
    return;
  }
  uint8_t const prefix[] = {packet_length, command_id, header};
  buffer_append_n(&master_out, prefix, sizeof(prefix));
  buffer_append_n(&master_out, data, data_length);
  uint8_t crc = crc8_p93_3b(packet_length, command_id, header);
  crc = crc8_p93_n(crc, data, data_length);
  buffer_append(&master_out, crc);
}

ISR(TWI_vect) {
  bool ack = false;
  uint8_t const st = TW_STATUS;
  switch (st) {
    case TW_ST_DATA_NACK:
      twi_stat.st_data_nack++;
      break;
    case TW_ST_LAST_DATA:
      twi_stat.st_last_data++;
      break;
    case TW_SR_DATA_NACK:
      twi_stat.sr_data_nack++;
      break;
    case TW_SR_GCALL_DATA_NACK:
      twi_stat.sr_gcall_data_nack++;
      break;
  }

  switch (st) {
    case TW_NO_INFO:
      twi_stat.no_info++;
      return;

    case TW_BUS_ERROR:
      twi_stat.bus_error++;
      buffer_clear_fast(&twi_in);
      buffer_clear_fast(&twi_out);  // TODO maybe bad idea
      TWCR = _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      twi_idle = false;
      buffer_clear_fast(&twi_in);
      ack = true;
      break;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      twi_idle = false;
      buffer_append(&twi_in, TWDR);
      ack = true;
      break;

    // data received, NACK returned
    case TW_SR_DATA_NACK:
    case TW_SR_GCALL_DATA_NACK:
      twi_idle = false;
      ack = false;
      break;

    // Receive Stop or ReStart
    case TW_SR_STOP:
      twi_stat.sr_stop++;
      twi_idle = true;
      ack = true;
      break;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      twi_idle = false;
      if (twi_out.length == 0) {
        twi_stat.out_empty_set_length++;
        // twi_out_set_2(Response_Status, master_out.length);
        TWDR = 0;
        ack = false;
        break;
      }
      twi_out.used = 0;
      TWDR = twi_out.length;
      ack = (twi_out.used < twi_out.length);
      break;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      twi_idle = false;
      ack = (twi_out.used < twi_out.length);
      if (ack) {
        TWDR = twi_out.data[twi_out.used];
        twi_out.used++;
      } else {
        TWDR = 0;
      }
      break;

    // Send Last Byte Receive ACK
    case TW_ST_LAST_DATA:
      twi_idle = true;
      buffer_clear_fast(&twi_out);
      ack = true;
      break;
    // Send Last Byte Receive NACK
    case TW_ST_DATA_NACK:
      twi_idle = true;
      twi_out.used = 0;
      break;
  }
  TWCR = _BV(TWINT) | (ack ? _BV(TWEA) : 0) | _BV(TWEN) | _BV(TWIE);
}
// End TWI driver

#endif  // INCLUDE_TWI_C

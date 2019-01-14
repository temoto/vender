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
} TwiStat_t;

// static uint8_t volatile twi_send_limit = 0;
static bool volatile twi_idle = true;
static uint8_t volatile twi_in_data[COMMAND_MAX_LENGTH];
static Buffer_t volatile twi_in;
static uint8_t volatile twi_out_data1[RESPONSE_MAX_LENGTH];
static uint8_t volatile twi_out_data2[RESPONSE_MAX_LENGTH];
// XXX
static TwiStat_t volatile twi_stat;

// master_out/twi_out is double buffer
static Buffer_t volatile master_out;
static Buffer_t volatile twi_out;

static void twi_init_slave(uint8_t const address) {
  TWCR = 0;
  TWBR = 0x0c;
  TWAR = address << 1;
  TWSR = 0;
  twi_idle = true;
  // twi_send_limit = 0;
  Buffer_Init(&twi_in, (uint8_t *)twi_in_data, sizeof(twi_in_data));
  Buffer_Init(&master_out, (uint8_t *)twi_out_data1, sizeof(twi_out_data1));
  Buffer_Init(&twi_out, (uint8_t *)twi_out_data2, sizeof(twi_out_data2));
  memset((void *)&twi_stat, 0, sizeof(TwiStat_t));
  TWCR = _BV(TWINT) | _BV(TWEA) | _BV(TWEN) | _BV(TWIE);
}

static bool twi_step(void) {
  bool again = false;

  // TWI read is finished
  if (twi_in.length > 0) {
    uint8_t *src = twi_in.data;
    if (twi_in.length == 1) {
      // keyboard sends 1 byte
      master_out_2(Response_TWI, src[0]);
      again = true;
    } else {
      // master sends >= 3 bytes
      uint8_t i = 0;
      for (;;) {
        uint8_t const consumed = master_command(src, twi_in.length - i);
        if (consumed == 0) {
          break;
        }
        i += consumed;
        src += consumed;
        if (i >= twi_in.length) {
          break;
        }
      }
      again = true;
    }
    Buffer_Clear_Fast(&twi_in);
  }
  if ((twi_out.used >= twi_out.length) && (master_out.length > 0)) {
    Buffer_Swap(&twi_out, &master_out);
    twi_out.used = 0;
    Buffer_Clear_Full(&master_out);
    again = true;
  }

  return again;
}

static void twi_out_set_2(uint8_t const header, uint8_t const data) {
  Buffer_Clear_Fast(&twi_out);
  uint8_t const length = 4;
  twi_out.data[0] = length;
  twi_out.data[1] = header;
  twi_out.data[2] = data;
  twi_out.data[3] = crc8_p93_n(0, twi_out.data, length - 1);
  twi_out.data[4] = 0;
  twi_out.length = length;
}

static void master_out_1(Response_t const header) {
  uint8_t const packet_length = 3;
  uint8_t const crc = crc8_p93_2b(packet_length, header);
  uint8_t const packet[] = {packet_length, header, crc};
  if (!Buffer_AppendN(&master_out, packet, packet_length)) {
    twi_out_set_2(Response_Buffer_Overflow, packet_length);
  }
}

static void master_out_2(Response_t const header, uint8_t const data) {
  uint8_t const packet_length = 4;
  uint8_t const crc = crc8_p93_3b(packet_length, header, data);
  uint8_t const packet[4] = {packet_length, header, data, crc};
  if (!Buffer_AppendN(&master_out, packet, packet_length)) {
    twi_out_set_2(Response_Buffer_Overflow, packet_length);
  }
}

static void master_out_n(Response_t const header, uint8_t const *const data,
                         uint8_t const data_length) {
  uint8_t const packet_length = 3 + data_length;
  if (master_out.free < packet_length) {
    twi_out_set_2(Response_Buffer_Overflow, packet_length);
    return;
  }
  Buffer_Append(&master_out, packet_length);
  Buffer_Append(&master_out, header);
  Buffer_AppendN(&master_out, data, data_length);
  uint8_t crc = crc8_p93_2b(packet_length, header);
  crc = crc8_p93_n(crc, data, data_length);
  Buffer_Append(&master_out, crc);
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
      Buffer_Clear_Fast(&twi_in);
      Buffer_Clear_Fast(&twi_out);  // TODO maybe bad idea
      TWCR = _BV(TWSTO) | _BV(TWINT) | _BV(TWEN) | _BV(TWIE);
      return;

    // Receive SLA+W
    case TW_SR_SLA_ACK:
    case TW_SR_GCALL_ACK:
    // Receive SLA+R LP
    case TW_SR_ARB_LOST_SLA_ACK:
    case TW_SR_ARB_LOST_GCALL_ACK:
      twi_idle = false;
      // twi_send_limit = UINT8_MAX;
      Buffer_Clear_Fast(&twi_in);
      ack = true;
      break;

    // data received, ACK returned
    case TW_SR_DATA_ACK:
    case TW_SR_GCALL_DATA_ACK:
      twi_idle = false;
      Buffer_Append(&twi_in, TWDR);
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
      Buffer_Clear_Fast(&twi_in);
      // FIXME likely not needed
      // twi_in.used = twi_in.length;
      ack = true;
      break;

    // Receive SLA+R
    case TW_ST_SLA_ACK:
      twi_idle = false;
      if (twi_out.length == 0) {
        twi_out_set_2(Response_Queue_Size, twi_out.length);
      } else {
        twi_out.used = 0;
      }
      TWDR = twi_out.length;
      // ack = (twi_out.used < uint8_min(twi_send_limit, twi_out.length));
      ack = (twi_out.used < twi_out.length);
      break;

    // Send Byte Receive ACK
    case TW_ST_DATA_ACK:
      twi_idle = false;
      // ack = (twi_out.used < uint8_min(twi_send_limit, twi_out.length));
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
    // Send Last Byte Receive NACK
    case TW_ST_DATA_NACK:
      twi_idle = true;
      Buffer_Clear_Fast(&twi_out);
      ack = true;
      break;
  }
  TWCR = _BV(TWINT) | (ack ? _BV(TWEA) : 0) | _BV(TWEN) | _BV(TWIE);
}
// End TWI driver

#endif  // INCLUDE_TWI_C

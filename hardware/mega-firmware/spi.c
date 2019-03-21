#ifndef _INCLUDE_FROM_MAIN
#error this file looks like standalone C source, but actually must be included in main.c
#endif
#ifndef INCLUDE_SPI_C
#define INCLUDE_SPI_C
#include "config.h"

#include <avr/interrupt.h>
#include <avr/io.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
#include "bit.h"
#include "buffer.h"
#include "common.h"
#include "crc.h"

static uint8_t volatile spi_last_out_crc;

#define SPI_WAIT_OR_RETURN \
  if (!spi_wait()) {       \
    return;                \
  }

#ifndef SPI_SEND_OR_RETURN
#define SPI_SEND_OR_RETURN(a) \
  SPDR = a;                   \
  SPI_WAIT_OR_RETURN;         \
  (void)SPDR;
#endif

// fwd

static bool spi_selected(void) __attribute__((warn_unused_result));
static void spi_end(uint8_t errcode, uint8_t const pad);
static bool spi_wait(void) __attribute__((warn_unused_result));
static inline void spi_do_send(void);
static inline void spi_do_ack(void);
static inline void spi_do_recv(void);

static void spi_init_slave(void) {
  bit_mask_set(SPI_MISO_DDR, _BV(SPI_MISO_PIN));
  SPCR = _BV(SPE);
  SPSR = 0;

  // drop SPIF
  (void)SPSR;
  (void)SPDR;

  bit_mask_set(PCICR, _BV(PCIE0));
  bit_mask_set(PCMSK0, _BV(PCINT2));
}

static void spi_step(void) {
  master_notify_set(!response_empty() && !spi_selected());
}

ISR(PCINT0_vect) {
  if (!spi_selected()) {
    return;
  }

  bool const out_ready = response.filled;  // anti-volatile
  bool const req_busy = request.filled;    // anti-volatile

  uint8_t out_header = PROTOCOL_VERSION;
  if (out_ready) {
    bit_mask_set(out_header, PROTOCOL_FLAG_PAYLOAD);
  }
  if (req_busy) {
    bit_mask_set(out_header, PROTOCOL_FLAG_REQUEST_BUSY);
  }

  SPDR = out_header;
  SPI_WAIT_OR_RETURN;
  uint8_t const in_header = SPDR;

  uint8_t const in_proto_ver = in_header & PROTOCOL_HEADER_VERSION_MASK;
  if (in_proto_ver != PROTOCOL_VERSION) {
    SPI_SEND_OR_RETURN(0);  // payload length
    SPI_SEND_OR_RETURN(0);  // payload crc
    spi_end(ERROR_FRAME_HEADER, PROTOCOL_PAD_ERROR);
    return;
  }

  uint8_t const in_mode = (in_header & PROTOCOL_HEADER_FLAG_MASK);
  switch (in_mode) {
    // master reads response
    case 0:
      if (!out_ready) {
        SPI_SEND_OR_RETURN(0);  // payload length
        SPI_SEND_OR_RETURN(0);  // payload crc
        // no packet flag was already communicated
        spi_end(0, PROTOCOL_PAD_OK);
        return;
      }

      spi_do_send();
      return;

    // master sends request
    case PROTOCOL_FLAG_PAYLOAD:
      // if (out_ready) {
      //   spi_do_send();
      //   return;
      // }

      if (req_busy) {
        SPI_SEND_OR_RETURN(0);  // payload length
        SPI_SEND_OR_RETURN(0);  // payload crc
        // request is busy flag was already communicated
        spi_end(ERROR_REQUEST_OVERWRITE, PROTOCOL_PAD_ERROR);
        return;
      }

      spi_do_recv();
      return;

    // master confirms previous response
    case PROTOCOL_FLAG_REQUEST_BUSY:
      spi_do_ack();
      return;

    default:
      // safety net, should never happen
      SPI_SEND_OR_RETURN(0);  // payload length
      spi_end(ERROR_FRAME_HEADER, PROTOCOL_PAD_ERROR);
      return;
  }
}

static inline void spi_do_send(void) {
  uint8_t const data_length = response.b.length;  // anti-volatile
  uint8_t const payload_length = data_length + 1;
  SPI_SEND_OR_RETURN(payload_length);
  uint8_t out_crc = crc8_p93_next(0, payload_length);

  uint8_t const tmpkind = response.h.response;  // anti-volatile
  SPI_SEND_OR_RETURN(tmpkind);
  out_crc = crc8_p93_next(out_crc, tmpkind);

  for (uint8_t i = 0; i < data_length; i++) {
    uint8_t const tmpdata = response.b.data[i];
    SPI_SEND_OR_RETURN(tmpdata);
    out_crc = crc8_p93_next(out_crc, tmpdata);
  }

  SPI_SEND_OR_RETURN(out_crc);
  spi_last_out_crc = out_crc;  // save for ack

  spi_end(0, PROTOCOL_PAD_OK);
  return;
}

static inline void spi_do_recv(void) {
  static uint8_t tmp_data[BUFFER_SIZE + 1];
  SPDR = 0;
  SPI_WAIT_OR_RETURN;
  uint8_t const in_length = SPDR;
  uint8_t payload_crc = crc8_p93_next(0, 0);

  if (in_length == 0) {
    // too bad we already sent length 0
    spi_end(ERROR_FRAME_LENGTH, PROTOCOL_PAD_ERROR);
    return;
  }
  if (in_length >= BUFFER_SIZE) {
    // too bad we already sent length 0
    spi_end(ERROR_BUFFER_OVERFLOW, PROTOCOL_PAD_ERROR);
    return;
  }

  uint8_t crc_local = crc8_p93_next(0, in_length);
  for (uint8_t i = 0; i < in_length; i++) {
    uint8_t const out_data = 0;
    SPDR = out_data;
    SPI_WAIT_OR_RETURN;
    tmp_data[i] = SPDR;
    crc_local = crc8_p93_next(crc_local, tmp_data[i]);
    payload_crc = crc8_p93_next(payload_crc, out_data);
  }

  // prevent garbage output if master pushes more bytes
  // while request is being parsed
  SPDR = 0;
  SPI_WAIT_OR_RETURN;
  uint8_t const crc_remote = SPDR;
  payload_crc = crc8_p93_next(payload_crc, 0);

  SPI_SEND_OR_RETURN(0);
  payload_crc = crc8_p93_next(payload_crc, 0);
  SPI_SEND_OR_RETURN(0xff);
  payload_crc = crc8_p93_next(payload_crc, 0xff);
  SPI_SEND_OR_RETURN(crc_local);
  payload_crc = crc8_p93_next(payload_crc, crc_local);
  SPI_SEND_OR_RETURN(crc_remote);
  payload_crc = crc8_p93_next(payload_crc, crc_remote);

  SPI_SEND_OR_RETURN(payload_crc);

  if (crc_local != crc_remote) {
    spi_end(ERROR_INVALID_CRC, PROTOCOL_PAD_ERROR);
    return;
  }

  packet_clear_fast((packet_t*)&request);
  request.h.command = tmp_data[0];
  buffer_copy((buffer_t*)&request.b, tmp_data + 1, in_length - 1);
  request.filled = true;
  spi_end(0, PROTOCOL_PAD_OK);
}

// master confirms previous response
static inline void spi_do_ack(void) {
  uint8_t const payload_length = 2;
  SPI_SEND_OR_RETURN(payload_length);
  uint8_t payload_crc = crc8_p93_next(0, payload_length);

  uint8_t local_length = response.b.length + 1;  // anti-volatile
  SPDR = local_length;
  payload_crc = crc8_p93_next(payload_crc, local_length);
  SPI_WAIT_OR_RETURN;
  uint8_t const remote_length = SPDR;

  uint8_t local_crc = spi_last_out_crc;  // anti-volatile
  SPDR = local_crc;
  payload_crc = crc8_p93_next(payload_crc, local_crc);
  SPI_WAIT_OR_RETURN;
  uint8_t const remote_crc = SPDR;

  SPI_SEND_OR_RETURN(payload_crc);

  bool const match =
      (local_length == remote_length) && (local_crc == remote_crc);
  if (!match) {
    spi_end(ERROR_INVALID_ACK, PROTOCOL_PAD_ERROR);
    return;
  }

  packet_clear_fast((packet_t*)&response);
  spi_end(0, PROTOCOL_PAD_OK);
}

static void spi_end(uint8_t const errcode, uint8_t const pad) {
  SPI_SEND_OR_RETURN(errcode);
  for (;;) {
    SPI_SEND_OR_RETURN(pad);
  }
}

static inline bool spi_selected(void) {
  return !bit_mask_test(SPI_SS_PORT, _BV(SPI_SS_PIN));
}

static inline bool spi_wait(void) {
  for (;;) {
    // new byte is shifted
    if (bit_mask_test(SPSR, _BV(SPIF))) {
      return true;
    }
    // end of transmission
    if (!spi_selected()) {
      return false;
    }
  }
}

#endif  // INCLUDE_SPI_C

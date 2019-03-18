#ifndef _INCLUDE_FROM_MAIN
#error this file looks like standalone C source, but actually must be included in main.c
#endif
#ifndef INCLUDE_MDB_C
#define INCLUDE_MDB_C
#include <avr/interrupt.h>
#include <inttypes.h>
#include <stdbool.h>
#include <string.h>
#include "bit.h"
#include "buffer.h"
#include "common.h"
#include "protocol.h"

/*
MDB timings:
  t = 1.0 mS inter-byte (max.)
  t = 5.0 mS response_t (max.)
  t = 100 mS break (min.)
  t = 200 mS setup (min.)
*/

typedef struct {
  mdb_state_t state;
  uint8_t request_id;
  mdb_result_t result;
  uint8_t error_data;
  uint8_t in_chk;
  bool retrying;
  uint16_t start_clock;
  uint16_t duration;
} mdb_t;

static mdb_t volatile mdb;
static buffer_t volatile mdb_in;
static buffer_t volatile mdb_out;
static uint16_t volatile mdb_timeout_ticks;

// UCSZ02 = 9 data bit
static uint8_t const UCSRB_BASE =
    _BV(RXEN0) | _BV(TXEN0) | _BV(UCSZ02) | _BV(RXCIE0);

// fwd

static inline bool uart_recv_ready(void) __attribute__((warn_unused_result));
static inline bool uart_send_ready(void) __attribute__((warn_unused_result));
static void mdb_start_receive(void);
static void mdb_finish(mdb_result_t const result, uint8_t const error_data);
static void mdb_bus_reset_finish(void);
static inline void mdb_handle_recv_end(void);
static inline uint16_t ms_to_timer16_p1024(uint16_t const ms)
    __attribute__((const, warn_unused_result));
static void timer1_set(uint16_t const ms);
static void timer1_stop(void);

// Baud Rate = 9600 +1%/-2% NRZ 9-N-1
static void mdb_init(void) {
  static uint8_t in_data[MDB_BLOCK_SIZE];
  static uint8_t out_data[MDB_BLOCK_SIZE];

  buffer_init(&mdb_in, (uint8_t * const)in_data, sizeof(in_data));
  buffer_init(&mdb_out, (uint8_t * const)out_data, sizeof(out_data));
  mdb_reset();

  uint32_t const MDB_BAUDRATE = 9600;
  uint32_t const BAUD_PRESCALE = (((F_CPU / (MDB_BAUDRATE * 16UL))) - 1);
  // set baud rate
  UBRR0H = (BAUD_PRESCALE >> 8);
  UBRR0L = BAUD_PRESCALE;
  // UCSZ00+UCSZ01+UCSZ02 = 9 data bits
  UCSR0C = _BV(UCSZ00) | _BV(UCSZ01);
  UCSR0B = UCSRB_BASE;

  mdb_timeout_ticks = ms_to_timer16_p1024(MDB_TIMEOUT_MS);
}

static void mdb_step(void) {
  if (mdb.state == MDB_STATE_RECV_END) {
    mdb_handle_recv_end();
  }
  if (mdb.state == MDB_STATE_DONE) {
    if (!response_empty()) {
      return;
    }

    uint8_t const r = mdb.result;  // anti-volatile
    uint8_t len = mdb_in.length;
    uint8_t const header =
        (r == MDB_RESULT_SUCCESS ? RESPONSE_OK : RESPONSE_ERROR);

    response_begin(mdb.request_id, header);
    response_f2(FIELD_MDB_RESULT, r, mdb.error_data);
    response_f2(FIELD_MDB_DURATION10U, (mdb.duration >> 8),
                (mdb.duration & 0xff));

    if ((r == MDB_RESULT_SUCCESS) && (len > 0)) {
      len--;
    }
    response_fn(FIELD_MDB_DATA, mdb_in.data, len);
    response_finish();
    mdb_reset();
    return;
  }

  return;
}
static inline void mdb_handle_recv_end(void) {
  uint8_t const len = mdb_in.length;
  if (len == 0) {
    mdb_finish(MDB_RESULT_CODE_ERROR, __LINE__);
    return;
  }
  if (len == 1) {
    mdb_finish(MDB_RESULT_CODE_ERROR, __LINE__);
    return;
  }

  uint8_t const last_byte = mdb_in.data[len - 1];
  if (last_byte != mdb.in_chk) {
    if (mdb.retrying) {
      // invalid checksum even after retry
      // VMC ---ADD*---DAT--CHK----------------RET----------------NAK--
      // Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----
      UDR0 = MDB_NAK;
      mdb_finish(MDB_RESULT_INVALID_CHK, 0);
      return;
    } else {
      // VMC ---ADD*---DAT--CHK----------------RET----------------ACK--
      // Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----
      UDR0 = MDB_RET;
      mdb.retrying = true;
      mdb_start_receive();
      return;
    }
  }

  // VMC ---ADD*---CHK----------------ACK-
  // Per -------------DAT---DAT---CHK*----
  UDR0 = MDB_ACK;
  mdb_finish(MDB_RESULT_SUCCESS, 0);
  return;
}

// Called from master_command()
// allowed to write response
static void mdb_tx_begin(uint8_t const request_id, uint8_t const *const data,
                         uint8_t const length) {
  if (length == 0) {
    response_error2(request_id, ERROR_INVALID_DATA, 0);
    return;
  }
  if (length + 1 > mdb_out.size) {
    response_error2(request_id, ERROR_BUFFER_OVERFLOW, length + 1);
    return;
  }
  uint8_t const mst = mdb.state;
  if (mst != MDB_STATE_IDLE) {
    response_begin(request_id, RESPONSE_ERROR);
    response_f2(FIELD_MDB_RESULT, MDB_RESULT_BUSY, mst);
    response_finish();
    return;
  }

  // modified MDB state after this point,
  // so must call mdb_reset() on errors
  buffer_copy(&mdb_out, data, length);
  buffer_append(&mdb_out, memsum(data, length));

  mdb.request_id = request_id;
  mdb.state = MDB_STATE_SEND;
  mdb.start_clock = clock_10us();
  if (!uart_send_ready()) {
    response_begin(request_id, RESPONSE_ERROR);
    response_f2(FIELD_MDB_RESULT, MDB_RESULT_UART_SEND_BUSY, 0);
    response_finish();
    return;
  }
  timer1_set(mdb_timeout_ticks);
  mdb.retrying = false;

  UCSR0B = UCSRB_BASE | _BV(TXB80);
  UDR0 = data[0];

  // clear 9 bit for following bytes
  UCSR0B = UCSRB_BASE;

  // important to set index before UDRIE
  mdb_out.used = 1;
  UCSR0B = UCSRB_BASE | _BV(UDRIE0);
  return;
}

// Called from master_command()
// allowed to write response
static void mdb_bus_reset_begin(uint8_t const request_id,
                                uint16_t const duration) {
  uint8_t const mst = mdb.state;
  if (mst != MDB_STATE_IDLE) {
    response_begin(request_id, RESPONSE_ERROR);
    response_f2(FIELD_MDB_RESULT, MDB_RESULT_BUSY, mst);
    response_finish();
    return;
  }

  mdb.request_id = request_id;
  mdb.state = MDB_STATE_BUS_RESET;
  mdb.start_clock = clock_10us();
  UCSR0B = 0;                          // disable RX,TX
  bit_mask_set(DDRD, _BV(1));          // set TX pin to output
  bit_mask_clear(PORTD, _BV(PORTD1));  // pull TX pin low
  timer1_set(ms_to_timer16_p1024(duration));
}
static void mdb_bus_reset_finish(void) {
  // MDB bus reset is finished, make UART override TX pin again
  mdb_finish(MDB_RESULT_SUCCESS, 0);
}

ISR(USART_RX_vect) {
  timer1_stop();
  // both UCSR[AB] must be read before UDR
  uint8_t const csa = UCSR0A;
  uint8_t const csb = UCSR0B;
  uint8_t const data = UDR0;

  // receive error
  // bit_mask_test would be true only if all error types happened at once
  //
  uint8_t const err = csa & (_BV(FE0) | _BV(DOR0) | _BV(UPE0));
  if (err != 0) {
    if (bit_mask_test(err, _BV(FE0))) {
      mdb_finish(MDB_RESULT_UART_READ_ERROR, err & (~_BV(FE0)));
    } else if (bit_mask_test(err, _BV(DOR0))) {
      mdb_finish(MDB_RESULT_UART_READ_OVERFLOW, err & (~_BV(DOR0)));
    } else if (bit_mask_test(err, _BV(UPE0))) {
      mdb_finish(MDB_RESULT_UART_READ_PARITY, err & (~_BV(UPE0)));
    }
    return;
  }

  uint8_t const mst = mdb.state;
  if (!(mst == MDB_STATE_SEND || mst == MDB_STATE_RECV)) {
    // received data out of session
    mdb_finish(MDB_RESULT_UART_READ_UNEXPECTED, data);
    return;
  }

  if (!buffer_append(&mdb_in, data)) {
    mdb_finish(MDB_RESULT_RECEIVE_OVERFLOW, 0);
    return;
  }

  if (!bit_mask_test(csb, _BV(RXB80))) {
    mdb.in_chk += data;
    timer1_set(mdb_timeout_ticks);
    return;
  }
  uint8_t const len = mdb_in.length;
  if (len == 1) {
    // VMC ---ADD*---DAT---DAT---CHK-----
    // VMC ---ADD*---CHK--
    // Per -------------ACK*-
    // Per -------------NAK*-
    switch (data) {
      case MDB_ACK:
        mdb_finish(MDB_RESULT_SUCCESS, 0);
        break;
      case MDB_NAK:
        mdb_finish(MDB_RESULT_NAK, 0);
        break;
      default:
        mdb_finish(MDB_RESULT_INVALID_END, 0);
        break;
    }
  } else {
    mdb.state = MDB_STATE_RECV_END;
  }
}

// UART TX buffer space available
ISR(USART_UDRE_vect) {
  timer1_stop();
  // anti-volatile
  uint8_t const used = mdb_out.used;
  uint8_t const len = mdb_out.length;
  // debug mode
  if (used >= len) {
    mdb_finish(MDB_RESULT_SEND_OVERFLOW, used);
    return;
  }

  uint8_t const data = mdb_out.data[used];
  mdb_out.used++;

  // last byte is (about to be) sent
  if (mdb_out.used == len) {
    // disable (this) TX ready interrupt
    // enable TX-finished interrupt
    UCSR0B = UCSRB_BASE | _BV(TXCIE0);
  }

  UDR0 = data;

  timer1_set(mdb_timeout_ticks);
}

// UART TX completed
ISR(USART_TX_vect) {
  timer1_stop();
  UCSR0B = UCSRB_BASE;  // disable (this) TX completed interrupt

  uint8_t const mst = mdb.state;
  if (mst != MDB_STATE_SEND) {
    mdb_finish(MDB_RESULT_UART_TXC_UNEXPECTED, mst);
    return;
  }

  mdb_start_receive();
}

static inline uint16_t ms_to_timer16_p1024(uint16_t const ms) {
  // cheap way to get more accuracy without float
  uint32_t const ticks_expanded = ms * ((F_CPU << 4) / 1024000UL);
  uint32_t const ticks = (ticks_expanded >> 4) + 1;
  return ticks;
}

static void timer1_set(uint16_t const ticks) {
  timer1_stop();
  TCNT1 = 0 - ticks;
  TIMSK1 = _BV(TOIE1);
  TCCR1A = 0;
  TCCR1B = _BV(CS12) | _BV(CS10);  // prescale 1024, normal mode
}
static inline void timer1_stop(void) {
  TCCR1B = 0;
  TCCR1A = 0;
  TIMSK1 = 0;
}
ISR(TIMER1_OVF_vect) {
  timer1_stop();
  uint8_t const mst = mdb.state;  // anti-volatile
  if (mst == MDB_STATE_BUS_RESET) {
    mdb_bus_reset_finish();
  } else if ((mst == MDB_STATE_RECV) || (mst == MDB_STATE_SEND)) {
    // MDB timeout while sending or receiving
    // VMC ---ADD*---CHK------------ADD*---CHK------
    // Per --------------[silence]------------ACK*--
    mdb_finish(MDB_RESULT_TIMEOUT, mst);
  } else {
    // remove if timer is used by something other than MDB
    mdb_finish(MDB_RESULT_TIMER_CODE_ERROR, 1);
  }
}

// helpers

static inline bool uart_recv_ready(void) {
  return bit_mask_test(UCSR0A, _BV(RXC0));
}
static inline bool uart_send_ready(void) {
  return bit_mask_test(UCSR0A, _BV(UDRE0));
}

static void mdb_reset(void) {
  timer1_stop();
  buffer_clear_fast(&mdb_in);
  buffer_clear_fast(&mdb_out);
  memset((void *)&mdb, 0, sizeof(mdb_t));
  mdb.state = MDB_STATE_IDLE;
}

static void mdb_start_receive(void) {
  buffer_clear_fast(&mdb_in);
  mdb.in_chk = 0;
  mdb.state = MDB_STATE_RECV;
  timer1_set(mdb_timeout_ticks);
}

static void mdb_finish(mdb_result_t const result, uint8_t const error_data) {
  UCSR0B = UCSRB_BASE;
  mdb.result = result;
  mdb.error_data = error_data;
  mdb.duration = clock_10us() - mdb.start_clock;
  mdb.state = MDB_STATE_DONE;
}

#endif  // INCLUDE_MDB_C

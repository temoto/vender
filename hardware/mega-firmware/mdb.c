#ifndef INCLUDE_MDB_C
#define INCLUDE_MDB_C
#include <inttypes.h>
#include <stdbool.h>

/*
MDB timings:
  t = 1.0 mS inter-byte (max.)
  t = 5.0 mS response_t (max.)
  t = 100 mS break (min.)
  t = 200 mS setup (min.)
*/

typedef struct { uint8_t timeout; } mdb_stat_t;
typedef struct {
  mdb_state_t state;
  uint8_t command_id;
  uint8_t in_chk;
  bool retrying;
} mdb_t;

static buffer_t mdb_in;
static buffer_t mdb_out;
static mdb_t volatile mdb;
static mdb_stat_t volatile mdb_stat;

static void mdb_reset(void);
static void mdb_finish_0(response_t const code);
static void mdb_finish_1(response_t const code, uint8_t const data);
static void mdb_finish_2(response_t const code, uint8_t const data1,
                         uint8_t const data2);
static void mdb_finish_n(response_t const code, uint8_t const* const data,
                         uint8_t const length);
static void mdb_start_receive(void);

static bool uart_send_ready(void) { return bit_mask_test(UCSR0A, _BV(UDRE0)); }

// Baud Rate = 9600 +1%/-2% NRZ 9-N-1
static void mdb_init(void) {
  static uint8_t in_data[MDB_PACKET_SIZE];
  static uint8_t out_data[MDB_PACKET_SIZE];
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
  UCSR0B = _BV(RXEN0) | _BV(TXEN0)  // Enable receiver and transmitter
           | _BV(UCSZ02)            // 9 data bit
           | _BV(RXCIE0)            // Enable RX Complete Interrupt
      ;
}

static void mdb_tx_begin(uint8_t const command_id) {
  mdb.command_id = command_id;
  // TODO wait for byte is sent?
  if (!uart_send_ready()) {
    mdb_finish_1(RESPONSE_UART_SEND_BUSY, 0);
    return;
  }
  timer1_set(MDB_TIMEOUT);
  mdb.state = MDB_STATE_SEND;
  mdb.retrying = false;

  uint8_t const csb = _BV(RXEN0) | _BV(TXEN0) | _BV(UCSZ02) | _BV(RXCIE0);
  UCSR0B = csb | _BV(TXB80);
  UDR0 = mdb_out.data[0];

  // clear 9 bit for following bytes
  UCSR0B = csb;

  // important to set index before UDRIE
  mdb_out.used = 1;
  UCSR0B = csb | _BV(UDRIE0);
  return;
}

static bool mdb_step(void) {
  if (mdb.state == MDB_STATE_RECV_END) {
    uint8_t const len = mdb_in.length;
    if (len == 0) {
      mdb_finish_1(RESPONSE_MDB_CODE_ERROR, __LINE__);
      return true;
    }
    uint8_t const last_byte = mdb_in.data[len - 1];
    if (len == 1) {
      // VMC ---ADD*---DAT---DAT---CHK-----
      // VMC ---ADD*---CHK--
      // Per -------------ACK*-
      // Per -------------NAK*-
      if (last_byte == MDB_ACK) {
        mdb_finish_0(RESPONSE_MDB_SUCCESS);
      } else if (last_byte == MDB_NAK) {
        mdb_finish_1(RESPONSE_MDB_NAK, last_byte);
      } else {
        mdb_finish_1(RESPONSE_MDB_INVALID_END, last_byte);
      }
      return true;
    }

    if (last_byte != mdb.in_chk) {
      if (mdb.retrying) {
        // invalid checksum even after retry
        // VMC ---ADD*---DAT--CHK----------------RET----------------NAK--
        // Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----
        UDR0 = MDB_NAK;
        mdb_finish_n(RESPONSE_MDB_INVALID_CHK, mdb_in.data, len);
        return true;
      } else {
        // VMC ---ADD*---DAT--CHK----------------RET----------------ACK--
        // Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----
        UDR0 = MDB_RET;
        mdb.retrying = true;
        mdb_start_receive();
        return true;
      }
    }

    // VMC ---ADD*---CHK----------------ACK-
    // Per -------------DAT---DAT---CHK*----
    UDR0 = MDB_ACK;
    mdb_finish_n(RESPONSE_MDB_SUCCESS, mdb_in.data, len - 1);
    return true;
  }
  return false;
}

static void mdb_bus_reset_begin(uint8_t const command_id,
                                uint16_t const duration) {
  mdb.command_id = command_id;
  mdb.state = MDB_STATE_BUS_RESET;
  bit_mask_clear(UCSR0B, _BV(TXEN0));  // disable UART TX
  bit_mask_set(DDRD, _BV(1));          // set TX pin to output
  bit_mask_clear(PORTD, _BV(PORTD1));  // pull TX pin low
  timer1_set(duration);
}
static void mdb_bus_reset_finish(void) {
  // MDB bus reset is finished, re-enable UART TX
  bit_mask_set(UCSR0B, _BV(TXEN0));
  mdb_finish_0(RESPONSE_MDB_SUCCESS);
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
      mdb_finish_1(RESPONSE_UART_READ_ERROR, err);
    } else if (bit_mask_test(err, _BV(DOR0))) {
      mdb_finish_1(RESPONSE_UART_READ_OVERFLOW, err);
    } else if (bit_mask_test(err, _BV(UPE0))) {
      mdb_finish_1(RESPONSE_UART_READ_PARITY, err);
    }
    return;
  }

  // received data out of session
  if (mdb.state != MDB_STATE_RECV) {
    mdb_finish_2(RESPONSE_UART_READ_UNEXPECTED, data, mdb.state);
    return;
  }

  if (!buffer_append(&mdb_in, data)) {
    mdb_finish_1(RESPONSE_MDB_RECEIVE_OVERFLOW, data);
    return;
  }
  if (bit_mask_test(csb, _BV(RXB80))) {
    mdb.state = MDB_STATE_RECV_END;
  } else {
    mdb.in_chk += data;
    timer1_set(MDB_TIMEOUT);
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
    mdb_finish_1(RESPONSE_MDB_SEND_OVERFLOW, used);
    return;
  }

  uint8_t const data = mdb_out.data[used];
  mdb_out.used++;

  // last byte is (about to be) sent
  if (mdb_out.used == len) {
    // variable hoop to make volatile UCSR0B assignment once
    uint8_t csb = UCSR0B;
    bit_mask_clear(csb, _BV(UDRIE0));  // disable (this) TX ready interrupt
    bit_mask_set(csb, _BV(TXCIE0));    // enable TX-finished interrupt
    UCSR0B = csb;
  }

  UDR0 = data;

  timer1_set(MDB_TIMEOUT);
}

// UART TX completed
ISR(USART_TX_vect) {
  timer1_stop();
  bit_mask_clear(UCSR0B, _BV(TXCIE0));  // disable (this) TX completed interrupt

  uint8_t const mst = mdb.state;
  if (mst != MDB_STATE_SEND) {
    mdb_finish_2(RESPONSE_MDB_CODE_ERROR, __LINE__, mst);
    return;
  }

  mdb_start_receive();
}

static void timer1_set(uint16_t const ms) {
  timer1_stop();
  uint16_t const per_ms = (F_CPU / 1024000UL);
  uint16_t const cnt = 0 - (ms * per_ms);
  TCNT1 = cnt;
  TIMSK1 = _BV(TOIE1);
  TCCR1A = 0;
  TCCR1B = _BV(CS12) | _BV(CS10);  // prescale 1024, normal mode
}
static void timer1_stop(void) {
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
    mdb_stat.timeout++;
    uint8_t const time_passed = MDB_TIMEOUT;  // FIXME get real value
    mdb_finish_1(RESPONSE_MDB_TIMEOUT, time_passed);
  } else {
    // remove if timer is used by something other than MDB
    master_out_1(mdb.command_id, RESPONSE_TIMER_CODE_ERROR, 1);
  }
}

// helpers

static void mdb_reset(void) {
  timer1_stop();
  memset((void*)&mdb_stat, 0, sizeof(mdb_stat_t));
  buffer_clear_fast(&mdb_in);
  buffer_clear_fast(&mdb_out);
  mdb.command_id = 0;
  mdb.in_chk = 0;
  mdb.retrying = false;
  mdb.state = MDB_STATE_IDLE;
}

static void mdb_start_receive(void) {
  buffer_clear_fast(&mdb_in);
  mdb.in_chk = 0;
  mdb.state = MDB_STATE_RECV;
  timer1_set(MDB_TIMEOUT);
}

static void mdb_finish_0(response_t const code) {
  master_out_0(mdb.command_id, code);
  mdb_reset();
}
static void mdb_finish_1(response_t const code, uint8_t const data) {
  master_out_1(mdb.command_id, code, data);
  mdb_reset();
}
static void mdb_finish_2(response_t const code, uint8_t const data1,
                         uint8_t const data2) {
  master_out_2(mdb.command_id, code, data1, data2);
  mdb_reset();
}
static void mdb_finish_n(response_t const code, uint8_t const* const data,
                         uint8_t const length) {
  master_out_n(mdb.command_id, code, data, length);
  mdb_reset();
}

#endif  // INCLUDE_MDB_C

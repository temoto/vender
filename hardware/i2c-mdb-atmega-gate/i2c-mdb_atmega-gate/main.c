/*
 * i2c-mdb_atmega-gate.c
 *
 * Created: 18/06/2018 21:16:04
 * Author : Alex
 */ 

#include <avr/io.h>
#include <avr/interrupt.h>
#include <stdbool.h>
#include <inttypes.h>

#define F_CPU 16000000UL // Clock Speed
#define USART_BAUDRATE 9600
#define BAUD_PRESCALE (((F_CPU / (USART_BAUDRATE * 16UL))) - 1)

#define MDB_TRANSMIT_READY 0x01
#define MDB_RECEIVE_READY  0x02
#define MDB_SESSION_READY  0x04

#define MDB_ACK 0x00
#define MDB_RET 0xAA
#define MDB_NAK 0xFF

#define ERROR_TIMEOUT										0b00000001
#define ERROR_START_FRAME								0b00000010
#define ERROR_RECIEVED_WITHOUT_SESSION	0b00000100
#define ERROR_RECIEVED_BYTE							0b00001000
#define ERROR_RESPOND_TIMEOUT						0b00010000
#define ERROR_CHECKSUM									0b00100000
#define ERROR_RECEIVED_LENTH						0b01000000

#define COMMAND_COMPLITE					0x01
#define COMMAND_MDB_WITHOUT_RET		0x02
#define COMMAND_MDB_WITH_RET			0x02

volatile uint8_t data_in[36];				// buffer for data from mdb
volatile uint8_t data_in_len;				// recieved bytes
volatile uint8_t data_in_checksum;  // recieved checksum

volatile uint8_t data_out[36];			// buffer for data to mdb
volatile uint8_t data_out_len;			// Number of bytes to transfer total bytes to количество байт для передачи
volatile uint8_t data_out_byte;			// total of bytes to transfer
volatile uint8_t data_out_checksum; // checksum 


volatile uint8_t hLockData;
volatile uint8_t mdb_error;
volatile uint8_t command;
// hLockData &= ~setbit
// hLockData |= clearbit
volatile unsigned char data_count;
const char testdata[3] = {1,14,55};

/*   
Baud Rate = 9600 +1%/-2% NRZ
t = 1.0 mS inter-byte (max.)
t = 5.0 mS response (max.)
t = 100 mS break (min.)
t = 200 mS setup (min.)

    Typical Session Examples 
VMC ---ADD*---CHK--
Per -------------ACK*-

VMC ---ADD*---CHK----------------ACK-
Per -------------DAT---DAT---CHK*----

VMC ---ADD*---DAT---DAT---CHK-----
Per -------------------------ACK*-

VMC ---ADD*---DAT--CHK----------------RET----------------ACK--
Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----

  silence max 5mc
VMC ---ADD*---CHK------------ADD*---CHK------
Per --------------[silence]------------ACK*--
*/
void MDB_Init();
void MDB_Start_session ();
void MDB_Stop_session ()
void timer_set(uint16_t ms);
void timer_stop();

volatile bool update = false;

int main(void)
{
	MDB_Init();
	data_out[0] = 1;
	data_out[1] = 14;
	data_out[2] = 251;
	data_out_len = 3;
	uint8_t aa = 0b11111111;
	uint8_t ab = (aa >> 1) & 0x01;
	
	MDB_Start_session ();
//hLockData &= ~(MDB_SESSION_READY | MDB_TRANSMIT_READY | MDB_RECEIVE_READY);
    while (1) 
    {
			if (hLockData & (MDB_SESSION_READY || MDB_TRANSMIT_READY) && data_out_byte) {
				// transmit
				if (data_out_byte < data_out_len) { //send next byte
					uint8_t data;
					data = data_out[data_out_byte];
					data_out_checksum += data;
					UDR0 = data;
					data_out_byte ++;
				} else { // send last byte (checksum)
					UDR0 = data_out_checksum;
					data_out_checksum = 0;
					data_out_byte = 0;
					data_in_checksum = 0;
					data_in_len = 0;
				}
				hLockData &= ~MDB_TRANSMIT_READY;
				timer_set(5);
			} // end transmit
			if ( hLockData & (MDB_SESSION_READY || MDB_RECEIVE_READY) && !mdb_error ) { // receive
				// (UCSRnA & (1<<RXCn))  check received flag
				uint8_t status = UCSR0A;
				uint8_t status9 = UCSR0B;
				uint8_t	data = UDR0;
				// check error on recieved
				if ( status & (1<<FE0)|(1<<DOR0)|(1<<UPE0) ) {
					mdb_error |= ERROR_RECIEVED_BYTE;
				} else {
					if (status9 & (1<<TXB80)) { // check 9 bit
						if (data_in_len == 0 && data == MDB_ACK){	
							//	VMC ---ADD*---CHK--
							//	Per -------------ACK*-
							UDR0 = MDB_ACK;
							MDB_Stop_session();
						} else {
							if ( data_in_checksum == data ) {
								//	VMC ---ADD*---CHK----------------ACK-
								//	Per -------------DAT---DAT---CHK*----
								UDR0 = MDB_ACK;
								MDB_Stop_session();
							} else { 
								// checksum error. send RET
								if (command == COMMAND_MDB_WITH_RET) {
									//VMC ---ADD*---DAT--CHK----------------RET----------------ACK--
									//Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----
									UDR0 = MDB_RET;
									command == COMMAND_MDB_WITHOUT_RET;
									data_in_checksum = 0;
									data_in_len = 0;
								} else {
									UDR0 = MDB_ACK;
									MDB_Stop_session();
									mdb_error |= ERROR_CHECKSUM;
								}
							}
						}
					} else {
						// receive next byte
						data_in_checksum += data;
						data_in[data_in_len] = data;
						data_in_len ++;
						if(data_in_len >36) {
							mdb_error |= ERROR_RECEIVED_LENTH;
						} else {
							hLockData &= ~MDB_RECEIVE_READY;
							timer_set(5);
						}
					}
				}
			} // end receive
			if (mdb_error){
				MDB_Stop_session();
			}
    }
}


void MDB_Init()
 {
  //set baud rate
  UBRR0H = (BAUD_PRESCALE >> 8);
  UBRR0L = BAUD_PRESCALE;
  //Enable receiver and transmitter
  UCSR0B = (1 << RXEN0) | (1 << TXEN0);
  //Set session format: 8 data, 1 stop bit
  UCSR0C = (3 << UCSZ00);
  //Activates 9th data bit
  UCSR0B |= (1 << UCSZ02);
  // RX Complete Interrupt Enable 
  UCSR0B |= (1 << RXCIE0);
  // TX Complete Interrupt Enable
  UCSR0B |= (1 << TXCIE0);
  
  //Global Interrupt Enable
  sei();
}

//void MDB_Flush( void )
//{
	//unsigned char dummy;
	//while ( UCSR0A & (1<<RXC0) ) dummy = UDR0;
//}void MDB_Stop_session (){
	timer_stop();
	data_out_checksum = 0;
	data_out_byte = 0;
	data_in_checksum = 0;
	data_in_len = 0;
	hLockData &= ~(MDB_SESSION_READY || MDB_TRANSMIT_READY || MDB_RECEIVE_READY);
}

void MDB_Start_session ()
{
	hLockData |= MDB_SESSION_READY;
	data_in_checksum = 0;
	data_in_len = 0;

	data_out_checksum = data_out[0];
	UCSR0B |= (1<<TXB80); //set 9 bit
	timer_set(5);
	if((UCSR0A & (1 << UDRE0))) {
			UDR0 = data_out[0];
			UCSR0B &= ~(1<<TXB80); //clear 9 bit
			data_out_byte = 1;
			if((UCSR0A & (1 << UDRE0))) hLockData |= MDB_TRANSMIT_READY; // for use second recieve buffer byte
	} else {
		mdb_error |= ERROR_START_FRAME;
	}
}

void timer_set(uint16_t ms)
{
	TCCR1B |= (1<<CS11) | (1<<CS10);  // prescale 64 & CTC mode
	TIMSK1 |= (1<<TOIE1);
	TCNT1 = 65536-(ms*250);
}

void timer_stop()
{
	hLockData &= ~(MDB_SESSION_READY | MDB_TRANSMIT_READY | MDB_RECEIVE_READY);
	TIMSK1 &= ~(1 << TOIE1);
}

ISR(TIMER1_OVF_vect){
	mdb_error |= ERROR_TIMEOUT;
	//timer_stop();
	TIMSK1 &= ~(1 << TOIE1);
}


ISR(USART_RX_vect)
{
	if(hLockData & MDB_SESSION_READY) {
		hLockData |= MDB_RECEIVE_READY;
	} else {
		// error received data in not session
		uint8_t tmp = UDR0; // Clear the receive buffer
		mdb_error |= ERROR_RECIEVED_WITHOUT_SESSION;
	}
}

ISR(USART_TX_vect)
{
	if(hLockData & MDB_SESSION_READY) {
		hLockData |= MDB_TRANSMIT_READY;
	}
}
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

#define MDB_9BIT					 0b00000001
#define MDB_TRANSMIT_READY 0b00000010
#define MDB_RECEIVE_READY  0b00000100
#define MDB_SESSION_READY  0b00001000

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
#define ERROR_RECEIVED_NOT_ACK					0b10000000

#define COMMAND_COMPLITE					0x01
#define COMMAND_MDB_WITHOUT_RET		0x02
#define COMMAND_MDB_WITH_RET			0x02

volatile uint8_t data_in[36];				// buffer for data from mdb
volatile uint8_t data_in_len;				// recieved bytes
volatile uint8_t data_in_checksum;  // recieved checksum

volatile uint8_t data_out[36];			// buffer for data to mdb
volatile uint8_t data_out_len;			// Number of bytes to transfer total bytes
volatile uint8_t data_out_byte;			// total of bytes to transfer
volatile uint8_t data_out_checksum; // checksum


//volatile uint8_t hLockData;
volatile uint8_t MDBState;
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
void MDB_Stop_session ();
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
	uint8_t aa = 0;
	uint8_t ab = (aa >> 1) & 0x01;
	
		
	MDB_Start_session ();
	while (1)
	{
		if ( (MDBState & MDB_SESSION_READY) && (MDBState & MDB_TRANSMIT_READY) && (data_out_byte < data_out_len) && !mdb_error ) {
			// transmit
				uint8_t data;
				data = data_out[data_out_byte];
				data_out_checksum += data;
				UDR0 = data;
				data_out_byte ++;
				MDBState &= ~MDB_TRANSMIT_READY;
				timer_set(5);
		}
		if ( (MDBState & MDB_SESSION_READY) && (MDBState & MDB_TRANSMIT_READY) && (data_out_byte == data_out_len)  && !mdb_error )  {				
				// send last byte (checksum)
				data_out_byte ++; // set overflow. ( disable handler)
				UDR0 = data_out_checksum;
				data_in_checksum = 0;
				data_in_len = 0;
				timer_set(5);
				MDBState &= ~MDB_TRANSMIT_READY;
		}
		if ( (MDBState & MDB_SESSION_READY) && (MDBState & MDB_RECEIVE_READY) && !(MDBState & MDB_9BIT) && !mdb_error){
			// receive next byte
			uint8_t	data = UDR0;
			if(data_in_len >36) { //max packet size 36 byte
				mdb_error |= ERROR_RECEIVED_LENTH;
				MDB_Stop_session();
				return;
			} 
			data_in_checksum += data;
			data_in[data_in_len] = data;
			data_in_len ++;
			MDBState &= ~MDB_RECEIVE_READY;
			timer_set(5);
		}

		if ( (MDBState & MDB_SESSION_READY) && (MDBState & MDB_RECEIVE_READY) && (MDBState & MDB_9BIT) && (data_in_len == 0) && !mdb_error){
			//	VMC ---ADD*---CHK--
			//	Per -------------ACK*-
			uint8_t	data = UDR0;
			if (data != MDB_ACK) {
				// error. returned not ACK
				mdb_error |= ERROR_RECEIVED_NOT_ACK;
			}
			command = COMMAND_COMPLITE;
			MDB_Stop_session();
		}

		if ( (MDBState & MDB_SESSION_READY) && (MDBState & MDB_RECEIVE_READY) && (MDBState & MDB_9BIT) && (data_in_len > 0) && !mdb_error){
			//	VMC ---ADD*---CHK----------------ACK-
			//	Per -------------DAT---DAT---CHK*----
			uint8_t	data = UDR0;
			if (data != data_in_checksum && command == COMMAND_MDB_WITHOUT_RET){
				UDR0 = MDB_ACK;
				mdb_error |= ERROR_CHECKSUM;
				MDB_Stop_session();
				return;
			}
			if (data != data_in_checksum && command == COMMAND_MDB_WITH_RET){
				//VMC ---ADD*---DAT--CHK----------------RET----------------ACK--
				//Per ------------------DAT---DAT---CHK*---DAT---DAT---CHK*-----
				UDR0 = MDB_RET;
				command = COMMAND_MDB_WITHOUT_RET;
				data_in_checksum = 0;
				data_in_len = 0;
				MDBState &= ~MDB_RECEIVE_READY;
				timer_set(5);
				return;
			}
			UDR0 = MDB_ACK;
			command = COMMAND_COMPLITE;
			MDB_Stop_session();
		}
		if(mdb_error) {
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
	MDBState = 0;
}

void MDB_Start_session ()
{
	data_out_byte = 0;
	data_out_checksum = data_out[0];
	timer_set(5);
	if(!(UCSR0A & (1 << UDRE0))) {
		mdb_error |= ERROR_START_FRAME;
		return;
	}
	MDBState |= MDB_SESSION_READY;
	UCSR0B |= (1<<TXB80); //set 9 bit
	UDR0 = data_out[0];
	UCSR0B &= ~(1<<TXB80); //clear 9 bit
	data_out_byte = 1;
	if((UCSR0A & (1 << UDRE0))) {
		MDBState |= MDB_TRANSMIT_READY; // for use second recieve buffer byte
	}
}

void timer_set(uint16_t ms) {
	TCCR1B |= (1<<CS11) | (1<<CS10);  // prescale 64 & CTC mode
	TIMSK1 |= (1<<TOIE1);
	TCNT1 = 65536-(ms*250);
}

void timer_stop() {
	MDBState = 0;
	TIMSK1 &= ~(1 << TOIE1);
}

ISR(TIMER1_OVF_vect){
	if (MDBState != 0) {
		mdb_error |= ERROR_TIMEOUT;	
		MDBState = 0;
	}
	TIMSK1 &= ~(1 << TOIE1);
}


ISR(USART_RX_vect)
{

	if(MDBState & MDB_SESSION_READY) {
		//check receive error
		if ( UCSR0A & (1<<FE0)|(1<<DOR0)|(1<<UPE0) ) {
			mdb_error |= ERROR_RECIEVED_BYTE;
			MDBState = 0;
			return;
		}
		MDBState &= ~MDB_9BIT;							// clear 9 bit
		MDBState += (UCSR0B & (1<<TXB80));	// if 9 bit true then set
		MDBState |= MDB_RECEIVE_READY;
	} else {
		// error received data in not session
		volatile uint8_t tmp = UDR0; // Clear the receive buffer
		mdb_error |= ERROR_RECIEVED_WITHOUT_SESSION;
	}
}

ISR(USART_TX_vect)
{
	if(MDBState & MDB_SESSION_READY) {
		MDBState |= MDB_TRANSMIT_READY;
	}
}
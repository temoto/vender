#!/usr/bin/make
F_CPU = 16000000
mmcu = atmega328p

cfiles   := main.c crc.c ring.c
hexfiles := $(patsubst %.c,%.hex,$(cfiles))

avr_gcc = avr-gcc
avr_objcopy = avr-objcopy
avr_size = avr-size
avrdude = avrdude

cflags_common = -Wall -Werror -std=c11 -pipe
CFLAGS = -mmcu=$(mmcu) -DF_CPU=$(F_CPU) -O2 $(cflags_common)
MAPFLAGS = -Wl,-Map,

.PHONY : clean
.DEFAULT_GOAL := all

all: crc_test ring_test main.hex

upload: main.hex
	$(avrdude) -p $(mmcu) -P usb -c dragon_isp -U flash:w:$<
upload-a: main.hex
	$(avrdude) -p $(mmcu) -P /dev/cu.usbmodemfa141 -c arduino -U flash:w:$<

fuse:
	$(avrdude) -p $(mmcu) -P usb -c dragon_isp -U lfuse:w:0x62:m -U hfuse:w:0xd9:m -U efuse:w:0xff:m

clean:
	rm -rf *.dSYM/ *.elf *.hex *.o crc_test ring_test

main.hex: main.elf
	$(avr_objcopy) -O ihex $< $@

main.elf: $(cfiles)
	$(avr_gcc) $(CFLAGS) -o $@ $^
	$(avr_size) --common $@

crc_test:
	$(CC) $(cflags_common) -DDEBUG -DTEST -g -O0 -o $@ crc_test.c crc.c
	./crc_test

ring_test:
	$(CC) $(cflags_common) -DDEBUG -DTEST -g -O0 -o $@ ring.c
	./ring_test

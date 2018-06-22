#!/usr/bin/make
# F_CPU = 16000000
F_CPU = 7372800
mmcu = atmega328p
# mmcu = atmega128

avr_gcc = avr-gcc
avr_objcopy = avr-objcopy
avr_size = avr-size
avrdude = avrdude

cflags_common = -Wall -Werror -std=c11 -pipe
CFLAGS = -mmcu=$(mmcu) -DF_CPU=$(F_CPU) -O2 $(cflags_common)
MAPFLAGS = -Wl,-Map,

.PHONY : all clean crc_test main_test upload upload-a fuse
.DEFAULT_GOAL := all

all: format crc_test main.hex

upload: main.hex
	$(avrdude) -p $(mmcu) -P usb -c dragon_isp -U flash:w:$<
upload-a: main.hex
	$(avrdude) -p $(mmcu) -P /dev/cu.usbmodemfa141 -c arduino -U flash:w:$<

fuse:
	$(avrdude) -p $(mmcu) -P usb -c dragon_isp -U lfuse:w:0xff:m -U hfuse:w:0xdb:m -U efuse:w:0x07:m

clean:
	rm -rf *.dSYM/ *.elf *.hex *.lst *.o *.s crc_test

format:
	-clang-format -style=Google -i *.c *.h

main.hex: main.elf
	$(avr_objcopy) -O ihex $< $@

main.elf: main.c crc.c crc.h
	$(avr_gcc) $(CFLAGS) -o $@ $^
	$(avr_size) --common $@

crc_test: crc.c crc.h crc_test.c
	$(CC) $(cflags_common) -DDEBUG -DTEST -g -O0 -o $@ crc_test.c crc.c
	./crc_test

main_test: main.c crc.c crc.h buffer.c
	$(avr_gcc) $(CFLAGS) -DDEBUG -DTEST -g -Og -o $@ main.c crc.c
	avr-run -v -m11 ./main_test

%.lst: %.c
	$(avr_gcc) $(CFLAGS) -fverbose-asm -g -c -Wa,-alhnd $^ >$@

asm: crc.lst main.lst

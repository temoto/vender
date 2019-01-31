mmcu = atmega328p
# mmcu = atmega128

avr_gcc = avr-gcc
avr_objcopy = avr-objcopy
avr_size = avr-size
avrdude = avrdude

cflags_common = -std=c11 -Wall -Wextra -Werror -pedantic -pipe -finline-functions -findirect-inlining -fno-predictive-commoning
CFLAGS = -mmcu=$(mmcu) -O2 $(cflags_common)
MAPFLAGS = -Wl,-Map,

.PHONY : all clean main_test upload upload-a fuse
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

main.elf: main.c crc.c
	$(avr_gcc) $(CFLAGS) -o $@ $^
	$(avr_size) --common $@
	cp main.elf i2c-mdb_atmega-gate.elf

main.o: main.c buffer.c crc.c crc.h main.h mdb.c twi.c

crc_test: crc.c crc.h crc_test.c
	$(CC) $(cflags_common) -DDEBUG -DTEST -g -O0 -o $@ crc_test.c crc.c
	./crc_test

main_test: main.c
	$(avr_gcc) $(CFLAGS) -DDEBUG -DTEST -g -Og -o $@ main.c
	avr-run -v -m11 ./main_test

%.lst: %.c
	$(avr_gcc) $(CFLAGS) -fverbose-asm -g -c -Wa,-alhnd $^ >$@

asm: main.lst

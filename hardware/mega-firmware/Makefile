# mmcu = atmega328p
mmcu = atmega168

avr_gcc = avr-gcc
avr_objcopy = avr-objcopy
avr_size = avr-size
avrdude = avrdude

cflags_common = -std=c11 -Wall -Wextra -Werror -pedantic -pipe -flto
CFLAGS = -mmcu=$(mmcu) -O2 -mbranch-cost=2 -g3 $(cflags_common)

.PHONY : all asm clean main_test upload upload-a fuse
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
	cksum main.hex

main.elf: main.c
	$(avr_gcc) $(CFLAGS) -o $@ $^
	$(avr_size) --common $@
	cksum main.elf
	cp main.elf mega-firmware.elf

crc_test: crc.c crc.h crc_test.c
	$(CC) $(cflags_common) -DDEBUG -DTEST -g -O0 -o $@ crc_test.c crc.c
	./crc_test

main_test: main.c
	$(avr_gcc) $(CFLAGS) -DDEBUG -DTEST -g -Og -o $@ main.c
	avr-run -v -m11 ./main_test

asm: main.elf
	avr-objdump -D -j .text -m avr5 main.elf

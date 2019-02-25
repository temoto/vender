package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/hardware/mdb"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	devicePath := cmdline.String("device", "/dev/ttyAMA0", "")
	iodinPath := cmdline.String("iodin", "./iodin", "Path to iodin executable")
	megaI2CBus := cmdline.Uint("mega-i2c-bus", 0, "mega I2C bus number")
	megaI2CAddr := cmdline.Uint("mega-i2c-addr", 0x78, "mega I2C address")
	megaPin := cmdline.Uint("mega-pin", 23, "mega notify pin")
	uarterName := cmdline.String("io", "file", "file|iodin|mega")
	cmdline.Parse(os.Args[1:])

	log := log2.NewStderr(log2.LDebug)

	var uarter mdb.Uarter
	switch *uarterName {
	case "file":
		uarter = mdb.NewFileUart(log)
	case "iodin":
		iodin, err := iodin.NewClient(*iodinPath)
		if err != nil {
			log.Fatal(errors.Trace(err))
		}
		uarter = mdb.NewIodinUart(iodin)
	case "mega":
		mega, err := mega.NewClient(byte(*megaI2CBus), byte(*megaI2CAddr), *megaPin, log)
		if err != nil {
			log.Fatal(errors.Trace(err))
		}
		uarter = mdb.NewMegaUart(mega)
	default:
		log.Fatalf("invalid -io=%s", *uarterName)
	}
	defer uarter.Close()

	m, err := mdb.NewMDB(uarter, *devicePath, log)
	if err != nil {
		log.Fatalf("mdb open: %v", errors.ErrorStack(err))
	}
	stdin := bufio.NewReader(os.Stdin)
	defer os.Stdout.WriteString("\n")
	for {
		fmt.Fprintf(os.Stdout, "> ")
		bline, _, err := stdin.ReadLine()
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatal(errors.ErrorStack(err))
		}
		line := string(bline)
		if len(line) == 0 {
			continue
		}

		words := strings.Split(line, " ")
		iteration := uint64(1)
	wordLoop:
		for _, word := range words {
			log.Debugf("(%d)%s", iteration, word)
			switch {
			case word == "help":
				log.Infof(`syntax: commands separated by whitespace
- break    MDB bus reset (TX high for 200ms, wait for 500ms)
- log=yes  enable debug logging
- log=no   disable debug logging
- lN       loop N times all commands on this line
- sN       pause N milliseconds
- tXX...   transmit MDB block from hex XX..., show response
`)
			case word == "break":
				m.BreakCustom(200*time.Millisecond, 500*time.Millisecond)
			case word == "log=yes":
				m.Log.SetLevel(log2.LDebug)
			case word == "log=no":
				m.Log.SetLevel(log2.LError)
			case word[0] == 'l':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					iteration++
					if iteration <= i {
						goto wordLoop
					}
				}
			case word[0] == 's':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					time.Sleep(time.Duration(i) * time.Millisecond)
				}
			case word[0] == 't':
				response := new(mdb.Packet)
				request, err := mdb.PacketFromHex(word[1:], true)
				if err == nil {
					err = m.Tx(request, response)
					log.Debugf("< %s", response.Format())
				}
				if err != nil {
					log.Errorf(errors.ErrorStack(err))
					if !errors.IsTimeout(err) {
						break wordLoop
					}
				}
			default:
				log.Errorf("error: invalid command: '%s'", word)
				break wordLoop
			}
		}
	}
}

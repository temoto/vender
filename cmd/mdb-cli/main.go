package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/iodin-client"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
)

func main() {
	devicePath := flag.String("device", "/dev/ttyAMA0", "")
	iodinPath := flag.String("iodin", "./iodin", "Path to iodin executable")
	uarterName := flag.String("io", "", "file|iodin")
	flag.Parse()

	var uarter mdb.Uarter
	switch *uarterName {
	case "", "file":
		uarter = mdb.NewFileUart()
	case "iodin":
		iodin, err := iodin.NewClient(*iodinPath)
		if err != nil {
			log.Fatal(errors.Trace(err))
		}
		uarter = mdb.NewIodinUart(iodin)
	default:
		log.Fatalf("invalid -io=%s", *uarterName)
	}
	defer uarter.Close()

	m, err := mdb.NewMDB(uarter, *devicePath)
	if err != nil {
		log.Fatalf("mdb open: %v", errors.ErrorStack(err))
	}
	m.SetLog(log.Printf)
	stdin := bufio.NewReader(os.Stdin)
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

		words := strings.Split(line, " ")
		iteration := uint64(1)
	wordLoop:
		for _, word := range words {
			log.Printf("(%d)%s", iteration, word)
			switch {
			case word == "help":
				log.Printf(`syntax: commands separated by whitespace
- break    MDB bus reset (TX high for 200ms, wait for 500ms)
- log=yes  enable debug logging
- log=no   disable debug logging
- lN       loop N times all commands on this line
- sN       pause N milliseconds
- tXX...   send MDB block from hex XX...
`)
			case word == "break":
				m.BreakCustom(200*time.Millisecond, 500*time.Millisecond)
			case word == "log=yes":
				m.SetLog(log.Printf)
			case word == "log=no":
				m.SetLog(helpers.Discardf)
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
				request := mdb.PacketFromHex(word[1:])
				response := new(mdb.Packet)
				if request != nil {
					err = m.Tx(request, response)
					response.Logf("< %s")
				}
				if err != nil {
					log.Printf(errors.ErrorStack(err))
					if !errors.IsTimeout(err) {
						break wordLoop
					}
				}
			default:
				log.Printf("error: invalid command: '%s'", word)
				break wordLoop
			}
		}
	}
}

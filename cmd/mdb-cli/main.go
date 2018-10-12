package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
)

func main() {
	uarterName := flag.String("io", "", "file|cgo|cproc|rustlib")
	flag.Parse()

	var uarter mdb.Uarter
	switch *uarterName {
	case "", "file":
		uarter = mdb.NewFileUart()
	default:
		log.Fatalf("invalid -io=%s", *uarterName)
	}
	devicePath := "/dev/ttyAMA0" // TODO flag
	m, err := mdb.NewMDB(uarter, devicePath, 0)
	if err != nil {
		log.Fatalf("mdb open: %v", errors.ErrorStack(err))
	}
	m.SetLog(log.Printf)
	stdin := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stdout, "> ")
		bline, _, err := stdin.ReadLine()
		if err != nil {
			log.Fatal(errors.ErrorStack(err))
		}
		line := string(bline)

		words := strings.Split(line, " ")
		iteration := uint64(1)
	wordLoop:
		for _, word := range words {
			log.Printf("(%d)%s", iteration, word)
			switch {
			case word == "break":
				m.BreakCustom(200, 500)
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
					log.Fatal(errors.ErrorStack(err))
				}
			default:
				log.Printf("error: invalid command: '%s'", word)
				break wordLoop
			}
		}
	}
}

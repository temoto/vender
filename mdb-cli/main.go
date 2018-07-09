package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/temoto/vender/mdb"
)

func main() {
	mfile, _ := mdb.NewMDB(mdb.NewFileUart(), "/dev/ttyAMA0", 9600)
	mfast, _ := mdb.NewMDB(mdb.NewFastUart(), "/dev/ttyAMA0", 9600)
	_, _ = mfile, mfast
	m := mfile
	m.Debug = true
	stdin := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stdout, "> ")
		bline, _, err := stdin.ReadLine()
		if err != nil {
			log.Fatal(err)
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
			case word == "debug=yes":
				m.Debug = true
			case word == "debug=no":
				m.Debug = false
			case word[0] == 'l':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(err)
				} else {
					iteration++
					if iteration <= i {
						goto wordLoop
					}
				}
			case word[0] == 's':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(err)
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
					log.Fatal(err)
				}
			default:
				log.Printf("error: invalid command: '%s'", word)
				break wordLoop
			}
		}
	}
}

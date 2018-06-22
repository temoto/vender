package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"github.com/temoto/vender/mdb"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	m := new(mdb.MDB)
	err := m.Open("/dev/ttyAMA0", 9600, 1)
	if err != nil {
		panic(err)
	}
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
			switch word[0] {
			case 'b':
				m.BreakCustom(200, 500)
			case 'l':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(err)
				} else {
					iteration++
					if iteration <= i {
						goto wordLoop
					}
				}
			case 's':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(err)
				} else {
					time.Sleep(time.Duration(i) * time.Millisecond)
				}
			case 't':
				if bout, err := hex.DecodeString(word[1:]); err != nil {
					log.Fatal(err)
				} else {
					m.Tx(bout, nil)
				}
			}
		}
	}
}

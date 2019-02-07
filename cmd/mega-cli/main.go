package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	mega "github.com/temoto/vender/hardware/mega-client"
)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	i2cBusNo := cmdline.Uint("i2cbus", 0, "")
	addr := cmdline.Uint("addr", 0x78, "")
	pin := cmdline.Uint("pin", 23, "")
	cmdline.Parse(os.Args[1:])

	log.SetFlags(log.Lshortfile | log.Ltime | log.Lmicroseconds)

	client, err := mega.NewClient(byte(*i2cBusNo), byte(*addr), *pin)
	if err != nil {
		log.Fatal(errors.Trace(err))
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
			log.Printf("(%d)%s", iteration, word)
			switch {
			case word == "help":
				log.Printf(`syntax: commands separated by whitespace
- tick=yes|no  enable|disable backup reading every second
- pin=yes|no  enable|disable reading when pin signal is detected
- pXX...   send proper packet from hex XX... and receive response
- rN       (debug) read N bytes
- sN       pause N milliseconds
- tXX...   (debug) transmit bytes from hex XX...
`)
			case word == "tick=yes":
				fallthrough
			case word == "tick=no":
				fallthrough
			case word == "pin=yes":
				fallthrough
			case word == "pin=no":
				log.Printf("TODO token=%s not implemented", word)
			case word[0] == 'l':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					iteration++
					if iteration <= i {
						goto wordLoop
					}
				}
			case word[0] == 'p':
				if bs, err := hex.DecodeString(word[1:]); err != nil {
					log.Fatalf("token=%s error=%v", word, errors.ErrorStack(err))
				} else {
					if len(bs) < 1 {
						log.Printf("pXX... requires at least 1 byte for command")
						break
					}
					p, err := client.DoTimeout(mega.Command_t(bs[0]), bs[1:], mega.DefaultTimeout)
					if err != nil {
						log.Printf("p rq=%02x rs=%s error=%v", bs, p.String(), err)
						break
					}
					log.Printf("response=%02x %s", p.Bytes(), p.String())
				}
			case word[0] == 'r':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					buf := make([]byte, i)
					err = client.RawRead(buf)
					if err != nil {
						log.Printf("read error=%v", err)
						break
					}
					// FIXME duplicate code
					err = mega.ParseResponse(buf, func(p mega.Packet) {
						log.Printf("- packet=%s %s", p.SimpleHex(), p.String())
					})
					if err != nil {
						log.Printf("rs=%02x parse error=%v", buf, err)
						break
					}
				}
			case word[0] == 's':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					time.Sleep(time.Duration(i) * time.Millisecond)
				}
			case word[0] == 't':
				if bs, err := hex.DecodeString(word[1:]); err != nil {
					log.Fatalf("token=%s error=%v", word, errors.ErrorStack(err))
				} else {
					err = client.RawWrite(bs)
					log.Printf("send error=%v", err)
				}
			}
		}
	}
}

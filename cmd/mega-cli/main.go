package main

import (
	"encoding/hex"
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	prompt "github.com/c-bata/go-prompt"
	"github.com/juju/errors"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

const usage = `syntax: commands separated by whitespace
- tick=yes|no  enable|disable backup reading every second
- pin=yes|no  enable|disable reading when pin signal is detected
- pXX...   send proper packet from hex XX... and receive response
- rN       (debug) read N bytes
- sN       pause N milliseconds
- tXX...   (debug) transmit bytes from hex XX...
`

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	i2cBusNo := cmdline.Uint("i2cbus", 0, "")
	addr := cmdline.Uint("addr", 0x78, "")
	pin := cmdline.Uint("pin", 23, "")
	cmdline.Parse(os.Args[1:])

	log.SetFlags(log2.LInteractiveFlags)

	client, err := mega.NewClient(byte(*i2cBusNo), byte(*addr), *pin, log)
	if err != nil {
		log.Fatal(errors.Trace(err))
	}

	// TODO OptionHistory
	prompt.New(newExecutor(client), newCompleter()).Run()
}

func newCompleter() func(d prompt.Document) []prompt.Suggest {
	suggests := []prompt.Suggest{
		prompt.Suggest{Text: "@01", Description: "status"},
		prompt.Suggest{Text: "@0301", Description: "soft reset (zero variables)"},
		prompt.Suggest{Text: "@03ff", Description: "hard reset (reboot)"},
		prompt.Suggest{Text: "@04", Description: "debug"},
		prompt.Suggest{Text: "@06", Description: "read TWI listen"},
		prompt.Suggest{Text: "@07", Description: "MDB bus reset"},
		prompt.Suggest{Text: "@08", Description: "MDB transaction"},
		prompt.Suggest{Text: "help"},
		prompt.Suggest{Text: "lN", Description: "repeat line N times"},
		prompt.Suggest{Text: "tXX", Description: "send packet"},
		prompt.Suggest{Text: "r70", Description: "read response"},
		prompt.Suggest{Text: "pXX", Description: "parse packet"},
	}

	return func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
	}
}

func newExecutor(client *mega.Client) func(string) {
	return func(line string) {
		if len(line) == 0 {
			return
		}

		tbegin := time.Now()

		words := strings.Split(line, " ")
		iteration := uint64(1)
	wordLoop:
		for _, word := range words {
			if strings.TrimSpace(word) == "" {
				continue
			}
			log.Debugf("(%d)%s", iteration, word)
			switch {
			case word == "help":
				log.Infof(usage)
			case word == "tick=yes":
				fallthrough
			case word == "tick=no":
				fallthrough
			case word == "pin=yes":
				fallthrough
			case word == "pin=no":
				log.Errorf("TODO token=%s not implemented", word)
			case word[0] == 'l':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					iteration++
					if iteration <= i {
						goto wordLoop
					}
				}
			case word[0] == '@':
				if bs, err := hex.DecodeString(word[1:]); err != nil {
					log.Fatalf("token=%s err=%v", word, err)
				} else {
					if len(bs) < 1 {
						log.Errorf("@XX... requires at least 1 byte for command")
						return
					}
					p, err := client.DoTimeout(mega.Command_t(bs[0]), bs[1:], mega.DefaultTimeout)
					if err != nil {
						log.Errorf("p rq=%x rs=%s err=%v", bs, p.String(), err)
						return
					}
					log.Debugf("response=%x %s", p.Bytes(), p.String())
				}
			case word[0] == 'p':
				if bs, err := hex.DecodeString(word[1:]); err != nil {
					log.Errorf("token=%s err=%v", word, err)
					return
				} else {
					p := mega.Packet{}
					err := p.Parse(bs)
					if err != nil {
						log.Errorf("parse input=%x err=%v", bs, err)
						return
					}
					log.Info(p.String())
				}
			case word[0] == 'r':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(errors.ErrorStack(err))
				} else {
					if i < 1 {
						return
					}
					buf := make([]byte, i)
					r := mega.PacketError{}
					client.Tx(0, nil, buf, 0, &r)
					if r.E != nil {
						log.Errorf("read err=%v", r.E)
						return
					} else {
						log.Debugf("packet=%s %s", r.P.SimpleHex(), r.P.String())
					}
				}
			case word[0] == 's':
				if i, err := strconv.ParseUint(word[1:], 10, 32); err != nil {
					log.Fatal(err)
					return
				} else {
					time.Sleep(time.Duration(i) * time.Millisecond)
				}
			case word[0] == 't':
				if bs, err := hex.DecodeString(word[1:]); err != nil {
					log.Fatalf("token=%s err=%v", word, errors.ErrorStack(err))
					return
				} else {
					r := mega.PacketError{}
					client.Tx(0, bs, nil, 0, &r)
					if r.E != nil {
						log.Errorf("send err=%v", r.E)
						return
					}
				}
			default:
				log.Errorf("unknown command '%s'", word)
				return
			}
		}

		lineDuration := time.Now().Sub(tbegin)
		log.Debugf("line duration=%s", lineDuration)
	}
}

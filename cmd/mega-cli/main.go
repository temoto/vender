package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/juju/errors"
	"github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/helpers/cli"
	"github.com/temoto/vender/log2"
)

const usage = `syntax: commands separated by whitespace
- @XX...   send proper packet from hex XX... and receive response
- pXX...   parse packet from hex XX... (without IO)
- rN       (debug) read N bytes
- sN       pause N milliseconds
- tXX...   (debug) transmit bytes from hex XX...
`

var log = log2.NewStderr(log2.LInfo)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	spiPort := cmdline.String("spi", "", "")
	gpiochip := cmdline.String("dev", "/dev/gpiochip0", "")
	pin := cmdline.String("pin", "25", "")
	testmode := cmdline.Bool("testmode", false, "run tests, exit code 0 if pass")
	rawmode := cmdline.Bool("raw", false, "raw mode skips ioLoop, if unsure do not use")
	logDebug := cmdline.Bool("log-debug", false, "")
	cmdline.Parse(os.Args[1:])

	log.SetFlags(log2.LInteractiveFlags)
	if *logDebug {
		log.SetLevel(log2.LDebug)
	}

	megaConfig := &mega.Config{
		SpiBus:         *spiPort,
		NotifyPinChip:  *gpiochip,
		NotifyPinName:  *pin,
		DontUseRawMode: *rawmode,
	}
	client, err := mega.NewClient(megaConfig, log)
	if err != nil {
		log.Fatal(errors.ErrorStack(errors.Trace(err)))
	}

	go func() {
		for kb := range client.TwiChan {
			log.Infof("keyboard event: %04x", kb)
		}
	}()

	if !*testmode {
		cli.MainLoop("vender-mega-cli", newExecutor(client), newCompleter())
	} else {
		cpu_burn := func() {
			var a []*flag.FlagSet
			for {
				a = make([]*flag.FlagSet, 0)
				// 3 cpu_burn 10000 iterations - works
				// 3 cpu_burn 15000 iterations - breaks mega to random errors
				for i := 1; i <= 10000; i++ {
					a = append(a, flag.NewFlagSet("", flag.ContinueOnError))
				}
				runtime.Gosched()
				// runtime.GC()
				_ = a
			}
		}
		go cpu_burn()
		go cpu_burn()
		go cpu_burn()
		newExecutor(client)("autotest")
	}
}

func newCompleter() func(d prompt.Document) []prompt.Suggest {
	suggests := []prompt.Suggest{
		prompt.Suggest{Text: "status", Description: "get status (@01)"},
		prompt.Suggest{Text: "reset_soft", Description: "soft reset, zero variables (@0301)"},
		prompt.Suggest{Text: "reset_hard", Description: "hard reset, full reboot (@03ff)"},
		prompt.Suggest{Text: "debug", Description: "get debug buffer (@04)"},
		prompt.Suggest{Text: "mdb_bus_reset", Description: "MDB bus reset (@07)"},
		prompt.Suggest{Text: "mdb=", Description: "MDB transaction (@08XX...)"},
		prompt.Suggest{Text: "help"},
		prompt.Suggest{Text: "lN", Description: "repeat line N times"},
		prompt.Suggest{Text: "tXX", Description: "send packet"},
		prompt.Suggest{Text: "r70", Description: "read response"},
		prompt.Suggest{Text: "pXX", Description: "parse packet without IO"},
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
		iteration := uint32(1)
	wordLoop:
		for _, word := range words {
			if strings.TrimSpace(word) == "" {
				continue
			}
			log.Infof("- (%d) %s", iteration, word)

			aliases := map[string]string{
				"status":        "@01",
				"reset_soft":    "@0301",
				"reset_hard":    "@03ff",
				"debug":         "@04",
				"mdb_bus_reset": "@0700c8",
			}
			if expanded := aliases[word]; expanded != "" {
				word = expanded
			}

			switch {
			case word == "help":
				log.Infof(usage)

			case word == "autotest":
				const autotestRequest = "0b"
				log.Infof("start autotest mdb=%s", autotestRequest)
				f, err := client.DoMdbTxSimple(mustDecodeHex(autotestRequest))
				if err != nil {
					log.Fatal(err)
				}
				log.Infof("response=%s", f.ResponseString())

				const N = 100000
				for i := 1; i <= N; i++ {
					_, err := client.DoMdbTxSimple(mustDecodeHex(autotestRequest))
					if err != nil {
						log.Fatal(err)
					}
					time.Sleep(200 * time.Microsecond)
					if i%1000 == 0 {
						fmt.Fprintf(os.Stderr, ".")
					}
				}

			case word[0] == 'l':
				i := mustParseUint(word[1:])
				iteration++
				if iteration <= i {
					goto wordLoop
				}

			case word[0] == '@':
				bs := mustDecodeHex(word[1:])
				if len(bs) < 1 {
					log.Errorf("@XX... requires at least 1 byte for command")
					return
				}
				cmd := mega.Command_t(bs[0])
				sendf := mega.NewCommand(cmd, bs[1:]...)
				log.Debugf("send=%x", sendf.Bytes())
				p, err := client.DoTimeout(cmd, bs[1:], mega.DefaultTimeout)
				if err != nil {
					log.Errorf("rq=%x rs=%s err=%v", bs, p.ResponseString(), err)
					return
				}
				log.Infof("response=%s", p.ResponseString())

			case strings.HasPrefix(word, "mdb="):
				bs := mustDecodeHex(word[len("mdb="):])
				if len(bs) < 1 {
					log.Errorf("mdb=... requires argument")
					return
				}
				r, err := client.DoMdbTxSimple(bs)
				if err != nil {
					log.Errorf("%s err=%v", err)
					return
				}
				log.Infof("response=%s", r.ResponseString())

			case word[0] == 'p':
				bs := mustDecodeHex(word[1:])
				if bs == nil {
					return
				}
				f := mega.Frame{}
				err := f.Parse(bs)
				if err != nil {
					log.Errorf("parse input=%x err=%v", bs, err)
					return
				}
				log.Info(f.ResponseString())

			case word[0] == 'r':
				i := mustParseUint(word[1:])
				if i < 1 {
					return
				}
				r := mega.Frame{}
				err := client.Tx(nil, &r, 0)
				switch err {
				case mega.ErrResponseEmpty:
					log.Infof("read empty")
				case nil:
					log.Infof("frame=%x %s", r.Bytes(), r.ResponseString())
				default:
					log.Errorf("read err=%v", err)
					return
				}

			case word[0] == 's':
				i := mustParseUint(word[1:])
				time.Sleep(time.Duration(i) * time.Millisecond)

			case word[0] == 't':
				send := mustDecodeHex(word[1:])
				if send == nil {
					return
				}
				recv, err := client.XXX_RawTx(send)
				log.Errorf("send=%x recv=%x err=%v", send, recv, err)
				if err == nil {
					r := mega.Frame{}
					err := r.Parse(recv)
					log.Errorf("parse frame=%s err=%v", r.ResponseString(), err)
				}

			default:
				log.Errorf("unknown command '%s'", word)
				return
			}
		}

		lineDuration := time.Since(tbegin)
		log.Debugf("line duration=%s", lineDuration)
	}
}

func mustDecodeHex(s string) []byte {
	bs, err := hex.DecodeString(s)
	if err != nil {
		log.Errorf("token=%s err=%v", s, err)
		return nil
	}
	return bs
}

func mustParseUint(s string) uint32 {
	x, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		log.Fatal(errors.ErrorStack(err))
	}
	return uint32(x)
}

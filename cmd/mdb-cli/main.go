package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	prompt "github.com/c-bata/go-prompt"
	"github.com/juju/errors"
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	mega "github.com/temoto/vender/hardware/mega-client"
	"github.com/temoto/vender/log2"
)

const usage = `syntax: commands separated by whitespace
(main)
- break    MDB bus reset (TX high for 200ms, wait for 500ms)
- sN       pause N milliseconds
- @XX...   transmit MDB block from hex XX..., show response

(meta)
- log=yes  enable debug logging
- log=no   disable debug logging
- loop=N   repeat N times all commands on this line
- par      execute concurrently all commands on this line
`

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	devicePath := cmdline.String("device", "/dev/ttyAMA0", "")
	iodinPath := cmdline.String("iodin", "./iodin", "Path to iodin executable")
	megaSpi := cmdline.String("mega-spi", "", "mega SPI port")
	megaPin := cmdline.String("mega-pin", "25", "mega notify pin")
	uarterName := cmdline.String("io", "file", "file|iodin|mega")
	cmdline.Parse(os.Args[1:])

	log.SetFlags(log2.LInteractiveFlags)

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
		mega, err := mega.NewClient(*megaSpi, *megaPin, log)
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
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, mdb.ContextKey, m)

	// TODO OptionHistory
	prompt.New(newExecutor(ctx), newCompleter(ctx)).Run()
}

var doUsage = engine.Func{F: func(ctx context.Context) error {
	log := log2.ContextValueLogger(ctx, log2.ContextKey)
	log.Infof(usage)
	return nil
}}
var doLogYes = engine.Func{Name: "log=yes", F: func(ctx context.Context) error {
	m := mdb.ContextValueMdber(ctx, mdb.ContextKey)
	m.Log.SetLevel(log2.LDebug)
	return nil
}}
var doLogNo = engine.Func{Name: "log=no", F: func(ctx context.Context) error {
	m := mdb.ContextValueMdber(ctx, mdb.ContextKey)
	m.Log.SetLevel(log2.LError)
	return nil
}}
var doBreak = engine.Func{Name: "break", F: func(ctx context.Context) error {
	m := mdb.ContextValueMdber(ctx, mdb.ContextKey)
	return m.BreakCustom(200*time.Millisecond, 500*time.Millisecond)
}}

func newCompleter(ctx context.Context) func(d prompt.Document) []prompt.Suggest {
	suggests := []prompt.Suggest{
		prompt.Suggest{Text: "break", Description: "MDB bus reset"},
		prompt.Suggest{Text: "sN", Description: "pause for N ms"},
		prompt.Suggest{Text: "loop=N", Description: "repeat line N times"},
		prompt.Suggest{Text: "par", Description: "execute commands on line concurrently"},
		prompt.Suggest{Text: "@XX", Description: "transmit MDB block, show response"},
	}

	return func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
	}
}

func newExecutor(ctx context.Context) func(string) {
	return func(line string) {
		d, err := parseLine(line)
		if err != nil {
			log.Errorf(errors.ErrorStack(err))
			// TODO continue when input is interactive (tty)
			return
		}
		err = d.Do(ctx)
		if err != nil {
			log.Errorf(errors.ErrorStack(err))
		}
	}
}

func newTx(request mdb.Packet) engine.Doer {
	return engine.Func{Name: "mdb:" + request.Format(), F: func(ctx context.Context) error {
		log := log2.ContextValueLogger(ctx, log2.ContextKey)
		m := mdb.ContextValueMdber(ctx, mdb.ContextKey)
		response := new(mdb.Packet)
		err := m.Tx(request, response)
		if err != nil {
			log.Errorf(errors.ErrorStack(err))
		} else {
			log.Infof("< %s", response.Format())
		}
		return err
	}}
}

func parseLine(line string) (engine.Doer, error) {
	words := strings.Split(line, " ")
	empty := true
	for i, w := range words {
		wt := strings.TrimSpace(w)
		if wt != "" {
			empty = false
			words[i] = wt
		}
	}
	if empty {
		return engine.Nothing{}, nil
	}

	// pre-parse special commands
	par := false
	loopn := uint(0)
	wordsRest := make([]string, 0, len(words))
	for _, word := range words {
		switch {
		case word == "help":
			return doUsage, nil
		case word == "par":
			par = true
		case strings.HasPrefix(word, "loop="):
			if loopn != 0 {
				return nil, errors.Errorf("multiple loop commands, expected at most one")
			}
			i, err := strconv.ParseUint(word[5:], 10, 32)
			if err != nil {
				return nil, errors.Annotatef(err, "word=%s", word)
			}
			loopn = uint(i)
		default:
			wordsRest = append(wordsRest, word)
		}
	}

	tx := engine.NewTransaction("input: " + line)
	var tail *engine.Node = &tx.Root
	for _, word := range wordsRest {
		if strings.HasPrefix(word, "log=") && par {
			log.Errorf("warning: log with par will produce unpredictable output, likely not what you want")
		}

		d, err := parseCommand(word)
		if d == nil && err == nil {
			log.Fatalf("code error parseCommand word='%s' both doer and err are nil", word)
		}
		if err != nil {
			// TODO accumulate errors into list
			return nil, err
		}
		if !par {
			tail = tail.Append(d)
		} else {
			tail.Append(d)
		}
	}

	if loopn != 0 {
		return engine.RepeatN{N: loopn, D: tx}, nil
	}
	return tx, nil
}

func parseCommand(word string) (engine.Doer, error) {
	switch {
	case word == "log=yes":
		return doLogYes, nil
	case word == "log=no":
		return doLogNo, nil
	case word == "break":
		return doBreak, nil
	case word[0] == 's':
		i, err := strconv.ParseUint(word[1:], 10, 32)
		if err != nil {
			return nil, errors.Annotatef(err, "word=%s", word)
		}
		return engine.Sleep{Duration: time.Duration(i) * time.Millisecond}, nil
	case word[0] == '@':
		request, err := mdb.PacketFromHex(word[1:], true)
		if err != nil {
			return nil, err
		}
		return newTx(request), nil
	default:
		return nil, errors.Errorf("error: invalid command: '%s'", word)
	}
}

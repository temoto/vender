package main

import (
	"context"
	"flag"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	prompt "github.com/c-bata/go-prompt"
	"github.com/juju/errors"
	iodin "github.com/temoto/iodin/client/go-iodin"
	"github.com/temoto/vender/engine"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/hardware/mdb/evend"
	"github.com/temoto/vender/head/state"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/log2"
)

const usage = `syntax: commands separated by whitespace
(main)
- sN       pause N milliseconds
- @ACTION  execute engine action
- mXX...   execute MDB sesuggestsion

(meta)
- loop=N   repeat N times all commands on this line
- par      execute concurrently all commands on this line
`

var log = log2.NewStderr(log2.LDebug)

func main() {
	cmdline := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagConfig := cmdline.String("config", "vender.hcl", "")
	flagUarter := cmdline.String("uarter", "file", "")
	cmdline.Parse(os.Args[1:])

	log.SetFlags(log2.LInteractiveFlags)

	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	eng := engine.NewEngine(ctx)
	ctx = context.WithValue(ctx, engine.ContextKey, eng)

	config := state.MustReadConfigFile(*flagConfig, log)
	log.Debugf("config=%+v", config)
	ctx = state.ContextWithConfig(ctx, config)

	if *flagUarter == "iodin" {
		iodin, err := iodin.NewClient(config.Hardware.IodinPath)
		if err != nil {
			err = errors.Annotatef(err, "config: mdb.uart_driver=%s iodin_path=%s", config.Hardware.Mdb.UartDriver, config.Hardware.IodinPath)
			log.Fatal(err)
		}
		config.Hardware.Mdb.Uarter = mdb.NewIodinUart(iodin)
		config.Hardware.Mdb.UartDevice = "\x0f\x0e"
	}

	mdber, err := mdb.NewMDB(config.Hardware.Mdb.Uarter, config.Hardware.Mdb.UartDevice, log.Clone(log2.LError))
	if err != nil {
		log.Fatal(errors.ErrorStack(err))
	}
	if config.Hardware.Mdb.Log {
		mdber.Log.SetLevel(log2.LDebug)
	}
	ctx = context.WithValue(ctx, mdb.ContextKey, mdber)
	if err := doMdbBreak.Do(ctx); err != nil {
		log.Fatal(errors.ErrorStack(err))
	}

	eng.Register("break", doMdbBreak)
	// TODO func(dev Devicer) { dev.Init() && dev.Register() }
	// right now Enum does IO implicitly
	evend.Enum(ctx, nil)

	log.Debugf("devices init complete")

	// TODO OptionHistory
	prompt.New(newExecutor(ctx), newCompleter(ctx)).Run()
}

func newCompleter(ctx context.Context) func(d prompt.Document) []prompt.Suggest {
	eng := engine.ContextValueEngine(ctx, engine.ContextKey)
	actions := eng.List()
	sort.Strings(actions)
	suggests := make([]prompt.Suggest, 0, len(actions))
	for _, a := range actions {
		suggests = append(suggests, prompt.Suggest{Text: "@" + a})
	}

	return func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
	}
}

func newExecutor(ctx context.Context) func(string) {
	eng := engine.ContextValueEngine(ctx, engine.ContextKey)

	return func(line string) {
		d, err := parseLine(eng, line)
		if err != nil {
			log.Errorf(errors.ErrorStack(err))
			return
		}
		err = d.Do(ctx)
		if err != nil {
			log.Errorf(errors.ErrorStack(err))
		}
	}
}

var doMdbBreak = engine.Func{Name: "mdb.break", F: func(ctx context.Context) error {
	mdber := mdb.ContextValueMdber(ctx, mdb.ContextKey)
	return mdber.BreakCustom(200*time.Millisecond, 500*time.Millisecond)
}}

var doUsage = engine.Func{F: func(ctx context.Context) error {
	log := log2.ContextValueLogger(ctx, log2.ContextKey)
	log.Infof(usage)
	return nil
}}

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

func parseLine(eng *engine.Engine, line string) (engine.Doer, error) {
	parts := strings.Split(line, " ")
	words := make([]string, 0, len(parts))
	empty := true
	for _, s := range parts {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			empty = false
			words = append(words, trimmed)
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
	errs := make([]error, 0, 32)
	for _, word := range wordsRest {
		if strings.HasPrefix(word, "log=") && par {
			log.Errorf("warning: log with par will produce unpredictable output, likely not what you want")
		}

		d, err := parseCommand(eng, word)
		if d == nil && err == nil {
			log.Fatalf("code error parseCommand word='%s' both doer and err are nil", word)
		}
		if err == nil {
			if !par {
				tail = tail.Append(d)
			} else {
				tail.Append(d)
			}
		} else {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return nil, helpers.FoldErrors(errs)
	}

	if loopn != 0 {
		return engine.RepeatN{N: loopn, D: tx}, nil
	}
	return tx, nil
}

func parseCommand(eng *engine.Engine, word string) (engine.Doer, error) {
	switch {
	case word[0] == 'm':
		request, err := mdb.PacketFromHex(word[1:], true)
		if err != nil {
			return nil, err
		}
		return newTx(request), nil
	case word[0] == 's':
		i, err := strconv.ParseUint(word[1:], 10, 32)
		if err != nil {
			return nil, errors.Annotatef(err, "word=%s", word)
		}
		return engine.Sleep{Duration: time.Duration(i) * time.Millisecond}, nil
	case strings.HasPrefix(word, "@"):
		arg := word[1:]
		d := eng.Resolve(arg)
		if d == nil {
			return nil, errors.Errorf("action='%s' is not registered", arg)
		}
		return d, nil
	default:
		return nil, errors.Errorf("invalid command: '%s'", word)
	}
}

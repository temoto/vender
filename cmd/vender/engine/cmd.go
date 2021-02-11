package engine

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	prompt "github.com/c-bata/go-prompt"
	"github.com/juju/errors"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/hardware"
	"github.com/temoto/vender/hardware/mdb"
	"github.com/temoto/vender/helpers"
	"github.com/temoto/vender/helpers/cli"
	"github.com/temoto/vender/internal/engine"
	"github.com/temoto/vender/internal/engine/inventory"
	"github.com/temoto/vender/internal/money"
	"github.com/temoto/vender/internal/state"
)

const usage = `syntax: commands separated by whitespace
(main)
- ACTION   execute engine action
- /sN      pause N milliseconds
- /mXX...  MDB send XX... in hex, receive

(meta)
- /loop=N  repeat N times all commands on this line
`

var Mod = subcmd.Mod{Name: "engine-cli", Main: Main}

func Main(ctx context.Context, config *state.Config) error {
	g := state.GetGlobal(ctx)
	g.MustInit(ctx, config)
	// g.Log.Debugf("config=%+v", g.Config)

	if err := g.Engine.ValidateExec(ctx, doMdbBusReset); err != nil {
		return errors.Annotate(err, "mdb bus reset")
	}

	if err := hardware.Enum(ctx); err != nil {
		return errors.Annotate(err, "hardware enum")
	}
	g.Log.Debugf("devices init complete")

	g.Engine.Register("mdb.bus_reset", doMdbBusReset)
	g.Engine.Register("money.commit", engine.Func0{Name: "money.commit", F: func() error {
		g.Log.Debugf("- money commit")
		return nil
	}})
	g.Engine.Register("stock.all.add(?)", engine.FuncArg{F: func(ctx context.Context, arg engine.Arg) error {
		g.Inventory.Iter(func(stock *inventory.Stock) {
			current := stock.Value()
			g.Log.Debugf("- source=%s value=%f", stock.Name, current)
			stock.Set(current + float32(arg))
		})
		return nil
	}})
	g.Engine.Register("stock.dump", engine.Func0{F: func() error {
		g.Inventory.Iter(func(stock *inventory.Stock) {
			g.Log.Debugf("- stock %#v", stock)
		})
		return nil
	}})
	ms := &money.MoneySystem{}
	if err := ms.Start(ctx); err != nil {
		g.Log.Error(errors.ErrorStack(err))
	}

	cli.MainLoop("vender-engine-cli", newExecutor(ctx), newCompleter(ctx))

	return nil
}

func newCompleter(ctx context.Context) func(d prompt.Document) []prompt.Suggest {
	g := state.GetGlobal(ctx)
	actions := g.Engine.List()
	sort.Strings(actions)
	suggests := make([]prompt.Suggest, 0, len(actions))
	for _, a := range actions {
		suggests = append(suggests, prompt.Suggest{Text: a})
	}

	return func(d prompt.Document) []prompt.Suggest {
		return prompt.FilterFuzzy(suggests, d.GetWordBeforeCursor(), true)
	}
}

func newExecutor(ctx context.Context) func(string) {
	g := state.GetGlobal(ctx)

	return func(line string) {
		d, err := parseLine(ctx, line)
		if err != nil {
			g.Log.Errorf(errors.ErrorStack(err))
			return
		}
		tbegin := time.Now()
		err = g.Engine.ValidateExec(ctx, d)
		if err != nil {
			g.Log.Errorf(errors.ErrorStack(err))
		}
		texec := time.Since(tbegin)
		g.Log.Infof("duration=%v", texec)
	}
}

var doMdbBusReset = engine.Func{Name: "mdb.bus_reset", F: func(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	m, err := g.Mdb()
	if err != nil {
		return err
	}
	return m.ResetDefault()
}}

var doUsage = engine.Func{F: func(ctx context.Context) error {
	g := state.GetGlobal(ctx)
	g.Log.Infof(usage)
	return nil
}}

func newTx(request mdb.Packet) engine.Doer {
	return engine.Func{Name: "mdb:" + request.Format(), F: func(ctx context.Context) error {
		g := state.GetGlobal(ctx)
		m, err := g.Mdb()
		if err != nil {
			return err
		}

		response := new(mdb.Packet)
		err = m.Tx(request, response)
		if err != nil {
			g.Log.Errorf(errors.ErrorStack(err))
		} else {
			g.Log.Infof("< %s", response.Format())
		}
		return err
	}}
}

func parseLine(ctx context.Context, line string) (engine.Doer, error) {
	g := state.GetGlobal(ctx)

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
	loopn := uint(0)
	wordsRest := make([]string, 0, len(words))
	for _, word := range words {
		switch {
		case word == "help":
			fallthrough
		case word == "/help":
			return doUsage, nil
		case strings.HasPrefix(word, "/loop="):
			if loopn != 0 {
				return nil, errors.Errorf("multiple loop commands, expected at most one")
			}
			i, err := strconv.ParseUint(word[6:], 10, 32)
			if err != nil {
				return nil, errors.Annotatef(err, "word=%s", word)
			}
			loopn = uint(i)
		default:
			wordsRest = append(wordsRest, word)
		}
	}

	tx := engine.NewSeq("input: " + line)
	errs := make([]error, 0, 32)
	for _, word := range wordsRest {
		d, err := parseCommand(g.Engine, word)
		if d == nil && err == nil {
			g.Log.Fatalf("code error parseCommand word='%s' both doer and err are nil", word)
		}
		if err == nil {
			tx.Append(d)
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
	if strings.HasPrefix(word, "/m") {
		request, err := mdb.PacketFromHex(word[2:], true)
		if err != nil {
			return nil, errors.Annotatef(err, engine.FmtErrContext, word)
		}
		return newTx(request), nil
	}

	d, err := eng.ResolveOrLazy(word)
	return d, errors.Annotatef(err, engine.FmtErrContext, word)
}

package main

import (
	"flag"
	"os"

	"github.com/juju/errors"
	"github.com/temoto/vender/cmd/vender/engine"
	"github.com/temoto/vender/cmd/vender/mdb"
	"github.com/temoto/vender/cmd/vender/subcmd"
	"github.com/temoto/vender/cmd/vender/tele"
	"github.com/temoto/vender/cmd/vender/ui"
	"github.com/temoto/vender/cmd/vender/vmc"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
)

var log = log2.NewStderr(log2.LDebug)
var modules = []subcmd.Mod{
	engine.Mod,
	mdb.Mod,
	tele.Mod,
	ui.Mod,
	vmc.Mod,
}

func main() {
	errors.SetSourceTrimPrefix(os.Getenv("source_trim_prefix"))
	log.SetFlags(0)

	mod, err := subcmd.Parse(os.Args, modules)
	if err != nil {
		log.Fatal(err)
	}

	var configPath string
	// flagset := mod.FlagSet( /*modArgs*/ )
	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.StringVar(&configPath, "config", "vender.hcl", "")
	if err := flagset.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	config := state.MustReadConfig(log, state.NewOsFullReader(), configPath)

	log.SetFlags(log2.LInteractiveFlags)
	ctx, _ := state.NewContext(log)
	if subcmd.SdNotify("start") {
		// under systemd assume systemd journal logging, no timestamp
		log.SetFlags(log2.LServiceFlags)
	}
	log.Debugf("starting command %s", mod.Name)

	if err := mod.Main(ctx, config); err != nil {
		log.Fatal(errors.ErrorStack(err))
	}
}

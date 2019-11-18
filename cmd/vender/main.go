package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/temoto/vender/cmd/vender/engine"
	"github.com/temoto/vender/cmd/vender/mdb"
	"github.com/temoto/vender/cmd/vender/subcmd"
	cmd_tele "github.com/temoto/vender/cmd/vender/tele"
	"github.com/temoto/vender/cmd/vender/ui"
	"github.com/temoto/vender/cmd/vender/vmc"
	"github.com/temoto/vender/head/tele"
	"github.com/temoto/vender/log2"
	"github.com/temoto/vender/state"
	state_new "github.com/temoto/vender/state/new"
)

var log = log2.NewStderr(log2.LDebug)
var modules = []subcmd.Mod{
	vmc.BrokenMod,
	engine.Mod,
	mdb.Mod,
	cmd_tele.Mod,
	ui.Mod,
	vmc.VmcMod,
	subcmd.Mod{Name: "version", Main: versionMain},
}

var BuildVersion string = "unknown" // set by ldflags -X
var reFlagVersion = regexp.MustCompile("-?-?version")

func main() {
	errors.SetSourceTrimPrefix(os.Getenv("source_trim_prefix"))
	log.SetFlags(0)

	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flagset.Usage = func() {
		fmt.Fprint(flagset.Output(), "Usage: [option...] command\n\nOptions:\n")
		flagset.PrintDefaults()
		commandNames := make([]string, len(modules))
		for i, m := range modules {
			commandNames[i] = m.Name
		}
		fmt.Fprintf(flagset.Output(), "Commands: %s\n", strings.Join(commandNames, " "))
	}
	configPath := flagset.String("config", "vender.hcl", "")
	onlyVersion := flagset.Bool("version", false, "print build version and exit")
	if err := flagset.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if *onlyVersion || (len(os.Args) == 2 && reFlagVersion.MatchString(os.Args[1])) {
		_ = versionMain(context.Background(), nil)
		return
	}

	mod, err := subcmd.Parse(flagset.Arg(0), modules)
	if err != nil {
		fmt.Fprintf(flagset.Output(), "command line error: %v\n\n", err)
		flagset.Usage()
		os.Exit(1)
	}

	config := state.MustReadConfig(log, state.NewOsFullReader(), *configPath)

	log.SetFlags(log2.LInteractiveFlags)
	ctx, g := state_new.NewContext(log, new(tele.Tele))
	g.BuildVersion = BuildVersion
	if subcmd.SdNotify("start") {
		// under systemd assume systemd journal logging, no timestamp
		log.SetFlags(log2.LServiceFlags)
	}
	log.Debugf("starting command %s", mod.Name)

	if err := mod.Main(ctx, config); err != nil {
		g.Fatal(err)
	}
}

func versionMain(ctx context.Context, config *state.Config) error {
	fmt.Printf("vender %s\n", BuildVersion)
	return nil
}

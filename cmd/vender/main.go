package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" //#nosec G108
	"os"
	"regexp"
	"strings"

	"github.com/juju/errors"
	cmd_engine "github.com/temoto/vender/cmd/vender/engine"
	"github.com/temoto/vender/cmd/vender/mdb"
	"github.com/temoto/vender/cmd/vender/subcmd"
	cmd_tele "github.com/temoto/vender/cmd/vender/tele"
	"github.com/temoto/vender/cmd/vender/ui"
	"github.com/temoto/vender/cmd/vender/vmc"
	"github.com/temoto/vender/internal/state"
	state_new "github.com/temoto/vender/internal/state/new"
	"github.com/temoto/vender/internal/tele"
	"github.com/temoto/vender/log2"
)

var log = log2.NewStderr(log2.LDebug)
var modules = []subcmd.Mod{
	vmc.BrokenMod,
	cmd_engine.Mod,
	mdb.Mod,
	cmd_tele.Mod,
	ui.Mod,
	vmc.VmcMod,
	subcmd.Mod{Name: "version", Main: versionMain},
}

var BuildVersion string = "unknown" // set by ldflags -X
var reFlagVersion = regexp.MustCompile("-?-?version")

func main() {
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
	ctx, g := state_new.NewContext(log, tele.New())
	g.BuildVersion = BuildVersion
	if subcmd.SdNotify("start") {
		// under systemd assume systemd journal logging, no timestamp
		log.SetFlags(log2.LServiceFlags)
	}
	g.Error(pprofStart(g)) // debug server

	log.Debugf("starting command %s", mod.Name)
	if err := mod.Main(ctx, config); err != nil {
		g.Fatal(err)
	}
}

func pprofStart(g *state.Global) error {
	addr := g.Config.Debug.PprofListen
	if addr == "" {
		return nil
	}

	srv := &http.Server{Addr: addr, Handler: nil} // TODO specific pprof handler
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.Annotate(err, "pprof")
	}
	g.Log.Debugf("pprof http://%s/debug/pprof/", ln.Addr().String())
	go pprofServe(g, srv, ln)
	return nil
}

// not inline only for clear goroutine source in panic trace
func pprofServe(g *state.Global, srv *http.Server, ln net.Listener) { g.Error(srv.Serve(ln)) }

func versionMain(ctx context.Context, config *state.Config) error {
	fmt.Printf("vender %s\n", BuildVersion)
	return nil
}

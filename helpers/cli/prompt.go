package cli

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/c-bata/go-prompt"
	"github.com/mattn/go-isatty"
)

func MainLoop(tag string, exec func(line string), complete func(d prompt.Document) []prompt.Suggest) {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		for range signalCh {
			// TODO engine.Interrupt()
			// if s == syscall.SIGINT { }
			os.Exit(1)
		}
	}()

	if isatty.IsTerminal(os.Stdin.Fd()) {
		// TODO OptionHistory
		prompt.New(exec, complete).Run()
	} else {
		stdinAll, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		linesb := bytes.Split(stdinAll, []byte{'\n'})
		for _, lineb := range linesb {
			line := string(bytes.TrimSpace(lineb))
			exec(line)
		}
	}
}

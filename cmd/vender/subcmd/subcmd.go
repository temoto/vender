// Support sub-commands in vender application.
// It's simple but fine so far.
// Can switch to github.com/urfave/cli later.
package subcmd

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/coreos/go-systemd/daemon"
	"github.com/temoto/errors"
	"github.com/temoto/vender/state"
)

type Mod struct {
	Name string
	Main func(context.Context, *state.Config) error
}

func Parse(args []string, modules []Mod) (*Mod, error) {
	if len(args) == 0 {
		panic("code error len(args)=0")
	}

	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = m.Name
	}
	usage := fmt.Sprintf(`usage: %s command [options]
Available commands: %s`, args[0], strings.Join(names, " "))
	if len(args) == 1 {
		return nil, fmt.Errorf(usage)
	}
	command := args[1]
	var found *Mod
	for i := range modules {
		m := &modules[i]
		if m.Name == "" {
			panic(fmt.Sprintf("code error Name='' module=%#v", m))
		}
		if command == m.Name {
			found = m
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("unknown command='%s'\n%s", command, usage)
	}
	return found, nil
}

func SdNotify(s string) bool {
	ok, err := daemon.SdNotify(false, s)
	if err != nil {
		log.Fatal("sdnotify: ", errors.ErrorStack(err))
	}
	return ok
}

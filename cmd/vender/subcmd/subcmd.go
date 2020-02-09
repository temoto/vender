// Support sub-commands in vender application.
// It's simple but fine so far.
// Can switch to github.com/urfave/cli later.
package subcmd

import (
	"context"
	"fmt"
	"log"

	"github.com/coreos/go-systemd/daemon"
	"github.com/juju/errors"
	"github.com/temoto/vender/internal/state"
)

type Mod struct {
	Name string
	Main func(context.Context, *state.Config) error
}

func Parse(command string, modules []Mod) (*Mod, error) {
	if command == "" {
		return nil, fmt.Errorf("empty command")
	}

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
		return nil, fmt.Errorf("unknown command='%s'", command)
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

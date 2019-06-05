package state

import "github.com/temoto/alive"

type Runner interface {
	Run(*alive.Alive)
}

type FuncRunner func(*alive.Alive)

func (fr FuncRunner) Run(a *alive.Alive) { fr(a) }

func (g *Global) UISwitch(u Runner, force bool) {
	// g.UI.lk.Lock()
	// defer g.UI.lk.Unlock()

	current := g.UI.currentAlive.Load()
	if current == nil {
		if force {
			panic("code error UISwitch force from nothing")
		}
	} else {
		ca := current.(*alive.Alive)
		if force {
			ca.Stop()
		}
		ca.Wait()
	}

	na := alive.NewAlive()
	g.UI.currentAlive.Store(na)
	go u.Run(na)
}

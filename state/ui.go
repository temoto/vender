package state

import "github.com/temoto/alive"

type Runner interface {
	Run(*alive.Alive)
	String() string
}

type FuncRunner struct {
	Name string
	F    func(*alive.Alive)
}

func (fr *FuncRunner) Run(a *alive.Alive) { fr.F(a) }
func (fr *FuncRunner) String() string     { return fr.Name }

func (g *Global) UIWait() {
	current := g.UI.currentAlive.Load()
	if current != nil {
		ca := current.(*alive.Alive)
		ca.Wait()
	}
}

func (g *Global) uiActivate(u Runner) {
	tagNext := u.String()
	g.Log.Debugf("UISwitch next=%s activating", tagNext)
	na := alive.NewAlive()
	g.UI.currentAlive.Store(na)
	go u.Run(na)
}

func (g *Global) UINext(u Runner) {
	tagNext := u.String()

	var current *alive.Alive
	for {
		g.Log.Debugf("UINext u=%s waiting", tagNext)
		g.UIWait()

		g.UI.lk.Lock()
		x := g.UI.currentAlive.Load()
		if x == nil {
			g.uiActivate(u)
			g.UI.lk.Unlock()
			return
		} else {
			current = x.(*alive.Alive)
			if !current.IsRunning() {
				g.uiActivate(u)
				g.UI.lk.Unlock()
				return
			}
			// concurrent next/switch won race, wait again
		}
		g.UI.lk.Unlock()
	}
}

func (g *Global) UISwitch(u Runner) {
	tagNext := u.String()
	g.Log.Debugf("UISwitch u=%s", tagNext)

	var current *alive.Alive
	g.UI.lk.Lock()
	x := g.UI.currentAlive.Load()
	if x == nil {
		g.UI.lk.Unlock()
		panic("code error UISwitch from nothing")
	} else {
		current = x.(*alive.Alive)
		g.Log.Debugf("UISwitch force")
		current.Stop()
		// g.UI.lk.Unlock()
		// continue
		current.Wait()
		g.uiActivate(u)
		g.UI.lk.Unlock()
		return
	}
	g.UI.lk.Unlock()
}

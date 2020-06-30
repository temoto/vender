package engine

import (
	"regexp"
	"sync/atomic"
	"time"
)

type ProfileFunc func(Doer, time.Duration)

// re=nil or fun=nil to disable profiling.
func (self *Engine) SetProfile(re *regexp.Regexp, min time.Duration, fun ProfileFunc) {
	fast := uint32(0)
	if re != nil || fun != nil {
		fast = 1
	}
	defer atomic.StoreUint32(&self.profile.fastpath, fast)
	self.profile.Lock()
	defer self.profile.Unlock()
	self.profile.re = re
	self.profile.fun = fun
	self.profile.min = min
}

func (self *Engine) matchProfile(s string) (ProfileFunc, time.Duration) {
	if atomic.LoadUint32(&self.profile.fastpath) != 1 {
		return nil, 0
	}
	self.profile.Lock()
	defer self.profile.Unlock()
	if self.profile.re != nil && self.profile.re.MatchString(s) {
		return self.profile.fun, self.profile.min
	}
	return nil, 0
}

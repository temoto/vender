package msync

type Nothing struct{}
type Signal chan Nothing

func NewSignal() Signal { return make(chan Nothing) }

func (s Signal) Closed() bool {
	select {
	case <-s:
		return true
	default:
		return false
	}
}

func (s Signal) Set() {
	select {
	case s <- Nothing{}:
	default:
	}
}
func (s Signal) Wait() { <-s }

package main

import (
	"log"
	"time"
)

func Hello(w *MultiWait, args interface{}) (err error) {
	if w.IsDone() {
		log.Println("hello aborted")
		return
	}
	log.Println("hello begin")
	select {
	case <-time.After(1 * time.Second):
	case <-w.Chan():
	}
	if w.IsDone() {
		log.Println("hello aborted")
		return
	}
	log.Println("hello done")
	return
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("hello")

	seq := NewSequence("head-init")
	seq.Append(NewAction("", Hello))
	seq.Append(MustGlobalAction("display-init"))

	seq.Start()
	time.Sleep(100 * time.Millisecond)
	seq.Abort()

	seq.Start()
	seq.Wait()
	time.Sleep(100 * time.Millisecond)

}

# What

alive waits for subtasks, coordinate graceful or fast shutdown. sync.WaitGroup on steroids.


# Usage [![GoDoc](https://godoc.org/github.com/temoto/alive?status.svg)](https://godoc.org/github.com/temoto/alive)

Key takeaways:

* `go get github.com/temoto/alive/v2`
* Zero value of `alive.Alive{}` is *not* usable ever, you *must* use `NewAlive()` constructor.
```
    srv := MyServer{ alive: alive.NewAlive() }
```
* Call `.Add(n)` and `.Done()` just as with `WaitGroup` but check return value.
```
    for {
        task := <-queue
        if !srv.alive.Add(1) {
            break
        }
        go func() {
            // be useful
            srv.alive.Done()
        }()
    }
```
* Call `.Stop()` to switch `IsRunning` and stop creating new tasks if programmed so.
```
    sigShutdownChan := make(chan os.Signal, 1)
    signal.Notify(sigShutdownChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
    go func(ch <-chan os.Signal) {
        <-ch
        log.Printf("graceful stop")
        sdnotify("READY=0\nSTATUS=stopping\n")
        srv.alive.Stop()
    }(sigShutdownChan)
```
* Call `.Wait()` to synchronize on all subtasks `.Done()`, just as with `WaitGroup`.
```
func main() {
    // ...
    srv.alive.Wait()
}
```
* `.StopChan()` lets you observe `.Stop()` call from another place. A better option to `IsRunning()` poll.
```
    stopch := srv.alive.StopChan()
    for {
        select {
        case job := <-queue:
            // be useful
        case <-stopch:
            // break for loop
        }
    }
```
* `.WaitChan()` is `select`-friendly version of `.Wait()`.
* There are few `panic()` which should never happen, like debug-build assertions. But please tell me if you find a way to trigger `"Bug in package"`


# Flair

[![Build status](https://travis-ci.org/temoto/alive.svg?branch=master)](https://travis-ci.org/temoto/alive)
[![Coverage](https://codecov.io/gh/temoto/alive/branch/master/graph/badge.svg)](https://codecov.io/gh/temoto/alive)
[![Go Report Card](https://goreportcard.com/badge/github.com/temoto/alive)](https://goreportcard.com/report/github.com/temoto/alive)

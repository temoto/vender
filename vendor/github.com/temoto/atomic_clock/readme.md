# What

atomic_clock is convenient API around atomic int64 of monotonic clock.
Use for time accounting. Do not use where actual time value matters.


# Usage [![GoDoc](https://pkg.go.dev/github.com/temoto/atomic_clock?status.svg)](https://godoc.org/github.com/temoto/atomic_clock)

Key takeaways:

* `go get github.com/temoto/atomic_clock`
* Zero value of `atomic_clock.Clock{}` is usable.
* Content is single int64 offset in nanoseconds from undefined epoch. Clock source is `time.Since(epoch)` which is monotonic since Go 1.9.


# Flair

[![Build status](https://travis-ci.org/temoto/atomic_clock.svg?branch=master)](https://travis-ci.org/temoto/atomic_clock)
[![Coverage](https://codecov.io/gh/temoto/atomic_clock/branch/master/graph/badge.svg)](https://codecov.io/gh/temoto/atomic_clock)
[![Go Report Card](https://goreportcard.com/badge/github.com/temoto/atomic_clock)](https://goreportcard.com/report/github.com/temoto/atomic_clock)

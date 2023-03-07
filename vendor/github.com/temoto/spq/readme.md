What
====

Persistent queue library with requirements:
- job API: Push, Peek, Delete
- data safety: critical
- zero maintenance, auto-recover as much as possible and keep working
- speed: sane, avoid obvious mistakes
- single process access, thread safe


Credits
=======

This library would be poorly reinvented wheel without following great work:
- https://github.com/beeker1121/goque
- https://github.com/syndtr/goleveldb


Flair
=====

[![Build status](https://travis-ci.org/temoto/spq.svg?branch=master)](https://travis-ci.org/temoto/spq)
[![Coverage](https://codecov.io/gh/temoto/spq/branch/master/graph/badge.svg)](https://codecov.io/gh/temoto/spq)
[![Go Report Card](https://goreportcard.com/badge/github.com/temoto/spq)](https://goreportcard.com/report/github.com/temoto/spq)

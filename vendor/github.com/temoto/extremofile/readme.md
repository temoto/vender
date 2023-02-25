# What

Extremofile is single binary value persistent storage that makes best effort to read your data back unchanged. [![GoDoc](https://godoc.org/github.com/temoto/extremofile?status.svg)](https://godoc.org/github.com/temoto/extremofile)

## Use case

Answer yes to all questions:

- Need to persist small amount of data? It costs 2 full data writes + fsync on each store operation.
- Expect power loss or storage media degradation? If you have reliable power supply and storage, that's awesome.
- Network backup is not available? If you have robust remote database available when you need it, that's great.

Failure modes tolerated:
- power loss
- random data corrupted in one replica

Failure modes detected:
- almost any data corruption, limited to checksum mismatch

Failures not handled by design: human error, files deleted.

## API guarantees

- You only get valid value bytes.
- You may get value/writer together with non-critical error.
- Internal storage format may change with major version.
- Library will read one previous storage format.
- Thread-safe.
- Not safe to use by multiple processes, data may be corrupted.


# FAQ

- > Why not SQLite or goleveldb or X?

  I could not find a ready solution that actually survives storage media degradation, rather than just reporting error. If you need similar durability with complex data structure, wrap your database of choice with automatic backup.


# Flair

[![Build status](https://travis-ci.org/temoto/extremofile.svg?branch=master)](https://travis-ci.org/temoto/extremofile)
[![Coverage](https://codecov.io/gh/temoto/extremofile/branch/master/graph/badge.svg)](https://codecov.io/gh/temoto/extremofile)
[![Go Report Card](https://goreportcard.com/badge/github.com/temoto/extremofile)](https://goreportcard.com/report/github.com/temoto/extremofile)

package helpers

type FatalFunc func(...interface{})

type Fataler interface {
	Fatal(...interface{})
}

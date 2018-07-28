package state

import (
	"io/ioutil"
	"os"

	"github.com/hashicorp/hcl"
)

type Config struct {
	Jfk string
}

func ReadConfigFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var b []byte
	b, err = ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = hcl.Unmarshal(b, c)
	return c, err
}

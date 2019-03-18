package ui

import "github.com/paulrosania/go-charset/charset"

var cs1251 charset.Translator

func translate(s string) []byte {
	_, bs, err := cs1251.Translate([]byte(s), true)
	if err != nil {
		panic(err)
	}
	return bs
}

func (self *UISystem) displayInit() (err error) {
	cs1251, err = charset.TranslatorTo("windows-1251")
	if err != nil {
		return err
	}

	self.Log.Debugf("display-init")
	if err = self.display.Init(); err != nil {
		return err
	}
	self.display.Init4()
	return
}

package main

import (
	"log"
	"time"

	"github.com/paulrosania/go-charset/charset"
	_ "github.com/paulrosania/go-charset/data"
	"github.com/temoto/vender/hardware/lcd"
)

var cs1251 charset.Translator

func translate(s string) []byte {
	_, bs, err := cs1251.Translate([]byte(s), true)
	if err != nil {
		panic(err)
	}
	return bs
}

func main() {
	log.Printf("Init GPIO")
	d := new(lcd.LCD)
	if err := d.Init(); err != nil {
		panic(err)
	}

	log.Printf("Init LCD")
	d.Init4()

	var err error
	cs1251, err = charset.TranslatorTo("windows-1251")
	if err != nil {
		panic(err)
	}

	for {
		d.WriteBytes(translate("Добро спасёт мир\n"))
		time.Sleep(2000 * time.Millisecond)
		d.WriteBytes(translate("если повезёт "))
		d.Data(0x1c)
		d.Data(0xbc)

		time.Sleep(5000 * time.Millisecond)
		d.CommandClear()
		d.CommandOff()
		time.Sleep(5000 * time.Millisecond)
		d.CommandOn(true, false, false)
	}
}

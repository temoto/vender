package main

import (
	"fmt"
	"time"

	"github.com/temoto/vender/hardware/display"
)

func main() {
	fmt.Printf("Init GPIO\n")
	d := new(lcd.LCD)
	if err := d.Init(); err != nil {
		panic(err)
	}

	fmt.Printf("Init LCD\n")
	d.Init4()

	for {
		d.WriteString("Добро спасёт мир", 70, 140)
		d.CommandAddress(0x40)
		time.Sleep(2000 * time.Millisecond)
		d.WriteString("если повезёт ", 133, 300)
		d.Data(0x1c)
		d.Data(0xbc)

		time.Sleep(5000 * time.Millisecond)
		d.CommandClear()
		d.CommandOff()
		time.Sleep(5000 * time.Millisecond)
		d.CommandOn(true, false, false)
	}
}

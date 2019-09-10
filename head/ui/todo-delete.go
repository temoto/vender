package ui

// TODO move all of these to config

import "time"

const (
	DefaultCream = 4
	MaxCream     = 6
	DefaultSugar = 4
	MaxSugar     = 8
)

const modTuneTimeout = 3 * time.Second

var ScaleAlpha = []byte{
	0x94, // empty
	0x95,
	0x96,
	0x97, // full
	// '0', '1', '2', '3',
}

const (
	MsgError   = "error"
	MsgCream   = "Сливки"
	MsgSugar   = "Сахар"
	MsgCredit  = "Кредит:"
	MsgMaking1 = "спасибо"
	MsgMaking2 = "готовлю"

	MsgMenuCodeEmpty          = "нажимайте цифры"
	MsgMenuCodeInvalid        = "проверьте код"
	MsgMenuInsufficientCredit = "добавьте денег"
	MsgMenuNotAvailable       = "сейчас недоступно"

	MsgInputCode = "код:%s\x00"

	msgServiceInputAuth = "\x8d %s\x00"
	msgServiceMenu      = "Menu"
)

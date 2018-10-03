package money

import "fmt"

var (
	ErrSensor      = fmt.Errorf("Defective Sensor")
	ErrNoStorage   = fmt.Errorf("Storage Unplugged")
	ErrJam         = fmt.Errorf("Jam")
	ErrROMChecksum = fmt.Errorf("ROM checksum")
	ErrFraud       = fmt.Errorf("Possible Credited Money Removal")
)

package money

import "fmt"

var (
	ErrSensor      = fmt.Errorf("Defective Sensor")
	ErrNoStorage   = fmt.Errorf("Storage Unplugged")
	ErrJam         = fmt.Errorf("Jam")
	ErrROMChecksum = fmt.Errorf("ROM checksum")
	ErrFraud       = fmt.Errorf("Possible Credited Money Removal")
	ErrFishingOK   = fmt.Errorf("ALERT !!!! Credited Money Removal. good fishing.")
	ErrFishingFail = fmt.Errorf("maybe alert !!!! Bill money fishing fail.")
)

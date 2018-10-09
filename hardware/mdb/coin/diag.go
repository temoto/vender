package coin

import (
	"encoding/binary"
	"fmt"
	"strings"
)

//go:generate stringer -type=DiagStatus -trimprefix=Diag
type DiagStatus uint16

const (
	DiagPoweringUp                 DiagStatus = 0x0100
	DiagPoweringDown               DiagStatus = 0x0200
	DiagOK                         DiagStatus = 0x0300
	DiagKeypadShifted              DiagStatus = 0x0400
	DiagManualActive               DiagStatus = 0x0510
	DiagNewInventoryInformation    DiagStatus = 0x0520
	DiagInhibited                  DiagStatus = 0x0600
	DiagGeneralError               DiagStatus = 0x1000
	DiagGeneralChecksum1           DiagStatus = 0x1001
	DiagGeneralChecksum2           DiagStatus = 0x1002
	DiagGeneralVoltage             DiagStatus = 0x1003
	DiagDiscriminatorError         DiagStatus = 0x1100
	DiagDiscriminatorFlightOpen    DiagStatus = 0x1110
	DiagDiscriminatorReturnOpen    DiagStatus = 0x1111
	DiagDiscriminatorJam           DiagStatus = 0x1130
	DiagDiscriminatorBelowStandard DiagStatus = 0x1141
	DiagDiscriminatorSensorA       DiagStatus = 0x1150
	DiagDiscriminatorSensorB       DiagStatus = 0x1151
	DiagDiscriminatorSensorC       DiagStatus = 0x1152
	DiagDiscriminatorTemperature   DiagStatus = 0x1153
	DiagDiscriminatorOptics        DiagStatus = 0x1154
	DiagAccepterError              DiagStatus = 0x1200
	DiagAccepterJam                DiagStatus = 0x1230
	DiagAccepterAlarm              DiagStatus = 0x1231
	DiagAccepterEmpty              DiagStatus = 0x1240
	DiagAccepterExitBeforeEnter    DiagStatus = 0x1250
	DiagSeparatorError             DiagStatus = 0x1300
	DiagSeparatorSortSensor        DiagStatus = 0x1310
	DiagDispenserError             DiagStatus = 0x1400
	DiagStorageError               DiagStatus = 0x1500
	DiagStorageCassetteRemoved     DiagStatus = 0x1502
	DiagStorageCashboxSensor       DiagStatus = 0x1503
	DiagStorageAmbientLight        DiagStatus = 0x1504
)

type DiagResult []DiagStatus

func (self DiagResult) OK() bool {
	l := len(self)
	return l == 0 || (l == 1 && self[0] == DiagOK)
}

func (self DiagResult) Error() string {
	ss := make([]string, len(self))
	for i, ds := range self {
		ss[i] = ds.String()
	}
	return strings.Join(ss, ",")
}

func parseDiagResult(b []byte, byteOrder binary.ByteOrder) (DiagResult, error) {
	lb := len(b)
	if lb == 0 {
		return nil, nil
	}
	if lb%2 != 0 {
		return nil, fmt.Errorf("Diag response must be 0..8 words of 16 bit")
	}
	dr := make(DiagResult, lb/2)
	for i := 0; i < lb/2; i++ {
		dr[i] = DiagStatus(byteOrder.Uint16(b[i*2 : i*2+2]))
	}
	return dr, nil
}

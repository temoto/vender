package mdb_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/hardware/mdb"
)

func TestDeviceTx(t *testing.T) {
	t.Parallel()

	mdbus, mock := mdb.NewMockBus(t)
	defer mock.Close()
	mock.ExpectMap(map[string]string{
		"30": "",
		"33": "",
		"":   "",
	})

	d := mdb.Device{}
	assert.Equal(t, d.State(), mdb.DeviceInvalid)
	d.Init(mdbus, 0x30, "mockdev", binary.BigEndian)
	assert.Equal(t, d.State(), mdb.DeviceInited)

	require.NoError(t, d.Tx(d.PacketPoll).E)
	assert.Equal(t, mdb.DeviceOnline, d.State())

	require.Error(t, mdb.ErrTimeout, d.Tx(d.PacketPoll).E)
	assert.Equal(t, mdb.DeviceError, d.State())

	require.Error(t, mdb.ErrTimeout, d.Tx(d.PacketPoll).E)
	assert.Equal(t, mdb.DeviceOffline, d.State())
}

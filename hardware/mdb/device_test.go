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
	assert.Equal(t, mdb.DeviceInvalid, d.State())
	d.Init(mdbus, 0x30, "mockdev", binary.BigEndian)
	assert.Equal(t, mdb.DeviceInited, d.State())

	require.NoError(t, d.TxKnown(d.PacketPoll, nil))
	assert.Equal(t, mdb.DeviceOnline, d.State())

	require.Error(t, mdb.ErrTimeout, d.TxKnown(d.PacketPoll, nil))
	assert.Equal(t, mdb.DeviceOffline, d.State())
}

package mega

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClose(t *testing.T) {
	env := testEnv(t)
	env.spiMock.PushOk("0400", "0400")
	m, err := NewClient(&env.config, env.log)
	require.NoError(t, err)
	require.NoError(t, m.Close())
}

func TestHighOnStart(t *testing.T) {
	env := testEnv(t)
	// mockGet(&env.notifyMock.Mock, "Close").Return(ErrRequestBusy)
	env.spiMock.PushOk("0400", "c40a")
	env.spiMock.PushOk("04000000000000000000000000000000000000000000", "c40a03026bb12104001b00b084000101010101010101")
	env.spiMock.PushOk("84020a8400000000000000000000", "c4020a84af000101010101010101")
	env.spiMock.PushOk("0400", "0400")
	m, err := NewClient(&env.config, env.log)
	require.NoError(t, err)
	require.NoError(t, m.Close())
}

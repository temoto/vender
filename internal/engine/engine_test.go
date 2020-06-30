package engine

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temoto/vender/log2"
)

func TestNotResolved(t *testing.T) {
	t.Parallel()

	_, e := newTestContext(t)
	require.NoError(t, e.RegisterParse("root", "sudo make me a sandwich"))
	e.Register("sudo", Nothing{})
	e.Register("make", Nothing{})

	assert.True(t, IsNotResolved(NewErrNotResolved("TODO_random")))
	for _, s := range []string{"sudo", "make"} {
		x := e.Resolve(s)
		assert.False(t, IsNotResolved(x), "x=%#v", x)
	}
	for _, s := range []string{"TODO_random", "me", "sandwich"} {
		x := e.Resolve(s)
		assert.True(t, IsNotResolved(x), "x=%#v", x)
	}
}

func TestResolveLazyArg(t *testing.T) {
	t.Parallel()

	ctx, e := newTestContext(t)

	// lazy reference step(?) before register
	require.NoError(t, e.RegisterParse("seq(?)", "sub step(?) fixed(2)"))
	// tx := NewTree("")
	// tx.Root.Append(e.ResolveOrLazy("step(?)"))
	// e.Register("tree(?)", tx)
	require.NoError(t, e.RegisterParse("fixed(?)", "ignore(?) inc"))
	require.NoError(t, e.RegisterParse("sub", "fixed(3) fixed(4)"))

	success := 0
	e.RegisterNewFunc("inc", func(ctx context.Context) error { success++; return nil })
	e.Register("step(?)", FuncArg{Name: "step", F: func(ctx context.Context, arg Arg) error {
		if arg == 42 || arg == 43 {
			success++
			return nil
		}
		err := errors.Errorf("unexpected arg=%v", arg)
		assert.NoError(t, err)
		return err
	}})

	e.TestDo(t, ctx, "seq(42)")
	assert.Equal(t, 1*4, success)
	e.TestDo(t, ctx, "seq(42)") // same arg again
	assert.Equal(t, 2*4, success)
	// e.TestDo(t, ctx, "tree(43)")
	// e.TestDo(t, ctx, "tree(43)") // same arg again
}

func TestParseText(t *testing.T) {
	t.Parallel()

	ctx, e := newTestContext(t)
	doHello, doWorld := &mockdo{}, Func0{F: func() error { return nil }}
	e.Register("hello", doHello) // eager register
	require.NoError(t, e.RegisterParse("subseq", "hello subarg(42)"))
	require.NoError(t, e.RegisterParse("subarg(?)", "world funarg(?)"))

	d, err := e.ParseText("root", "\n  hello\n  \n world   \n\nsubseq")
	require.NoError(t, err, "ParseText")

	err = d.Validate() // second action is not resolved
	require.Error(t, err)
	assert.Contains(t, err.Error(), "world not resolved")

	e.Register("world", doWorld) // lazy register after parse
	e.Register("funarg(?)", IgnoreArg{Nothing{}})
	require.NoError(t, d.Validate())
	assert.Zero(t, doHello.called)
	require.NoError(t, e.Exec(ctx, d))
	assert.Equal(t, int32(2), doHello.called)
}

func TestRegisterNewFunc(t *testing.T) {
	t.Parallel()

	ctx, e := newTestContext(t)
	mock := &mockdo{}
	e.RegisterNewFunc("lights-on", mock.Do)
	d := e.Resolve("lights-on")
	require.NoError(t, e.ValidateExec(ctx, d))
	assert.Equal(t, int32(1), mock.called)
}

func TestMeasure(t *testing.T) {
	t.Parallel()

	durs := make([]time.Duration, 0)
	mfun := func(d Doer, td time.Duration) {
		durs = append(durs, td)
		t.Logf("profile d=%s duration=%s\n", d.String(), td)
	}
	ctx, e := newTestContext(t)
	e.SetProfile(regexp.MustCompile(`^ignore`), 0, mfun)
	require.NoError(t, e.ValidateExec(ctx, e.Resolve("ignore(115)")))
	require.Len(t, durs, 1)
	assert.NotZero(t, durs[0])
}

func newTestContext(t testing.TB) (context.Context, *Engine) {
	log := log2.NewTest(t, log2.LDebug)
	e := NewEngine(log)
	ctx := context.Background()
	ctx = context.WithValue(ctx, log2.ContextKey, log)
	ctx = context.WithValue(ctx, ContextKey, e)
	return ctx, e
}

package native

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gopkg.in/src-d/go-errors.v1"

	"github.com/stretchr/testify/require"

	derrors "gopkg.in/bblfsh/sdk.v2/driver/errors"
	"gopkg.in/bblfsh/sdk.v2/uast/nodes"
)

func mockResponse(src string) nodes.Node {
	return nodes.Object{
		"root": nodes.Object{
			"key": nodes.String(src),
		},
	}
}

func TestEncoding(t *testing.T) {
	cases := []string{
		"test message",
	}
	encodings := []struct {
		enc Encoding
		exp []string
	}{
		{enc: UTF8, exp: cases},
		{enc: Base64, exp: []string{
			"dGVzdCBtZXNzYWdl",
		}},
	}

	for _, c := range encodings {
		enc, exp := Encoding(c.enc), c.exp
		t.Run(string(c.enc), func(t *testing.T) {
			for i, m := range cases {
				t.Run("", func(t *testing.T) {
					out, err := enc.Encode(m)
					require.NoError(t, err)
					require.Equal(t, exp[i], out)

					got, err := enc.Decode(out)
					require.NoError(t, err)
					require.Equal(t, m, got)
				})
			}
		})
	}
}

func TestNativeDriverNativeParse(t *testing.T) {
	require := require.New(t)

	d := NewDriverAt("internal/simple/mock", "")
	err := d.Start()
	require.NoError(err)

	r, err := d.Parse(context.Background(), "foo")
	require.NoError(err)
	require.Equal(mockResponse("foo"), r)

	err = d.Close()
	require.NoError(err)
}

func TestNativeDriverNativeParse_Lock(t *testing.T) {
	require := require.New(t)

	d := NewDriverAt("internal/simple/mock", "")
	err := d.Start()
	require.NoError(err)

	// it fails even with two, but is better having a big number, to identify
	// concurrency issues.
	count := 1000

	var wg sync.WaitGroup
	call := func(i int) {
		defer wg.Done()
		key := fmt.Sprintf("foo_%d", i)
		r, err := d.Parse(context.Background(), key)
		require.NoError(err)
		require.Equal(mockResponse(key), r)
	}

	wg.Add(count)
	for i := 0; i < count; i++ {
		go call(i)
	}

	wg.Wait()
	err = d.Close()
	require.NoError(err)
}

func TestNativeDriverStart_BadPath(t *testing.T) {
	require := require.New(t)

	d := NewDriverAt("non-existent", "")
	err := d.Start()
	require.Error(err)
}

func TestNativeDriverNativeParse_Malfunctioning(t *testing.T) {
	require := require.New(t)

	d := NewDriverAt("echo", "")

	err := d.Start()
	require.Nil(err)

	_, err = d.Parse(context.Background(), "foo")
	require.NotNil(err)
	require.True(derrors.ErrDriverFailure.Is(err))
}

func TestNativeDriverNativeParse_Malformed(t *testing.T) {
	require := require.New(t)

	d := NewDriverAt("yes", "")

	err := d.Start()
	require.NoError(err)

	_, err = d.Parse(context.Background(), "foo")
	require.NotNil(err)
	require.True(derrors.ErrDriverFailure.Is(err))
}

func TestNativeDriverParse_Timeout(t *testing.T) {
	require := require.New(t)

	// This test check if the Go driver server can recover from response read timeout.
	//
	// For this, we start a mock that will sleep 3 sec before answering any requests.
	//
	// On the client side, we will set the timeout of 1 sec, expecting the first request
	// to fail with an error. Then, we will fire a second request with no timeout and
	// will check if it will see the first response (lagged) or the second one (proper).

	d := NewDriverAt("internal/slow/mock", "")

	err := d.Start()
	require.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = d.Parse(ctx, "first")
	require.NotNil(err)
	require.True(derrors.ErrDriverFailure.Is(err))
	e, ok := err.(*errors.Error)
	require.True(ok, "%T", err)
	_, ok = e.Cause().(timeoutError)
	require.True(ok, "%T", e.Cause())

	r, err := d.Parse(context.Background(), "second")
	require.NoError(err)
	require.Equal(mockResponse("second"), r)
}

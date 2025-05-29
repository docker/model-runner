package tailbuffer

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogBufferCreation(t *testing.T) {
	lb := NewTailBuffer(0)
	require.NotNil(t, lb)
}

func TestLogBufferWrite(t *testing.T) {
	lb := NewTailBuffer(1024)
	n, err := lb.Write([]byte("asdf"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
}

func TestLogBufferReadEmpty(t *testing.T) {
	lb := NewTailBuffer(4)
	buf := make([]byte, 4)
	_, err := lb.Read(buf)
	require.Error(t, err, io.EOF)
}

func TestLogBufferWriteRead(t *testing.T) {
	lb := NewTailBuffer(4)
	n, err := lb.Write([]byte("asdfg"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	buf := make([]byte, 4)
	n, err = lb.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte("sdfg"), buf)
	n, err = lb.Write([]byte("hjklzx"))
	require.NoError(t, err)
	require.Equal(t, 6, n)
	buf = make([]byte, 3)
	n, err = lb.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.Equal(t, []byte("klz"), buf)
	n, err = lb.Read(buf)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestLogBufferWriteReadString(t *testing.T) {
	lb := NewTailBuffer(4)
	n, err := lb.Write([]byte("asdfg"))
	require.NoError(t, err)
	require.Equal(t, 5, n)
	str := new(strings.Builder)
	nw, err := io.Copy(str, lb)
	require.NoError(t, err)
	require.Equal(t, int64(4), nw)
	require.Equal(t, "sdfg", str.String())
}

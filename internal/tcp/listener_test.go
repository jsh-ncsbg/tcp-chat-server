package tcp

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeConnections(t *testing.T) {
	server := newTestServer(t)
	listener, result := startTestListener(t, server)
	alice := dialTestListener(t, server, listener, 1)
	alice.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	alice.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	bob := dialTestListener(t, server, listener, 2)
	bob.exchange(t, "REGISTER Friend\n", "INFO OK REGISTER\n")
	bob.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	assert.True(t, server.IsRegistered(1))
	assert.True(t, server.IsRegistered(2))

	alice.send(t, "MSG hello\n")
	assert.Equal(t, "MSG general Josh hello\n", alice.receive(t))
	assert.Equal(t, "MSG general Josh hello\n", bob.receive(t))
	require.NoError(t, alice.conn.Close())
	require.Eventually(t, func() bool { return !server.IsRegistered(1) }, clientTestTimeout, time.Millisecond)

	charlie := dialTestListener(t, server, listener, 3)
	charlie.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	charlie.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	assert.True(t, server.IsRegistered(3))
	charlie.send(t, "MSG back again\n")
	assert.Equal(t, "MSG general Josh back again\n", charlie.receive(t))
	assert.Equal(t, "MSG general Josh back again\n", bob.receive(t))

	require.NoError(t, listener.Close())
	require.NoError(t, awaitClientResult(t, result))
	bob.send(t, "MSG still connected\n")
	assert.Equal(t, "MSG general Friend still connected\n", bob.receive(t))
	assert.Equal(t, "MSG general Friend still connected\n", charlie.receive(t))
}

func TestServeContinuesAfterHandlerFailure(t *testing.T) {
	server := newTestServer(t)
	listener, _ := startTestListener(t, server)
	first := dialTestListener(t, server, listener, 1)
	first.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	io.WriteString(first.conn, strings.Repeat("x", bufio.MaxScanTokenSize+1)+"\n")
	require.Eventually(t, func() bool { return !server.IsRegistered(1) }, clientTestTimeout, time.Millisecond)

	second := dialTestListener(t, server, listener, 2)
	second.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	second.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	second.exchange(t, "MSG hello\n", "MSG general Josh hello\n")
	assert.True(t, server.IsRegistered(2))
}

func TestServeAcceptError(t *testing.T) {
	acceptFailure := errors.New("injected accept failure")
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "listener closed", err: net.ErrClosed},
		{name: "accept failed", err: acceptFailure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			listener := &failingListener{err: tt.err}
			err := Serve(chat.NewServer(), listener)
			if tt.err == net.ErrClosed {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, acceptFailure)
				assert.ErrorContains(t, err, "accept")
			}
			assert.True(t, listener.closed)
		})
	}
}

func startTestListener(t *testing.T, server *chat.Server) (net.Listener, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result <- Serve(server, listener)
	}()
	t.Cleanup(func() {
		listener.Close()
		select {
		case <-finished:
		case <-time.After(clientTestTimeout):
			t.Error("accept loop did not finish")
		}
	})
	return listener, result
}

func dialTestListener(t *testing.T, server *chat.Server, listener net.Listener, id chat.ClientID) *connectionPeer {
	t.Helper()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), clientTestTimeout)
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
		assert.Eventually(t, func() bool {
			return !server.IsRegistered(id) && server.RoomOf(id) == nil
		}, clientTestTimeout, time.Millisecond)
	})
	require.NoError(t, conn.SetDeadline(time.Now().Add(clientTestTimeout)))
	return &connectionPeer{conn: conn, reader: bufio.NewReader(conn)}
}

type failingListener struct {
	err    error
	closed bool
}

func (l *failingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (l *failingListener) Close() error {
	l.closed = true
	return nil
}

func (l *failingListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

package tcp

import (
	"bufio"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientWriteLoop(t *testing.T) {
	client, peer := newPipeClient(t)
	assert.Equal(t, chat.ClientID(1), client.ID())
	messages := []chat.Response{
		chat.NewInfoResponse("connected"),
		chat.NewMsgResponse("general", "Josh", "hello world"),
		chat.NewErrResponse("INVALID", "invalid command"),
	}
	for _, msg := range messages {
		require.NoError(t, client.Deliver(msg))
	}

	result := startClientWriter(client)
	reader := bufio.NewReader(peer)
	for _, msg := range messages {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		assert.Equal(t, msg.String()+"\n", line)
	}
	require.NoError(t, client.Close())
	require.NoError(t, awaitClientResult(t, result))
}

func TestClientQueueFull(t *testing.T) {
	client, peer := newPipeClient(t)
	msg := chat.NewInfoResponse("queued")
	for i := 0; i < MsgBufferSize; i++ {
		require.NoError(t, client.Deliver(msg))
	}
	require.ErrorIs(t, client.Deliver(msg), ErrQueueFull)

	result := startClientWriter(client)
	reader := bufio.NewReader(peer)
	for i := 0; i < MsgBufferSize; i++ {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		assert.Equal(t, msg.String()+"\n", line)
	}
	require.NoError(t, client.Deliver(chat.NewInfoResponse("available again")))
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "INFO available again\n", line)
	require.NoError(t, client.Close())
	require.NoError(t, awaitClientResult(t, result))
}

func TestClientCloseBeforeWriter(t *testing.T) {
	conn, _ := newTestPipe(t)
	var writes atomic.Int32
	client := NewClient(1, &clientTestConn{
		Conn: conn,
		write: func(p []byte) (int, error) {
			writes.Add(1)
			return conn.Write(p)
		},
	})
	t.Cleanup(func() { client.Close() })
	for i := 0; i < MsgBufferSize; i++ {
		require.NoError(t, client.Deliver(chat.NewInfoResponse("pending")))
	}
	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
	assert.ErrorIs(t, client.Deliver(chat.NewInfoResponse("late")), ErrClientClosed)
	require.NoError(t, awaitClientResult(t, startClientWriter(client)))
	assert.Equal(t, int32(0), writes.Load(), "queued responses must be abandoned")
}

func TestClientCloseIdleWriter(t *testing.T) {
	client, _ := newPipeClient(t)
	result := startClientWriter(client)
	require.NoError(t, client.Close())
	require.NoError(t, awaitClientResult(t, result))
	assert.ErrorIs(t, client.Deliver(chat.NewInfoResponse("late")), ErrClientClosed)
}

func TestClientCloseBlockedWriter(t *testing.T) {
	conn, _ := newTestPipe(t)
	var writes atomic.Int32
	started := make(chan struct{})
	client := NewClient(1, &clientTestConn{
		Conn: conn,
		write: func(p []byte) (int, error) {
			if writes.Add(1) == 1 {
				close(started)
			}
			return conn.Write(p)
		},
	})
	t.Cleanup(func() { client.Close() })
	require.NoError(t, client.Deliver(chat.NewInfoResponse("blocked")))
	require.NoError(t, client.Deliver(chat.NewInfoResponse("pending")))
	result := startClientWriter(client)
	select {
	case <-started:
	case <-time.After(clientTestTimeout):
		t.Fatal("writer did not start socket I/O")
	}

	require.NoError(t, client.Close())
	require.NoError(t, awaitClientResult(t, result))
	assert.Equal(t, int32(1), writes.Load(), "shutdown must not write pending responses")
	assert.ErrorIs(t, client.Deliver(chat.NewInfoResponse("late")), ErrClientClosed)
}

func TestClientWriteFailure(t *testing.T) {
	writeFailure := errors.New("socket write failed")
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "write error", err: writeFailure},
		{name: "short write", err: io.ErrShortWrite},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn, peer := newTestPipe(t)
			client := NewClient(1, &clientTestConn{
				Conn: conn,
				write: func(p []byte) (int, error) {
					if tt.err == io.ErrShortWrite {
						return len(p) - 1, nil
					}
					return 0, tt.err
				},
			})
			t.Cleanup(func() { client.Close() })
			require.NoError(t, client.Deliver(chat.NewInfoResponse("hello")))

			err := awaitClientResult(t, startClientWriter(client))
			assert.ErrorIs(t, err, tt.err)
			assert.ErrorContains(t, err, "client write")
			assert.ErrorIs(t, client.Deliver(chat.NewInfoResponse("late")), ErrClientClosed)
			_, err = peer.Read(make([]byte, 1))
			assert.ErrorIs(t, err, io.EOF, "a write failure must close the socket")
		})
	}
}

func TestClientCloseResult(t *testing.T) {
	conn, _ := newTestPipe(t)
	closeFailure := errors.New("socket close failed")
	var closes atomic.Int32
	client := NewClient(1, &clientTestConn{
		Conn: conn,
		close: func() error {
			closes.Add(1)
			conn.Close()
			return closeFailure
		},
	})
	t.Cleanup(func() { client.Close() })
	const callers = 16
	results := make(chan error, callers)
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		go func() {
			<-start
			results <- client.Close()
		}()
	}
	close(start)
	for i := 0; i < callers; i++ {
		assert.ErrorIs(t, awaitClientResult(t, results), closeFailure)
	}
	assert.Equal(t, int32(1), closes.Load())
	assert.ErrorIs(t, client.Close(), closeFailure)
}

func TestClientConcurrentDeliverAndClose(t *testing.T) {
	client, _ := newPipeClient(t)
	const senders = 8
	results := make(chan error, senders+1)
	start := make(chan struct{})
	for i := 0; i < senders; i++ {
		go func() {
			<-start
			for j := 0; j < 128; j++ {
				err := client.Deliver(chat.NewInfoResponse("hello"))
				if err != nil && !errors.Is(err, ErrQueueFull) && !errors.Is(err, ErrClientClosed) {
					results <- err
					return
				}
			}
			results <- nil
		}()
	}
	go func() {
		<-start
		results <- client.Close()
	}()
	close(start)
	for i := 0; i < senders+1; i++ {
		require.NoError(t, awaitClientResult(t, results))
	}
	assert.ErrorIs(t, client.Deliver(chat.NewInfoResponse("late")), ErrClientClosed)
}

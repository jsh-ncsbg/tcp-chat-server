package tcp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCommand(t *testing.T) {
	for _, tt := range []struct {
		line     string
		name     string
		argument string
		err      error
	}{
		{line: "REGISTER Josh", name: "REGISTER", argument: "Josh"},
		{line: " \tJOIN\t general  ", name: "JOIN", argument: "general"},
		{line: "LEAVE", name: "LEAVE"},
		{line: " \tLEAVE \t", name: "LEAVE"},
		{line: "QUIT", name: "QUIT"},
		{line: " \tQUIT \t", name: "QUIT"},
		{line: "MSG\t hello  world  ", name: "MSG", argument: "hello  world  "},
		{line: "", err: errInvalidCommand},
		{line: " \t", err: errInvalidCommand},
		{line: "register Josh", err: errInvalidCommand},
		{line: "quit", err: errInvalidCommand},
		{line: "QUIT now", err: errInvalidArguments},
		{line: "REGISTER", err: errInvalidArguments},
		{line: "REGISTER Josh Smith", err: errInvalidArguments},
		{line: "REGISTER Josh\u2003Smith", err: errInvalidArguments},
		{line: "JOIN general extra", err: errInvalidArguments},
		{line: "LEAVE breakout-1", err: errInvalidArguments},
		{line: "MSG", err: errInvalidArguments},
		{line: "MSG \t\u2003", err: errInvalidArguments},
	} {
		t.Run(tt.line, func(t *testing.T) {
			name, argument, err := parseCommand(tt.line)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.name, name)
			assert.Equal(t, tt.argument, argument)
		})
	}
}

func TestCommandErrorResponse(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code string
	}{
		{errInvalidCommand, "INVALID_COMMAND"},
		{errInvalidArguments, "INVALID_ARGUMENTS"},
		{chat.ErrInvalidUsername, "INVALID_ARGUMENTS"},
		{chat.ErrDuplicateUsername, "USERNAME_TAKEN"},
		{chat.ErrAlreadyRegistered, "ALREADY_REGISTERED"},
		{chat.ErrUnregisteredClient, "UNREGISTERED"},
		{chat.ErrRoomNotFound, "ROOM_NOT_FOUND"},
		{chat.ErrNotInRoom, "NOT_IN_ROOM"},
		{chat.ErrUnknownClient, "INTERNAL"},
		{errors.New("unexpected failure"), "INTERNAL"},
	} {
		t.Run(tt.err.Error(), func(t *testing.T) {
			response := commandErrorResponse(fmt.Errorf("wrapped: %w", tt.err))
			assert.Equal(t, chat.ErrorResponse, response.Type)
			assert.True(t, strings.HasPrefix(response.String(), "ERR "+tt.code+" "), response.String())
			if tt.code == "INTERNAL" {
				assert.Equal(t, "ERR INTERNAL internal server error", response.String())
			}
		})
	}
}

func TestConnectionCommandRecovery(t *testing.T) {
	server := newTestServer(t)
	peer := newConnectionPeer(t, server, 1)
	for _, tt := range []struct {
		command string
		code    string
	}{
		{"\n", "INVALID_COMMAND"},
		{"register Josh\n", "INVALID_COMMAND"},
		{"CREATE room\n", "INVALID_COMMAND"},
		{"quit\n", "INVALID_COMMAND"},
		{"QUIT now\n", "INVALID_ARGUMENTS"},
		{"REGISTER\n", "INVALID_ARGUMENTS"},
		{"REGISTER Josh Smith\n", "INVALID_ARGUMENTS"},
		{"JOIN general extra\n", "INVALID_ARGUMENTS"},
		{"LEAVE general\n", "INVALID_ARGUMENTS"},
		{"MSG \t\n", "INVALID_ARGUMENTS"},
		{"JOIN general\n", "UNREGISTERED"},
		{"LEAVE\n", "UNREGISTERED"},
		{"MSG hello\n", "UNREGISTERED"},
	} {
		peer.send(t, tt.command)
		response := peer.receive(t)
		assert.True(t, strings.HasPrefix(response, "ERR "+tt.code+" "), response)
	}
	assert.False(t, server.IsRegistered(1))
	assert.Nil(t, server.RoomOf(1))

	peer.exchange(t, "REGISTER Josh\r\n", "INFO OK REGISTER\n")
	for _, tt := range []struct {
		command string
		code    string
	}{
		{"REGISTER Josh\n", "ALREADY_REGISTERED"},
		{"REGISTER James\n", "ALREADY_REGISTERED"},
		{"MSG hello\n", "NOT_IN_ROOM"},
		{"JOIN missing\n", "ROOM_NOT_FOUND"},
		{"LEAVE missing\n", "INVALID_ARGUMENTS"},
	} {
		peer.send(t, tt.command)
		response := peer.receive(t)
		assert.True(t, strings.HasPrefix(response, "ERR "+tt.code+" "), response)
	}
	peer.exchange(t, " \tJOIN\tgeneral \r\n", "INFO OK JOIN\n")
	peer.exchange(t, "MSG hello  world  \n", "MSG general Josh hello  world  \n")
	peer.send(t, "QUIT now\n")
	assert.True(t, strings.HasPrefix(peer.receive(t), "ERR INVALID_ARGUMENTS "))
	assert.True(t, server.IsRegistered(1))
	peer.exchange(t, "MSG still here\n", "MSG general Josh still here\n")
	peer.send(t, "LEAVE general\n")
	assert.True(t, strings.HasPrefix(peer.receive(t), "ERR INVALID_ARGUMENTS "))
	require.NotNil(t, server.RoomOf(1))
	assert.Equal(t, chat.RoomID("general"), server.RoomOf(1).ID())
	peer.exchange(t, "LEAVE\r\n", "INFO OK LEAVE\n")
	assert.Nil(t, server.RoomOf(1))
	peer.exchange(t, "LEAVE \t\n", "INFO OK LEAVE\n")
	peer.send(t, "MSG roomless\n")
	assert.True(t, strings.HasPrefix(peer.receive(t), "ERR NOT_IN_ROOM "))
	require.NoError(t, peer.conn.Close())
	require.NoError(t, awaitClientResult(t, peer.result))
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionQuit(t *testing.T) {
	for _, state := range []struct {
		name       string
		registered bool
		joined     bool
	}{
		{name: "unauthenticated"},
		{name: "registered", registered: true},
		{name: "joined", registered: true, joined: true},
	} {
		for _, ending := range []struct {
			name string
			text string
		}{
			{name: "LF", text: "\n"},
			{name: "CRLF", text: "\r\n"},
		} {
			t.Run(state.name+"/"+ending.name, func(t *testing.T) {
				server := newTestServer(t)
				peer := newConnectionPeer(t, server, 1)
				if state.registered {
					peer.register(t, "Josh")
				}
				if state.joined {
					peer.join(t, "general")
				}

				peer.send(t, " \tQUIT \t"+ending.text)
				_, err := peer.reader.ReadByte()
				require.ErrorIs(t, err, io.EOF)
				require.NoError(t, awaitClientResult(t, peer.result))
				assertConnectionRemoved(t, server, 1, "Josh")
			})
		}
	}
}

func TestConnectionQuitIgnoresBufferedCommands(t *testing.T) {
	server := newTestServer(t)
	sender := newConnectionPeer(t, server, 1)
	observer := newConnectionPeer(t, server, 2)
	sender.register(t, "Josh")
	sender.join(t, "general")
	observer.register(t, "Observer")
	observer.join(t, "general")

	sender.send(t, "QUIT\nMSG should not appear\n")
	_, err := sender.reader.ReadByte()
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, awaitClientResult(t, sender.result))
	assertConnectionRemoved(t, server, 1, "Josh")
	observer.exchange(t, "MSG marker\n", "MSG general Observer marker\n")
}

func TestConnectionQuitInterruptsBlockedWriter(t *testing.T) {
	server := newTestServer(t)
	conn, remote := newTestPipe(t)
	started := make(chan struct{})
	var writes atomic.Int32
	peer := startConnectionPeer(t, server, 1, &clientTestConn{
		Conn: conn,
		write: func(p []byte) (int, error) {
			if writes.Add(1) == 3 {
				close(started)
			}
			return conn.Write(p)
		},
	}, remote)
	peer.register(t, "Josh")
	peer.join(t, "general")
	peer.send(t, "MSG blocked\n")
	select {
	case <-started:
	case <-time.After(clientTestTimeout):
		t.Fatal("writer did not start socket I/O")
	}
	require.NoError(t, server.HandleMessage(1, "pending"))

	peer.send(t, "QUIT\n")
	require.NoError(t, awaitClientResult(t, peer.result))
	_, err := peer.reader.ReadByte()
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, int32(3), writes.Load())
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionRoomRouting(t *testing.T) {
	server := newTestServer(t)
	alice := newConnectionPeer(t, server, 1)
	bob := newConnectionPeer(t, server, 2)
	alice.register(t, "Josh")
	bob.send(t, "REGISTER Josh\n")
	assert.True(t, strings.HasPrefix(bob.receive(t), "ERR USERNAME_TAKEN "))
	bob.register(t, "Friend")
	alice.join(t, "general")
	bob.join(t, "general")

	alice.send(t, "MSG hello\n")
	assert.Equal(t, "MSG general Josh hello\n", alice.receive(t))
	assert.Equal(t, "MSG general Josh hello\n", bob.receive(t))
	alice.join(t, "breakout-1")
	bob.exchange(t, "MSG old room\n", "MSG general Friend old room\n")
	alice.exchange(t, "MSG new room\n", "MSG breakout-1 Josh new room\n")
	bob.exchange(t, "LEAVE\n", "INFO OK LEAVE\n")
	alice.exchange(t, "LEAVE\n", "INFO OK LEAVE\n")
	alice.send(t, "MSG roomless\n")
	assert.True(t, strings.HasPrefix(alice.receive(t), "ERR NOT_IN_ROOM "))

	require.NoError(t, alice.conn.Close())
	require.NoError(t, bob.conn.Close())
	require.NoError(t, awaitClientResult(t, alice.result))
	require.NoError(t, awaitClientResult(t, bob.result))
	assertConnectionRemoved(t, server, 1, "Josh")
	assertConnectionRemoved(t, server, 2, "Friend")
}

func TestConnectionDuplicateID(t *testing.T) {
	server := newTestServer(t)
	original := newConnectionPeer(t, server, 1)
	original.register(t, "Josh")
	original.join(t, "general")
	peer := newConnectionPeer(t, server, 1)

	assert.ErrorIs(t, awaitClientResult(t, peer.result), chat.ErrDuplicateClientID)
	_, err := peer.conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
	assert.True(t, server.IsRegistered(1))
	require.NotNil(t, server.RoomOf(1))
	assert.Equal(t, chat.RoomID("general"), server.RoomOf(1).ID())
	original.exchange(t, "MSG still connected\n", "MSG general Josh still connected\n")
}

func TestConnectionOversizedInput(t *testing.T) {
	server := newTestServer(t)
	peer := newConnectionPeer(t, server, 1)
	peer.register(t, "Josh")
	peer.join(t, "general")
	io.WriteString(peer.conn, strings.Repeat("x", bufio.MaxScanTokenSize+1)+"\n")

	err := awaitClientResult(t, peer.result)
	require.Error(t, err)
	assert.ErrorContains(t, err, "connection read")
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionWriteFailure(t *testing.T) {
	server := newTestServer(t)
	conn, remote := newTestPipe(t)
	writeFailure := errors.New("injected write failure")
	var writes atomic.Int32
	peer := startConnectionPeer(t, server, 1, &clientTestConn{
		Conn: conn,
		write: func(p []byte) (int, error) {
			if writes.Add(1) > 2 {
				return 0, writeFailure
			}
			return conn.Write(p)
		},
	}, remote)
	peer.register(t, "Josh")
	peer.join(t, "general")
	peer.send(t, "MSG fail\n")

	assert.ErrorIs(t, awaitClientResult(t, peer.result), writeFailure)
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionReadAndCleanupFailure(t *testing.T) {
	server := newTestServer(t)
	conn, remote := newTestPipe(t)
	input := strings.NewReader("REGISTER Josh\nJOIN general\n")
	readFailure := errors.New("injected read failure")
	closeFailure := errors.New("injected close failure")
	peer := startConnectionPeer(t, server, 1, &clientTestConn{
		Conn: conn,
		read: func(p []byte) (int, error) {
			n, err := input.Read(p)
			if err == io.EOF {
				return 0, readFailure
			}
			return n, err
		},
		close: func() error {
			conn.Close()
			return closeFailure
		},
	}, remote)

	err := awaitClientResult(t, peer.result)
	assert.ErrorIs(t, err, readFailure)
	assert.ErrorIs(t, err, closeFailure)
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionResponseQueueFull(t *testing.T) {
	server := newTestServer(t)
	peer := newConnectionPeer(t, server, 1)
	peer.register(t, "Josh")
	peer.join(t, "general")
	peer.send(t, strings.Repeat("JOIN general\n", MsgBufferSize+2))

	assert.ErrorIs(t, awaitClientResult(t, peer.result), ErrQueueFull)
	assertConnectionRemoved(t, server, 1, "Josh")
}

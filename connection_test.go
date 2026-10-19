package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		{line: "LEAVE breakout-1", name: "LEAVE", argument: "breakout-1"},
		{line: "MSG\t hello  world  ", name: "MSG", argument: "hello  world  "},
		{line: "", err: errInvalidCommand},
		{line: " \t", err: errInvalidCommand},
		{line: "register Josh", err: errInvalidCommand},
		{line: "QUIT", err: errInvalidCommand},
		{line: "REGISTER", err: errInvalidArguments},
		{line: "REGISTER Josh Smith", err: errInvalidArguments},
		{line: "REGISTER Josh\u2003Smith", err: errInvalidArguments},
		{line: "JOIN general extra", err: errInvalidArguments},
		{line: "LEAVE", err: errInvalidArguments},
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
		{ErrInvalidUsername, "INVALID_ARGUMENTS"},
		{ErrDuplicateUsername, "USERNAME_TAKEN"},
		{ErrAlreadyRegistered, "ALREADY_REGISTERED"},
		{ErrUnregisteredClient, "UNREGISTERED"},
		{ErrRoomNotFound, "ROOM_NOT_FOUND"},
		{ErrNotInRoom, "NOT_IN_ROOM"},
		{ErrUnknownClient, "INTERNAL"},
		{errors.New("unexpected failure"), "INTERNAL"},
	} {
		t.Run(tt.err.Error(), func(t *testing.T) {
			response := commandErrorResponse(fmt.Errorf("wrapped: %w", tt.err))
			assert.Equal(t, ErrorResponse, response.Type)
			assert.True(t, strings.HasPrefix(response.String(), "ERR "+tt.code+" "), response.String())
			if tt.code == "INTERNAL" {
				assert.Equal(t, "ERR INTERNAL internal server error", response.String())
			}
		})
	}
}

func TestConnectionCommandRecovery(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	peer := newConnectionPeer(t, server, 1)
	for _, tt := range []struct {
		command string
		code    string
	}{
		{"\n", "INVALID_COMMAND"},
		{"register Josh\n", "INVALID_COMMAND"},
		{"CREATE room\n", "INVALID_COMMAND"},
		{"REGISTER\n", "INVALID_ARGUMENTS"},
		{"REGISTER Josh Smith\n", "INVALID_ARGUMENTS"},
		{"JOIN general extra\n", "INVALID_ARGUMENTS"},
		{"LEAVE\n", "INVALID_ARGUMENTS"},
		{"MSG \t\n", "INVALID_ARGUMENTS"},
		{"JOIN general\n", "UNREGISTERED"},
		{"LEAVE general\n", "UNREGISTERED"},
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
		{"LEAVE missing\n", "ROOM_NOT_FOUND"},
	} {
		peer.send(t, tt.command)
		response := peer.receive(t)
		assert.True(t, strings.HasPrefix(response, "ERR "+tt.code+" "), response)
	}
	peer.exchange(t, " \tJOIN\tgeneral \r\n", "INFO OK JOIN\n")
	peer.exchange(t, "MSG hello  world  \n", "MSG general Josh hello  world  \n")
	require.NoError(t, peer.conn.Close())
	require.NoError(t, awaitClientResult(t, peer.result))
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionRoomRouting(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	alice := newConnectionPeer(t, server, 1)
	bob := newConnectionPeer(t, server, 2)
	alice.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	bob.send(t, "REGISTER Josh\n")
	assert.True(t, strings.HasPrefix(bob.receive(t), "ERR USERNAME_TAKEN "))
	bob.exchange(t, "REGISTER Friend\n", "INFO OK REGISTER\n")
	alice.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	bob.exchange(t, "JOIN general\n", "INFO OK JOIN\n")

	alice.send(t, "MSG hello\n")
	assert.Equal(t, "MSG general Josh hello\n", alice.receive(t))
	assert.Equal(t, "MSG general Josh hello\n", bob.receive(t))
	alice.exchange(t, "JOIN breakout-1\n", "INFO OK JOIN\n")
	bob.exchange(t, "MSG old room\n", "MSG general Friend old room\n")
	alice.exchange(t, "MSG new room\n", "MSG breakout-1 Josh new room\n")
	bob.exchange(t, "LEAVE general\n", "INFO OK LEAVE\n")
	alice.exchange(t, "LEAVE breakout-1\n", "INFO OK LEAVE\n")
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
	server, err := newDefaultServer()
	require.NoError(t, err)
	original := newFakeClient(1)
	registerAndJoin(t, server, server.rooms["general"], original, "Josh")
	peer := newConnectionPeer(t, server, 1)

	assert.ErrorIs(t, awaitClientResult(t, peer.result), ErrDuplicateClientID)
	_, err = peer.conn.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
	assert.True(t, server.IsRegistered(1))
	assert.Same(t, server.rooms["general"], server.RoomOf(1))
	assert.Same(t, original, server.clients[1].connection)
}

func TestConnectionOversizedInput(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	peer := newConnectionPeer(t, server, 1)
	peer.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	peer.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	io.WriteString(peer.conn, strings.Repeat("x", bufio.MaxScanTokenSize+1)+"\n")

	err = awaitClientResult(t, peer.result)
	require.Error(t, err)
	assert.ErrorContains(t, err, "connection read")
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionWriteFailure(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	conn, remote := net.Pipe()
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
	peer.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	peer.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	peer.send(t, "MSG fail\n")

	assert.ErrorIs(t, awaitClientResult(t, peer.result), writeFailure)
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionReadAndCleanupFailure(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	conn, remote := net.Pipe()
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

	err = awaitClientResult(t, peer.result)
	assert.ErrorIs(t, err, readFailure)
	assert.ErrorIs(t, err, closeFailure)
	assertConnectionRemoved(t, server, 1, "Josh")
}

func TestConnectionResponseQueueFull(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	peer := newConnectionPeer(t, server, 1)
	peer.exchange(t, "REGISTER Josh\n", "INFO OK REGISTER\n")
	peer.exchange(t, "JOIN general\n", "INFO OK JOIN\n")
	peer.send(t, strings.Repeat("JOIN general\n", MsgBufferSize+2))

	assert.ErrorIs(t, awaitClientResult(t, peer.result), ErrQueueFull)
	assertConnectionRemoved(t, server, 1, "Josh")
}

type connectionPeer struct {
	conn   net.Conn
	reader *bufio.Reader
	result <-chan error
}

func newConnectionPeer(t *testing.T, server *Server, id ClientID) *connectionPeer {
	t.Helper()
	conn, peer := net.Pipe()
	return startConnectionPeer(t, server, id, conn, peer)
}

func startConnectionPeer(t *testing.T, server *Server, id ClientID, conn, peer net.Conn) *connectionPeer {
	t.Helper()
	require.NoError(t, peer.SetDeadline(time.Now().Add(clientTestTimeout)))
	result := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		result <- HandleConnection(server, id, conn)
	}()
	t.Cleanup(func() {
		peer.Close()
		select {
		case <-finished:
		case <-time.After(clientTestTimeout):
			t.Error("connection handler did not finish")
		}
	})
	return &connectionPeer{conn: peer, reader: bufio.NewReader(peer), result: result}
}

func (p *connectionPeer) send(t *testing.T, command string) {
	t.Helper()
	_, err := io.WriteString(p.conn, command)
	require.NoError(t, err)
}

func (p *connectionPeer) receive(t *testing.T) string {
	t.Helper()
	line, err := p.reader.ReadString('\n')
	require.NoError(t, err)
	return line
}

func (p *connectionPeer) exchange(t *testing.T, command, response string) {
	t.Helper()
	p.send(t, command)
	assert.Equal(t, response, p.receive(t))
}

func assertConnectionRemoved(t *testing.T, server *Server, id ClientID, username string) {
	t.Helper()
	server.mu.RLock()
	defer server.mu.RUnlock()
	assert.NotContains(t, server.clients, id)
	assert.NotContains(t, server.usernames, username)
	for _, members := range server.memberships {
		assert.NotContains(t, members, id)
	}
}

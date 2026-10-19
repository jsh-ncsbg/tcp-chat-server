package tcp

import (
	"bufio"
	"io"
	"net"
	"testing"
	"time"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const clientTestTimeout = 5 * time.Second

func newTestPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	conn, peer := net.Pipe()
	t.Cleanup(func() {
		conn.Close()
		peer.Close()
	})
	require.NoError(t, peer.SetDeadline(time.Now().Add(clientTestTimeout)))
	return conn, peer
}

func newPipeClient(t *testing.T) (*client, net.Conn) {
	t.Helper()
	conn, peer := newTestPipe(t)
	client := NewClient(1, conn)
	t.Cleanup(func() { client.Close() })
	return client, peer
}

func startClientWriter(client *client) <-chan error {
	result := make(chan error, 1)
	go func() { result <- client.WriteLoop() }()
	return result
}

func awaitClientResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(clientTestTimeout):
		t.Fatal("client operation did not finish")
		return nil
	}
}

type clientTestConn struct {
	net.Conn
	read  func([]byte) (int, error)
	write func([]byte) (int, error)
	close func() error
}

func (c *clientTestConn) Read(p []byte) (int, error) {
	if c.read != nil {
		return c.read(p)
	}
	return c.Conn.Read(p)
}

func (c *clientTestConn) Write(p []byte) (int, error) {
	if c.write != nil {
		return c.write(p)
	}
	return c.Conn.Write(p)
}

func (c *clientTestConn) Close() error {
	if c.close != nil {
		return c.close()
	}
	return c.Conn.Close()
}

type connectionPeer struct {
	conn   net.Conn
	reader *bufio.Reader
	result <-chan error
}

func newConnectionPeer(t *testing.T, server *chat.Server, id chat.ClientID) *connectionPeer {
	t.Helper()
	conn, peer := newTestPipe(t)
	return startConnectionPeer(t, server, id, conn, peer)
}

func startConnectionPeer(t *testing.T, server *chat.Server, id chat.ClientID, conn, peer net.Conn) *connectionPeer {
	t.Helper()
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

func assertConnectionRemoved(t *testing.T, server *chat.Server, id chat.ClientID, username string) {
	t.Helper()
	assert.False(t, server.IsRegistered(id))
	assert.Nil(t, server.RoomOf(id))
	require.NoError(t, server.Connect(&cleanupProbe{id: id}))
	defer server.Disconnect(id)
	require.NoError(t, server.Register(id, username))
	assert.Nil(t, server.RoomOf(id))
	assert.ErrorIs(t, server.HandleMessage(id, "roomless"), chat.ErrNotInRoom)
}

func newTestServer(t *testing.T) *chat.Server {
	t.Helper()
	server := chat.NewServer()
	for _, id := range []chat.RoomID{"general", "breakout-1"} {
		require.NoError(t, server.AddRoom(chat.NewRoom(id)))
	}
	return server
}

type cleanupProbe struct {
	id chat.ClientID
}

func (p *cleanupProbe) ID() chat.ClientID {
	return p.id
}

func (p *cleanupProbe) Deliver(chat.Response) error {
	return nil
}

func (p *connectionPeer) register(t *testing.T, username string) {
	t.Helper()
	p.exchange(t, "REGISTER "+username+"\n", "INFO OK REGISTER\n")
}

func (p *connectionPeer) join(t *testing.T, room string) {
	t.Helper()
	p.exchange(t, "JOIN "+room+"\n", "INFO OK JOIN\n")
}

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("client can register", func(t *testing.T) {
		server := NewServer()
		client := NewClient(2, "Josh")

		registerClients(t, server, client)

		assert.True(t, server.IsRegistered(client.ID()))
		assert.False(t, server.IsRegistered(1))
	})

	t.Run("duplicate username is rejected", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1, "Josh")
		dup := NewClient(2, "Josh")

		registerClients(t, server, client)

		err := server.Register(dup)
		assert.ErrorIs(t, err, ErrDuplicateUsername)
	})

	t.Run("room creation", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")

		addRooms(t, server, room)

		assert.True(t, server.RoomExists(room.ID()))
	})

	t.Run("reject duplicate room name", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		dup := NewRoom("general")

		addRooms(t, server, room)

		err := server.AddRoom(dup)
		assert.ErrorIs(t, err, ErrDuplicateRoom)
	})
}

func TestJoin(t *testing.T) {
	t.Run("client can join room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1, "Josh")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client)

		got := server.RoomOf(client.ID())
		assert.Equal(t, room, got)
	})

	t.Run("client can switch rooms", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1, "Josh")
		roomA := NewRoom("A")
		roomB := NewRoom("B")

		registerClients(t, server, client)
		addRooms(t, server, roomA, roomB)

		joinRoom(t, server, roomA, client)
		require.Equal(t, roomA, server.RoomOf(client.ID()))

		joinRoom(t, server, roomB, client)
		require.Equal(t, roomB, server.RoomOf(client.ID()))
	})

	t.Run("unknown client or room returns an error", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1, "Josh")

		addRooms(t, server, room)
		registerClients(t, server, client)

		err := server.Join(2, room.ID())
		assert.ErrorIs(t, err, ErrClientNotFound)
		err = server.Join(client.ID(), "unknown")
		assert.ErrorIs(t, err, ErrRoomNotFound)
	})
}

func TestLeave(t *testing.T) {
	t.Run("client can leave room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1, "Josh")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client)

		err := server.Leave(client.ID(), room.ID())
		assert.NoError(t, err)
		assert.Nil(t, server.RoomOf(client.ID()))
	})

	t.Run("invalid client or room returns an error", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1, "Josh")
		room := NewRoom("general")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client)

		err := server.Leave(2, room.ID())
		assert.ErrorIs(t, err, ErrClientNotFound)
		err = server.Leave(client.ID(), "unknown")
		assert.ErrorIs(t, err, ErrRoomNotFound)
	})
}

func TestDisconnect(t *testing.T) {
	t.Run("remove client from registered clients", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1, "Josh")

		registerClients(t, server, client)

		err := server.Disconnect(client.ID())
		require.NoError(t, err)

		assert.False(t, server.IsRegistered(client.ID()))
	})

	t.Run("remove membership", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1, "Josh")
		room := NewRoom("general")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client)

		err := server.Disconnect(client.ID())
		require.NoError(t, err)
		assert.Nil(t, server.RoomOf(client.ID()))
	})
}

func TestBroadcast(t *testing.T) {
	t.Run("broadcasts to all client in that room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := newFakeClient(1, "Josh")
		sameRoomClient := newFakeClient(2, "Friend")
		outsider := newFakeClient(3, "outsider")

		addRooms(t, server, room)
		registerClients(t, server, outsider)
		registerAndJoin(t, server, room, client, sameRoomClient)

		content := "sample message content"
		server.HandleMessage(client.ID(), content)

		expected := client.Username() + ": " + content

		assert.Equal(t, expected, client.inbox[0])
		assert.Equal(t, expected, sameRoomClient.inbox[0])
		assert.Empty(t, outsider.inbox)
	})
	t.Run("unregistered clients gets declined", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1, "")

		registerClients(t, server, client)
		server.HandleMessage(client.ID(), "nonsense message")

		expected := "server: " + ErrUnregisteredClient.Error()
		got := client.inbox[0]
		assert.Equal(t, expected, got)
	})

	t.Run("clients with no room are declined", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1, "Josh")

		registerClients(t, server, client)
		server.HandleMessage(client.ID(), "sample message")

		expected := "server: " + ErrRoomNotFound.Error()
		got := client.inbox[0]
		assert.Equal(t, expected, got)
	})
}

// ==== HELPER FUNCTIONS

func registerClients(t testing.TB, s *Server, clients ...Client) {
	t.Helper()
	for _, c := range clients {
		err := s.Register(c)
		require.NoError(t, err)
	}
}

func joinRoom(t testing.TB, s *Server, room *Room, clients ...Client) {
	t.Helper()
	for _, c := range clients {
		err := s.Join(c.ID(), room.ID())
		require.NoError(t, err)
	}
}

func registerAndJoin(t testing.TB, s *Server, room *Room, clients ...Client) {
	t.Helper()
	registerClients(t, s, clients...)
	for _, c := range clients {
		joinRoom(t, s, room, c)
	}
}

func addRooms(t testing.TB, s *Server, rooms ...*Room) {
	t.Helper()
	for _, r := range rooms {
		err := s.AddRoom(r)
		require.NoError(t, err)
	}
}

func newFakeClient(id ClientID, u string) *fakeClient {
	return &fakeClient{
		id:    id,
		u:     u,
		inbox: []string{},
	}
}

// === MOCKS / SPIES / FAKES

type fakeClient struct {
	id    ClientID
	u     string
	inbox []string
}

func (fc *fakeClient) Username() string {
	return fc.u
}

func (fc *fakeClient) ID() ClientID {
	return fc.id
}

func (fc *fakeClient) Deliver(msg Message) error {
	fc.inbox = append(fc.inbox, fmt.Sprintf("%s: %s", msg.From, msg.Content))
	return nil
}

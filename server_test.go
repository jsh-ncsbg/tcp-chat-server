package main

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("client can register", func(t *testing.T) {
		server := NewServer()
		client := NewClient(2)

		connectAndRegister(t, server, client, "Josh")

		assert.True(t, server.IsRegistered(client.ID()))
		assert.False(t, server.IsRegistered(1))
	})

	t.Run("duplicate client id is handled", func(t *testing.T) {
		// server error
		server := NewServer()
		client := NewClient(2)
		dup := NewClient(2)

		connectAndRegister(t, server, client, "Josh")

		err := server.Connect(dup)
		assert.ErrorIs(t, err, ErrDuplicateClientID)
		assert.Same(t, client, server.clients[client.ID()].connection)
		assert.True(t, server.IsRegistered(client.ID()))
	})

	t.Run("duplicate username is rejected", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1)
		dup := NewClient(2)

		connectAndRegister(t, server, client, "Josh")

		connectClients(t, server, dup)
		err := server.Register(dup.ID(), "Josh")
		assert.ErrorIs(t, err, ErrDuplicateUsername)
		assert.False(t, server.IsRegistered(dup.ID()))
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

func TestRoomOf(t *testing.T) {
	server := NewServer()
	room := NewRoom("general")
	addRooms(t, server, room)

	assert.Nil(t, server.RoomOf(1))
	connectClients(t, server, NewClient(1))
	assert.Nil(t, server.RoomOf(1))
	require.NoError(t, server.Register(1, "Josh"))
	assert.Nil(t, server.RoomOf(1))
	assert.Empty(t, server.memberships[room.ID()])
}

func TestJoin(t *testing.T) {
	t.Run("client can join room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1)

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		got := server.RoomOf(client.ID())
		assert.Same(t, room, got)
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room.ID()])

		// Rejoining the same room preserves its canonical pointer and membership.
		require.NoError(t, server.Join(client.ID(), room.ID()))
		assert.Same(t, room, server.RoomOf(client.ID()))
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room.ID()])
	})

	t.Run("client can switch rooms", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1)
		roomA := NewRoom("A")
		roomB := NewRoom("B")

		connectAndRegister(t, server, client, "Josh")
		addRooms(t, server, roomA, roomB)

		joinRoom(t, server, roomA, client)
		require.Equal(t, roomA, server.RoomOf(client.ID()))

		joinRoom(t, server, roomB, client)
		require.Same(t, roomB, server.RoomOf(client.ID()))
		assert.Empty(t, server.memberships[roomA.ID()])
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[roomB.ID()])
	})

	t.Run("failed room switch preserves current membership", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1)
		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		assert.ErrorIs(t, server.Join(client.ID(), "unknown"), ErrRoomNotFound)
		assert.Same(t, room, server.RoomOf(client.ID()))
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room.ID()])
		assert.NotContains(t, server.memberships, RoomID("unknown"))
	})

	t.Run("unknown client or room returns an error", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1)

		addRooms(t, server, room)
		connectAndRegister(t, server, client, "Josh")

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
		client := NewClient(1)

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.Leave(client.ID(), room.ID())
		assert.NoError(t, err)
		assert.Nil(t, server.RoomOf(client.ID()))
		assert.Empty(t, server.memberships[room.ID()])

		require.NoError(t, server.Leave(client.ID(), room.ID()))
		assert.Nil(t, server.RoomOf(client.ID()))
		assert.Empty(t, server.memberships[room.ID()])
	})

	t.Run("leaving a different room preserves current membership", func(t *testing.T) {
		server := NewServer()
		roomA := NewRoom("A")
		roomB := NewRoom("B")
		client := NewClient(1)
		other := NewClient(2)
		addRooms(t, server, roomA, roomB)
		registerAndJoin(t, server, roomA, client, "Josh")
		registerAndJoin(t, server, roomB, other, "Friend")

		require.NoError(t, server.Leave(client.ID(), roomB.ID()))
		assert.Same(t, roomA, server.RoomOf(client.ID()))
		assert.Same(t, roomB, server.RoomOf(other.ID()))
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[roomA.ID()])
		assert.Equal(t, map[ClientID]struct{}{other.ID(): {}}, server.memberships[roomB.ID()])
	})

	t.Run("roomless client can leave an existing room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient(1)
		addRooms(t, server, room)
		connectAndRegister(t, server, client, "Josh")

		require.NoError(t, server.Leave(client.ID(), room.ID()))
		assert.Nil(t, server.RoomOf(client.ID()))
		assert.Empty(t, server.memberships[room.ID()])
	})

	t.Run("invalid client or room returns an error", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1)
		room := NewRoom("general")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.Leave(2, room.ID())
		assert.ErrorIs(t, err, ErrClientNotFound)
		err = server.Leave(client.ID(), "unknown")
		assert.ErrorIs(t, err, ErrRoomNotFound)
		assert.Same(t, room, server.RoomOf(client.ID()))
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room.ID()])
	})
}

func TestDisconnect(t *testing.T) {
	t.Run("remove client from registered clients", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1)

		connectAndRegister(t, server, client, "Josh")

		err := server.Disconnect(client.ID())
		require.NoError(t, err)

		exists := server.clientExists(client.ID())
		assert.False(t, exists)

		exists = server.usernameExists("Josh")
		assert.False(t, exists, "username must be removed from registry")
	})

	t.Run("remove membership", func(t *testing.T) {
		server := NewServer()
		client := NewClient(1)
		room := NewRoom("general")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.Disconnect(client.ID())
		require.NoError(t, err)
		assert.Nil(t, server.RoomOf(client.ID()))
		assert.Empty(t, server.memberships[room.ID()])
	})
}

func TestBroadcast(t *testing.T) {
	t.Run("broadcasts to all client in that room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := newFakeClient(1)
		sameRoomClient := newFakeClient(2)
		outsider := newFakeClient(3)

		addRooms(t, server, room)
		connectAndRegister(t, server, outsider, "outsider")
		registerAndJoin(t, server, room, client, "Josh")
		registerAndJoin(t, server, room, sameRoomClient, "Friend")

		content := "sample message content"
		err := server.HandleMessage(client.ID(), content)
		require.NoError(t, err)

		expected := fmt.Sprintf("MSG %s %s %s", room.ID(), "Josh", content)

		assert.Equal(t, expected, client.inbox[0])
		assert.Equal(t, expected, sameRoomClient.inbox[0])
		assert.Empty(t, outsider.inbox)
	})

	t.Run("room switch updates broadcast recipients", func(t *testing.T) {
		server := NewServer()
		roomA := NewRoom("A")
		roomB := NewRoom("B")
		sender := newFakeClient(1)
		formerRoommate := newFakeClient(2)
		newRoommate := newFakeClient(3)
		addRooms(t, server, roomA, roomB)
		registerAndJoin(t, server, roomA, sender, "Josh")
		registerAndJoin(t, server, roomA, formerRoommate, "Old")
		registerAndJoin(t, server, roomB, newRoommate, "New")

		require.NoError(t, server.Join(sender.ID(), roomB.ID()))
		require.NoError(t, server.HandleMessage(sender.ID(), "hello"))
		assert.Equal(t, []string{"MSG B Josh hello"}, sender.inbox)
		assert.Equal(t, []string{"MSG B Josh hello"}, newRoommate.inbox)
		assert.Empty(t, formerRoommate.inbox)

		require.NoError(t, server.HandleMessage(formerRoommate.ID(), "still in A"))
		assert.Equal(t, []string{"MSG A Old still in A"}, formerRoommate.inbox)
		assert.Equal(t, []string{"MSG B Josh hello"}, sender.inbox)
		assert.Equal(t, []string{"MSG B Josh hello"}, newRoommate.inbox)
	})

	t.Run("unknown clients are checked", func(t *testing.T) {
		server := NewServer()
		err := server.HandleMessage(ClientID(123), "sample message")
		assert.ErrorIs(t, err, ErrUnknownClient)
	})

	t.Run("unregistered clients gets declined", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1)

		connectClients(t, server, client)
		err := server.HandleMessage(client.ID(), "nonsense message")
		assert.ErrorIs(t, err, ErrUnregisteredClient)
	})

	t.Run("clients with no room are declined", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1)

		connectAndRegister(t, server, client, "Josh")
		err := server.HandleMessage(client.ID(), "sample message")
		assert.ErrorIs(t, err, ErrNotInRoom)
	})
}

func TestConcurrentJoin(t *testing.T) {
	server := NewServer()
	client := NewClient(1)
	rooms := []*Room{NewRoom("A"), NewRoom("B")}
	addRooms(t, server, rooms...)
	connectAndRegister(t, server, client, "Josh")

	start := make(chan struct{})
	errs := make([]error, len(rooms))
	var wg sync.WaitGroup
	for i, room := range rooms {
		wg.Add(1)
		go func(i int, room *Room) {
			defer wg.Done()
			<-start
			errs[i] = server.Join(client.ID(), room.ID())
		}(i, room)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	currentRoom := server.RoomOf(client.ID())
	require.NotNil(t, currentRoom)
	assert.Same(t, server.rooms[currentRoom.ID()], currentRoom)
	for _, room := range rooms {
		if room == currentRoom {
			assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room.ID()])
		} else {
			assert.Empty(t, server.memberships[room.ID()])
		}
	}
}

// ==== HELPER FUNCTIONS

func connectClients(t testing.TB, s *Server, clients ...Client) {
	t.Helper()
	for _, c := range clients {
		require.NoError(t, s.Connect(c))
	}
}

func connectAndRegister(t testing.TB, s *Server, client Client, username string) {
	t.Helper()
	connectClients(t, s, client)
	require.NoError(t, s.Register(client.ID(), username))
}

func joinRoom(t testing.TB, s *Server, room *Room, clients ...Client) {
	t.Helper()
	for _, c := range clients {
		err := s.Join(c.ID(), room.ID())
		require.NoError(t, err)
	}
}

func registerAndJoin(t testing.TB, s *Server, room *Room, client Client, username string) {
	t.Helper()
	connectAndRegister(t, s, client, username)
	joinRoom(t, s, room, client)
}

func addRooms(t testing.TB, s *Server, rooms ...*Room) {
	t.Helper()
	for _, r := range rooms {
		err := s.AddRoom(r)
		require.NoError(t, err)
	}
}

func newFakeClient(id ClientID) *fakeClient {
	return &fakeClient{
		id:    id,
		inbox: []string{},
	}
}

// === MOCKS / SPIES / FAKES

type fakeClient struct {
	id    ClientID
	inbox []string
}

func (fc *fakeClient) ID() ClientID {
	return fc.id
}

func (fc *fakeClient) Deliver(msg Response) error {
	fc.inbox = append(fc.inbox, msg.String())
	return nil
}

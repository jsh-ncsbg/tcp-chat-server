package chat

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddRoom(t *testing.T) {
	t.Run("room creation", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")

		addRooms(t, server, room)

		assert.True(t, server.RoomExists(room))
	})

	t.Run("reject duplicate room name", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")
		client := newFakeClient(1)

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.AddRoom(room)
		assert.ErrorIs(t, err, ErrDuplicateRoom)
		assertRoomOf(t, server, client.ID(), room)
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room])
	})
}

func TestNewServerStartsWithoutRooms(t *testing.T) {
	server := NewServer()
	assert.Empty(t, server.memberships)
}

func TestRoomOf(t *testing.T) {
	server := NewServer()
	room := RoomID("general")
	addRooms(t, server, room)

	assertNoRoom(t, server, 1)
	connectClients(t, server, newFakeClient(1))
	assertNoRoom(t, server, 1)
	require.NoError(t, server.Register(1, "Josh"))
	assertNoRoom(t, server, 1)
	assert.Empty(t, server.memberships[room])
}

func TestEmptyRoomID(t *testing.T) {
	server := NewServer()
	client := newFakeClient(1)
	addRooms(t, server, "", "general")
	connectAndRegister(t, server, client, "Josh")
	assertNoRoom(t, server, client.ID())

	require.NoError(t, server.Join(client.ID(), ""))
	assertRoomOf(t, server, client.ID(), "")
	require.NoError(t, server.Join(client.ID(), ""))
	require.NoError(t, server.HandleMessage(client.ID(), "hello"))
	assert.Equal(t, []string{"MSG  Josh hello"}, client.inbox)

	require.NoError(t, server.Join(client.ID(), "general"))
	assertRoomOf(t, server, client.ID(), "general")
	assert.Empty(t, server.memberships[""])
	require.NoError(t, server.Join(client.ID(), ""))
	require.NoError(t, server.Leave(client.ID()))
	assertNoRoom(t, server, client.ID())
	assert.Empty(t, server.memberships[""])
	assert.True(t, server.RoomExists(""))
	assert.ErrorIs(t, server.HandleMessage(client.ID(), "roomless"), ErrNotInRoom)
}

func TestJoin(t *testing.T) {
	t.Run("client can join room", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")
		client := newFakeClient(1)

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		assertRoomOf(t, server, client.ID(), room)
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room])

		// Rejoining the same room preserves its membership.
		require.NoError(t, server.Join(client.ID(), room))
		assertRoomOf(t, server, client.ID(), room)
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room])
	})

	t.Run("client can switch rooms", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1)
		roomA := RoomID("A")
		roomB := RoomID("B")

		connectAndRegister(t, server, client, "Josh")
		addRooms(t, server, roomA, roomB)

		joinRoom(t, server, roomA, client)
		assertRoomOf(t, server, client.ID(), roomA)

		joinRoom(t, server, roomB, client)
		assertRoomOf(t, server, client.ID(), roomB)
		assert.Empty(t, server.memberships[roomA])
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[roomB])
	})

	t.Run("failed room switch preserves current membership", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")
		client := newFakeClient(1)
		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		assert.ErrorIs(t, server.Join(client.ID(), "unknown"), ErrRoomNotFound)
		assertRoomOf(t, server, client.ID(), room)
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room])
		assert.NotContains(t, server.memberships, RoomID("unknown"))
	})

	t.Run("unknown client or room returns an error", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")
		client := newFakeClient(1)

		addRooms(t, server, room)
		connectAndRegister(t, server, client, "Josh")

		err := server.Join(2, room)
		assert.ErrorIs(t, err, ErrClientNotFound)
		err = server.Join(client.ID(), "unknown")
		assert.ErrorIs(t, err, ErrRoomNotFound)
	})
}

func TestLeave(t *testing.T) {
	t.Run("client can leave room", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")
		client := newFakeClient(1)

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.Leave(client.ID())
		assert.NoError(t, err)
		assertNoRoom(t, server, client.ID())
		assert.Empty(t, server.memberships[room])
		assert.True(t, server.RoomExists(room))

		require.NoError(t, server.Leave(client.ID()))
		assertNoRoom(t, server, client.ID())
		assert.Empty(t, server.memberships[room])
	})

	t.Run("leaving after a switch preserves other clients", func(t *testing.T) {
		server := NewServer()
		roomA := RoomID("A")
		roomB := RoomID("B")
		client := newFakeClient(1)
		other := newFakeClient(2)
		addRooms(t, server, roomA, roomB)
		registerAndJoin(t, server, roomA, client, "Josh")
		registerAndJoin(t, server, roomB, other, "Friend")
		require.NoError(t, server.Join(client.ID(), roomB))

		require.NoError(t, server.Leave(client.ID()))
		assertNoRoom(t, server, client.ID())
		assertRoomOf(t, server, other.ID(), roomB)
		assert.Empty(t, server.memberships[roomA])
		assert.Equal(t, map[ClientID]struct{}{other.ID(): {}}, server.memberships[roomB])
	})

	t.Run("roomless client can leave without any rooms", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1)
		connectAndRegister(t, server, client, "Josh")

		require.NoError(t, server.Leave(client.ID()))
		assertNoRoom(t, server, client.ID())
		assert.Empty(t, server.memberships)
		assert.True(t, server.IsRegistered(client.ID()))
	})

	t.Run("unknown or unauthenticated client returns an error", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1)
		room := RoomID("general")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.Leave(2)
		assert.ErrorIs(t, err, ErrClientNotFound)
		connectClients(t, server, newFakeClient(2))
		assert.ErrorIs(t, server.Leave(2), ErrUnregisteredClient)
		assertRoomOf(t, server, client.ID(), room)
		assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room])
	})
}

func TestDisconnect(t *testing.T) {
	t.Run("remove client from registered clients", func(t *testing.T) {
		server := NewServer()
		client := newFakeClient(1)

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
		client := newFakeClient(1)
		room := RoomID("general")

		addRooms(t, server, room)
		registerAndJoin(t, server, room, client, "Josh")

		err := server.Disconnect(client.ID())
		require.NoError(t, err)
		assertNoRoom(t, server, client.ID())
		assert.Empty(t, server.memberships[room])
		assert.True(t, server.RoomExists(room))
	})
}

func TestBroadcast(t *testing.T) {
	t.Run("broadcasts to all client in that room", func(t *testing.T) {
		server := NewServer()
		room := RoomID("general")
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

		expected := fmt.Sprintf("MSG %s %s %s", room, "Josh", content)

		assert.Equal(t, expected, client.inbox[0])
		assert.Equal(t, expected, sameRoomClient.inbox[0])
		assert.Empty(t, outsider.inbox)
	})

	t.Run("room switch updates broadcast recipients", func(t *testing.T) {
		server := NewServer()
		roomA := RoomID("A")
		roomB := RoomID("B")
		sender := newFakeClient(1)
		formerRoommate := newFakeClient(2)
		newRoommate := newFakeClient(3)
		addRooms(t, server, roomA, roomB)
		registerAndJoin(t, server, roomA, sender, "Josh")
		registerAndJoin(t, server, roomA, formerRoommate, "Old")
		registerAndJoin(t, server, roomB, newRoommate, "New")

		require.NoError(t, server.Join(sender.ID(), roomB))
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
	client := newFakeClient(1)
	rooms := []RoomID{"A", "B"}
	addRooms(t, server, rooms...)
	connectAndRegister(t, server, client, "Josh")

	start := make(chan struct{})
	errs := make([]error, len(rooms))
	var wg sync.WaitGroup
	for i, room := range rooms {
		wg.Add(1)
		go func(i int, room RoomID) {
			defer wg.Done()
			<-start
			errs[i] = server.Join(client.ID(), room)
		}(i, room)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	currentRoom, inRoom := server.RoomOf(client.ID())
	require.True(t, inRoom)
	require.Contains(t, rooms, currentRoom)
	for _, room := range rooms {
		if room == currentRoom {
			assert.Equal(t, map[ClientID]struct{}{client.ID(): {}}, server.memberships[room])
		} else {
			assert.Empty(t, server.memberships[room])
		}
	}
}

// ==== HELPER FUNCTIONS

func assertRoomOf(t testing.TB, s *Server, clientID ClientID, expected RoomID) {
	t.Helper()
	roomID, inRoom := s.RoomOf(clientID)
	require.True(t, inRoom)
	assert.Equal(t, expected, roomID)
}

func assertNoRoom(t testing.TB, s *Server, clientID ClientID) {
	t.Helper()
	roomID, inRoom := s.RoomOf(clientID)
	assert.False(t, inRoom)
	assert.Equal(t, RoomID(""), roomID)
}

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

func joinRoom(t testing.TB, s *Server, room RoomID, clients ...Client) {
	t.Helper()
	for _, c := range clients {
		err := s.Join(c.ID(), room)
		require.NoError(t, err)
	}
}

func registerAndJoin(t testing.TB, s *Server, room RoomID, client Client, username string) {
	t.Helper()
	connectAndRegister(t, s, client, username)
	joinRoom(t, s, room, client)
}

func addRooms(t testing.TB, s *Server, rooms ...RoomID) {
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

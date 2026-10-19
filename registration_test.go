package main

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnect(t *testing.T) {
	server := NewServer()
	client := newFakeClient(1)

	connectClients(t, server, client)

	assert.False(t, server.IsRegistered(client.ID()))
	assert.True(t, server.clientExists(client.ID()))
	assert.Empty(t, server.usernames)

	assert.ErrorIs(t, server.Connect(newFakeClient(client.ID())), ErrDuplicateClientID)
	assert.Same(t, client, server.clients[client.ID()].connection)
	assert.False(t, server.IsRegistered(client.ID()))

	require.NoError(t, server.Register(client.ID(), "Josh"))
	assert.True(t, server.IsRegistered(client.ID()))
}

func TestRegister(t *testing.T) {
	t.Run("connection is required before username validation", func(t *testing.T) {
		server := NewServer()

		assert.ErrorIs(t, server.Register(1, "Josh"), ErrUnknownClient)
		assert.ErrorIs(t, server.Register(1, ""), ErrUnknownClient)
		assert.False(t, server.IsRegistered(1))
		assert.Empty(t, server.clients)
		assert.Empty(t, server.usernames)

		connectAndRegister(t, server, newFakeClient(1), "Josh")
		assert.True(t, server.IsRegistered(1))
	})

	t.Run("empty username does not register or reserve a name", func(t *testing.T) {
		server := NewServer()
		connectClients(t, server, newFakeClient(1))

		assert.ErrorIs(t, server.Register(1, ""), ErrInvalidUsername)
		assert.False(t, server.IsRegistered(1))
		assert.Empty(t, server.usernames)

		require.NoError(t, server.Register(1, "Josh"))
		assert.True(t, server.IsRegistered(1))
	})

	t.Run("duplicate username can be retried with an available name", func(t *testing.T) {
		server := NewServer()
		connectAndRegister(t, server, newFakeClient(1), "Josh")
		connectClients(t, server, newFakeClient(2))

		assert.ErrorIs(t, server.Register(2, "Josh"), ErrDuplicateUsername)
		assert.False(t, server.IsRegistered(2))
		assert.Equal(t, ClientID(1), server.usernames["Josh"])

		require.NoError(t, server.Register(2, "James"))
		assert.True(t, server.IsRegistered(2))
		assert.Equal(t, ClientID(2), server.usernames["James"])
	})

	t.Run("registered client cannot register again", func(t *testing.T) {
		server := NewServer()
		connectAndRegister(t, server, newFakeClient(1), "Josh")
		connectAndRegister(t, server, newFakeClient(2), "Taken")

		for _, username := range []string{"Josh", "James", "", "Taken"} {
			assert.ErrorIs(t, server.Register(1, username), ErrAlreadyRegistered)
		}

		assert.True(t, server.IsRegistered(1))
		assert.Equal(t, "Josh", server.clients[1].username)
		assert.Equal(t, map[string]ClientID{"Josh": 1, "Taken": 2}, server.usernames)
		connectAndRegister(t, server, newFakeClient(3), "James")
	})

	t.Run("names are exact and only empty names are rejected", func(t *testing.T) {
		server := NewServer()
		for i, username := range []string{"Josh", "josh", " Josh ", " "} {
			id := ClientID(i + 1)
			connectAndRegister(t, server, newFakeClient(id), username)
			assert.True(t, server.IsRegistered(id))
			assert.Equal(t, username, server.clients[id].username)
		}
		assert.Len(t, server.usernames, 4)
	})
}

func TestUnauthenticatedRoomOperations(t *testing.T) {
	for _, operation := range []string{"join", "leave"} {
		t.Run(operation, func(t *testing.T) {
			server := NewServer()
			room := NewRoom("general")
			addRooms(t, server, room)
			connectClients(t, server, newFakeClient(1))
			var roomOperation func(ClientID, RoomID) error
			if operation == "join" {
				roomOperation = server.Join
			} else {
				roomOperation = server.Leave
			}

			for _, roomID := range []RoomID{room.ID(), "missing"} {
				assert.ErrorIs(t, roomOperation(1, roomID), ErrUnregisteredClient)
				assert.ErrorIs(t, roomOperation(2, roomID), ErrClientNotFound)
			}
			assert.Nil(t, server.RoomOf(1))
			assert.Empty(t, server.memberships[room.ID()])
		})
	}
}

func TestDisconnectLifecycle(t *testing.T) {
	t.Run("disconnect before registration allows ID reuse", func(t *testing.T) {
		server := NewServer()
		connectAndRegister(t, server, newFakeClient(2), "Other")
		connectClients(t, server, newFakeClient(1))

		require.NoError(t, server.Disconnect(1))
		assert.False(t, server.clientExists(1))
		assert.False(t, server.IsRegistered(1))
		assert.ErrorIs(t, server.Register(1, "Josh"), ErrUnknownClient)
		assert.ErrorIs(t, server.Disconnect(1), ErrClientNotFound)
		assert.True(t, server.IsRegistered(2))
		assert.Equal(t, map[string]ClientID{"Other": 2}, server.usernames)

		connectAndRegister(t, server, newFakeClient(1), "Josh")
		assert.True(t, server.IsRegistered(1))
	})

	t.Run("disconnect releases username and room membership", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		oldClient := newFakeClient(1)
		addRooms(t, server, room)
		registerAndJoin(t, server, room, oldClient, "Josh")

		require.NoError(t, server.Disconnect(oldClient.ID()))
		assert.False(t, server.IsRegistered(oldClient.ID()))
		assert.Nil(t, server.RoomOf(oldClient.ID()))
		assert.Empty(t, server.memberships[room.ID()])
		assert.False(t, server.usernameExists("Josh"))

		replacement := newFakeClient(1)
		connectClients(t, server, replacement)
		assert.False(t, server.IsRegistered(replacement.ID()))
		assert.Nil(t, server.RoomOf(replacement.ID()))
		require.NoError(t, server.Register(replacement.ID(), "James"))
		registerAndJoin(t, server, room, newFakeClient(2), "Josh")
		require.NoError(t, server.Join(replacement.ID(), room.ID()))
		require.NoError(t, server.HandleMessage(replacement.ID(), "hello"))
		assert.Empty(t, oldClient.inbox)
		assert.Equal(t, []string{"MSG general James hello"}, replacement.inbox)
	})
}

func TestConcurrentRegister(t *testing.T) {
	for _, sameConnection := range []bool{false, true} {
		name := "two connections claim one username"
		if sameConnection {
			name = "one connection claims two usernames"
		}
		t.Run(name, func(t *testing.T) {
			server := NewServer()
			connectClients(t, server, newFakeClient(1), newFakeClient(2))
			ids := []ClientID{1, 2}
			usernames := []string{"Josh", "Josh"}
			loserError := ErrDuplicateUsername
			if sameConnection {
				ids[1] = ids[0]
				usernames[1] = "James"
				loserError = ErrAlreadyRegistered
			}

			start := make(chan struct{})
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i := range ids {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					errs[i] = server.Register(ids[i], usernames[i])
				}(i)
			}
			close(start)
			wg.Wait()

			winner := -1
			for i, err := range errs {
				if err == nil {
					require.Equal(t, -1, winner, "only one registration may succeed")
					winner = i
				} else {
					assert.ErrorIs(t, err, loserError)
				}
			}
			require.NotEqual(t, -1, winner, "one registration must succeed")
			assert.True(t, server.IsRegistered(ids[winner]))
			assert.Equal(t, usernames[winner], server.clients[ids[winner]].username)
			assert.Equal(t, map[string]ClientID{usernames[winner]: ids[winner]}, server.usernames)
			if !sameConnection {
				assert.False(t, server.IsRegistered(ids[1-winner]))
			}
		})
	}
}

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("client can register", func(t *testing.T) {
		server := NewServer()
		client := NewClient("Josh")

		err := server.Register(client)

		require.NoError(t, err)
		assert.True(t, server.IsRegistered(client.Username()))
		assert.False(t, server.IsRegistered("random"))
	})

	t.Run("duplicate username is rejected", func(t *testing.T) {
		server := NewServer()
		client := NewClient("Josh")
		dup := NewClient("Josh")

		err := server.Register(client)
		require.NoError(t, err)

		err = server.Register(dup)
		assert.ErrorIs(t, err, ErrDuplicateUsername)
	})

	t.Run("room creation", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")

		err := server.AddRoom(room)
		require.NoError(t, err)
		assert.True(t, server.RoomExists(room.ID()))
	})

	t.Run("reject duplicate room name", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		dup := NewRoom("general")

		err := server.AddRoom(room)
		require.NoError(t, err)

		err = server.AddRoom(dup)
		assert.ErrorIs(t, err, ErrDuplicateRoom)
	})
}

func TestJoin(t *testing.T) {
	t.Run("client can join room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient("Josh")

		err := server.AddRoom(room)
		require.NoError(t, err)

		err = server.Register(client)
		require.NoError(t, err)

		err = server.Join(client.Username(), room.ID())
		require.NoError(t, err)

		got := server.RoomOf(client.Username())
		assert.Equal(t, room, got)
	})

	t.Run("client can switch rooms", func(t *testing.T) {
		server := NewServer()

		client := NewClient("Josh")

		err := server.Register(client)
		require.NoError(t, err)

		rooms := []*Room{}
		for _, roomID := range []string{"A", "B", "C"} {
			curr := NewRoom(RoomID(roomID))
			rooms = append(rooms, curr)

			err := server.AddRoom(curr)
			require.NoError(t, err)
		}

		var oldRoom *Room
		for _, room := range rooms {
			err := server.Join(client.Username(), room.ID())
			require.NoError(t, err)
			require.Equal(t, room, server.RoomOf(client.Username()))

			// check if membership with old room is gone
			if oldRoom != nil {
				assert.False(t, server.isMember(client.Username(), oldRoom.ID()))
			}

			oldRoom = room
		}
	})

	t.Run("unknown client or room returns an error", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient("Josh")

		err := server.AddRoom(room)
		require.NoError(t, err)

		err = server.Register(client)
		require.NoError(t, err)

		err = server.Join("unknown", room.ID())
		assert.ErrorIs(t, err, ErrClientNotFound)
		err = server.Join(client.Username(), "unknown")
		assert.ErrorIs(t, err, ErrRoomNotFound)
	})
}

func TestLeave(t *testing.T) {
	t.Run("client can leave room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient("Josh")

		err := server.AddRoom(room)
		require.NoError(t, err)

		err = server.Register(client)
		require.NoError(t, err)

		err = server.Join(client.Username(), room.ID())
		require.NoError(t, err)

		err = server.Leave(client.Username(), room.ID())
		assert.NoError(t, err)
		assert.Nil(t, server.RoomOf(client.Username()))
	})

	t.Run("invalid client or room returns an error", func(t *testing.T) {
		server := NewServer()
		client := NewClient("Josh")
		room := NewRoom("general")

		err := server.AddRoom(room)
		require.NoError(t, err)

		err = server.Register(client)
		require.NoError(t, err)

		err = server.Join(client.Username(), room.ID())
		require.NoError(t, err)

		err = server.Leave("unknown", room.ID())
		assert.ErrorIs(t, err, ErrClientNotFound)
		err = server.Leave(client.Username(), "unknown")
		assert.ErrorIs(t, err, ErrRoomNotFound)
	})
}

func TestDisconnect(t *testing.T) {
	t.Run("remove client from registered clients", func(t *testing.T) {
		server := NewServer()
		client := NewClient("Josh")

		err := server.Register(client)
		require.NoError(t, err)

		err = server.Disconnect(client.Username())
		require.NoError(t, err)

		assert.False(t, server.IsRegistered(client.Username()))
	})

	t.Run("remove membership", func(t *testing.T) {
		server := NewServer()
		client := NewClient("Josh")
		room := NewRoom("general")

		err := server.AddRoom(room)
		require.NoError(t, err)

		err = server.Register(client)
		require.NoError(t, err)

		// membership created
		err = server.Join(client.Username(), room.ID())
		require.NoError(t, err)

		err = server.Disconnect(client.Username())
		require.NoError(t, err)
		assert.Nil(t, server.RoomOf(client.Username()))
	})
}

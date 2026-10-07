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

		_ = server.Register(client)
		err := server.Register(dup)

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

		_ = server.AddRoom(room)
		err := server.AddRoom(dup)
		assert.ErrorIs(t, err, ErrDuplicateRoom)
	})
}

func TestJoin(t *testing.T) {
	t.Run("client can join room", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient("Josh")

		_ = server.AddRoom(room)
		_ = server.Register(client)

		err := server.Join(client.Username(), room.ID())
		require.NoError(t, err)

		got := server.RoomOf(client.Username())
		assert.Equal(t, room, got)
	})

	t.Run("invalid client returns an error", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient("Josh")

		_ = server.AddRoom(room)
		// _ = server.Register(client)

		err := server.Join(client.Username(), room.ID())
		assert.ErrorIs(t, err, ErrClientNotFound)
	})

	t.Run("invalid room returns an error", func(t *testing.T) {
		server := NewServer()
		room := NewRoom("general")
		client := NewClient("Josh")

		// _ = server.AddRoom(room)
		_ = server.Register(client)

		err := server.Join(client.Username(), room.ID())
		assert.ErrorIs(t, err, ErrRoomNotFound)
	})
}

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultServer(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	ids := []RoomID{"general", "breakout-1", "breakout-2", "breakout-3", "breakout-4", "breakout-5"}
	assert.Len(t, server.rooms, len(ids))
	assert.Len(t, server.memberships, len(ids))
	for _, id := range ids {
		require.True(t, server.RoomExists(id))
		assert.Equal(t, id, server.rooms[id].ID())
		assert.Empty(t, server.memberships[id])
	}
	assert.Empty(t, server.clients)
	assert.Empty(t, server.usernames)
}

func TestNewServerStartsWithoutRooms(t *testing.T) {
	server := NewServer()
	assert.Empty(t, server.rooms)
	assert.Empty(t, server.memberships)
}

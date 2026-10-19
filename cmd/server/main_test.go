package main

import (
	"testing"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultServer(t *testing.T) {
	server, err := newDefaultServer()
	require.NoError(t, err)
	ids := []chat.RoomID{"general", "breakout-1", "breakout-2", "breakout-3", "breakout-4", "breakout-5"}
	for _, id := range ids {
		require.True(t, server.RoomExists(id))
		assert.ErrorIs(t, server.AddRoom(id), chat.ErrDuplicateRoom)
	}
	assert.False(t, server.RoomExists("breakout-0"))
	assert.False(t, server.RoomExists("breakout-6"))
}

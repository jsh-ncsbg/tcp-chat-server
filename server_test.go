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
}

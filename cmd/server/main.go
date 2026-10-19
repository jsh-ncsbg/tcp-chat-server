package main

import (
	"fmt"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
)

func main() {
	fmt.Println("Hello World!")
}

func newDefaultServer() (*chat.Server, error) {
	server := chat.NewServer()
	for _, id := range []chat.RoomID{"general", "breakout-1", "breakout-2", "breakout-3", "breakout-4", "breakout-5"} {
		if err := server.AddRoom(chat.NewRoom(id)); err != nil {
			return nil, fmt.Errorf("default room %q: %w", id, err)
		}
	}
	return server, nil
}

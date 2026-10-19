package main

import (
	"fmt"
	"log"
	"net"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
	"github.com/jsh-ncsbg/tcp-chat-server/internal/tcp"
)

func main() {
	server, err := newDefaultServer()
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s", listener.Addr())
	if err := tcp.Serve(server, listener); err != nil {
		log.Fatal(err)
	}
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

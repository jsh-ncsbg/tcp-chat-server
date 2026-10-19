package main

import "fmt"

func main() {
	fmt.Println("Hello World!")
}

func newDefaultServer() (*Server, error) {
	server := NewServer()
	for _, id := range []RoomID{"general", "breakout-1", "breakout-2", "breakout-3", "breakout-4", "breakout-5"} {
		if err := server.AddRoom(NewRoom(id)); err != nil {
			return nil, fmt.Errorf("default room %q: %w", id, err)
		}
	}
	return server, nil
}

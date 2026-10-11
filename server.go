package main

import (
	"errors"
	"fmt"
	"log"
)

var (
	ErrDuplicateUsername = errors.New("username already exists")
	ErrDuplicateRoom     = errors.New("room already exists")

	ErrClientNotFound = errors.New("client not found")
	ErrRoomNotFound   = errors.New("room not found")

	ErrUnregisteredClient = errors.New("client not registered")
)

type Message struct {
	From    string
	Content string
}

type Server struct {
	clients   map[ClientID]Client
	rooms     map[RoomID]*Room
	usernames map[string]ClientID

	memberships map[RoomID]map[ClientID]struct{}
}

func NewServer() *Server {
	return &Server{
		clients:     make(map[ClientID]Client),
		rooms:       make(map[RoomID]*Room),
		usernames:   make(map[string]ClientID),
		memberships: make(map[RoomID]map[ClientID]struct{}),
	}
}

func (s *Server) Register(client Client) error {
	if s.usernameExists(client.Username()) {
		return ErrDuplicateUsername
	}
	if client.Username() != "" {
		s.usernames[client.Username()] = client.ID()
	}
	s.clients[client.ID()] = client
	return nil
}

func (s *Server) IsRegistered(id ClientID) bool {
	c, ok := s.clients[id]
	return ok && c.Username() != ""
}

func (s *Server) AddRoom(room *Room) error {
	_, ok := s.rooms[room.ID()]
	if ok {
		return ErrDuplicateRoom
	}

	// Register room
	s.rooms[room.ID()] = room

	// Prepare membership map
	s.memberships[room.ID()] = make(map[ClientID]struct{})

	return nil
}

func (s *Server) RoomExists(id RoomID) bool {
	_, ok := s.rooms[id]
	return ok
}

func (s *Server) Join(clientID ClientID, roomID RoomID) error {
	if !s.IsRegistered(clientID) {
		return fmt.Errorf("join: %w", ErrClientNotFound)
	}

	if !s.RoomExists(roomID) {
		return fmt.Errorf("join: %w", ErrRoomNotFound)
	}

	if connectedRoom := s.RoomOf(clientID); connectedRoom != nil {
		s.leave(clientID, connectedRoom.ID())
	}

	s.memberships[roomID][clientID] = struct{}{}
	return nil
}

func (s *Server) Leave(clientID ClientID, roomID RoomID) error {
	if !s.IsRegistered(clientID) {
		return fmt.Errorf("leave: %w", ErrClientNotFound)
	}

	if !s.RoomExists(roomID) {
		return fmt.Errorf("leave: %w", ErrRoomNotFound)
	}

	s.leave(clientID, roomID)
	return nil
}

func (s *Server) leave(clientID ClientID, roomID RoomID) {
	// assumes caller has guards
	delete(s.memberships[roomID], clientID)
}

func (s *Server) isMember(clientID ClientID, roomID RoomID) bool {
	// assume caller has guards
	_, ok := s.memberships[roomID][clientID]
	return ok
}

func (s *Server) usernameExists(username string) bool {
	_, ok := s.usernames[username]
	return ok
}

func (s *Server) RoomOf(clientID ClientID) *Room {
	for _, room := range s.rooms {
		if s.isMember(clientID, room.ID()) {
			return room
		}
	}
	return nil
}

func (s *Server) Disconnect(clientID ClientID) error {
	if !s.IsRegistered(clientID) {
		return fmt.Errorf("disconnect: %w", ErrClientNotFound)
	}

	connectedRoom := s.RoomOf(clientID)
	if connectedRoom != nil {
		s.leave(clientID, connectedRoom.ID())
	}

	delete(s.clients, clientID)

	return nil
}

func (s *Server) membersOf(roomID RoomID) []Client {
	clientIDs, ok := s.memberships[roomID]
	if !ok {
		return nil
	}
	result := []Client{}
	for id := range clientIDs {
		c, _ := s.clients[id]
		result = append(result, c)
	}
	return result
}

func (s *Server) HandleMessage(clientID ClientID, content string) {
	sender := s.clients[clientID]
	if !s.IsRegistered(sender.ID()) {
		err := sender.Deliver(NewErrResponse("UNAUTHENTICATED", "client is not registered"))
		if err != nil {
			log.Printf("client deliver: %v\n", err)
		}
		return
	}

	room := s.RoomOf(clientID)
	if room == nil {
		err := sender.Deliver(NewErrResponse(
			"ROOM_NOT_FOUND",
			"client not in a room",
		))
		if err != nil {
			log.Printf("client deliver: %v\n", err)
		}
		return
	}

	clients := s.membersOf(room.ID())

	for _, c := range clients {
		err := c.Deliver(
			NewMsgResponse(
				string(room.ID()),
				sender.Username(),
				content,
			),
		)
		if err != nil {
			log.Printf("client deliver: %v\n", err)
		}
	}
}

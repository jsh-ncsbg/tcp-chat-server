package main

import (
	"errors"
	"fmt"
)

var (
	ErrDuplicateUsername = errors.New("username already exists")
	ErrDuplicateRoom     = errors.New("room already exists")

	ErrClientNotFound = errors.New("client not found")
	ErrRoomNotFound   = errors.New("room not found")
)

type Server struct {
	clients map[string]Client
	rooms   map[RoomID]*Room

	memberships map[RoomID]map[string]struct{}
}

func NewServer() *Server {
	return &Server{
		clients:     make(map[string]Client),
		rooms:       make(map[RoomID]*Room),
		memberships: make(map[RoomID]map[string]struct{}),
	}
}

func (s *Server) Register(client Client) error {
	_, ok := s.clients[client.Username()]
	if ok {
		return ErrDuplicateUsername
	}
	s.clients[client.Username()] = client
	return nil
}

func (s *Server) IsRegistered(username string) bool {
	_, ok := s.clients[username]
	return ok
}

func (s *Server) AddRoom(room *Room) error {
	_, ok := s.rooms[room.ID()]
	if ok {
		return ErrDuplicateRoom
	}

	// Register room
	s.rooms[room.ID()] = room

	// Prepare membership map
	s.memberships[room.ID()] = make(map[string]struct{})

	return nil
}

func (s *Server) RoomExists(id RoomID) bool {
	_, ok := s.rooms[id]
	return ok
}

func (s *Server) Join(username string, roomID RoomID) error {
	if !s.IsRegistered(username) {
		return fmt.Errorf("join: %w", ErrClientNotFound)
	}

	if !s.RoomExists(roomID) {
		return fmt.Errorf("join: %w", ErrRoomNotFound)
	}

	if connectedRoom := s.RoomOf(username); connectedRoom != nil {
		s.leave(username, connectedRoom.ID())
	}

	s.memberships[roomID][username] = struct{}{}
	return nil
}

func (s *Server) Leave(username string, roomID RoomID) error {
	if !s.IsRegistered(username) {
		return fmt.Errorf("leave: %w", ErrClientNotFound)
	}

	if !s.RoomExists(roomID) {
		return fmt.Errorf("leave: %w", ErrRoomNotFound)
	}

	s.leave(username, roomID)
	return nil
}

func (s *Server) leave(username string, roomID RoomID) {
	// assumes caller has guards
	delete(s.memberships[roomID], username)
}

func (s *Server) isMember(username string, roomID RoomID) bool {
	// assume caller has guards
	_, ok := s.memberships[roomID][username]
	return ok
}

func (s *Server) RoomOf(username string) *Room {
	for _, room := range s.rooms {
		if s.isMember(username, room.ID()) {
			return room
		}
	}
	return nil
}

func (s *Server) Disconnect(username string) error {
	if !s.IsRegistered(username) {
		return fmt.Errorf("disconnect: %w", ErrClientNotFound)
	}

	connectedRoom := s.RoomOf(username)
	if connectedRoom != nil {
		s.leave(username, connectedRoom.ID())
	}

	delete(s.clients, username)

	return nil
}

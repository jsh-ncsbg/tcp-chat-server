package main

import (
	"errors"
)

var ErrDuplicateUsername = errors.New("duplicate username")

type Server struct {
	clients map[string]*Client
	rooms   map[string]*Room
}

func NewServer() *Server {
	return &Server{
		clients: make(map[string]*Client),
		rooms:   make(map[string]*Room),
	}
}

func (s *Server) Register(client *Client) error {
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
	s.rooms[room.Name()] = room
	return nil
}

func (s *Server) RoomExists(name string) bool {
	_, ok := s.rooms[name]
	return ok
}

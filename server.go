package main

import (
	"errors"
)

var ErrDuplicateUsername = errors.New("username already exists")
var ErrDuplicateRoom = errors.New("room already exists")

type Server struct {
	clients map[string]*Client
	rooms   map[RoomID]*Room
}

func NewServer() *Server {
	return &Server{
		clients: make(map[string]*Client),
		rooms:   make(map[RoomID]*Room),
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
	_, ok := s.rooms[room.ID()]
	if ok {
		return ErrDuplicateRoom
	}
	s.rooms[room.ID()] = room
	return nil
}

func (s *Server) RoomExists(id RoomID) bool {
	_, ok := s.rooms[id]
	return ok
}

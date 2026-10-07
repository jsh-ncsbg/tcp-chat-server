package main

import "errors"

var ErrDuplicateUsername = errors.New("duplicate username")

type Server struct {
	clients map[string]*Client
}

func NewServer() *Server {
	return &Server{
		clients: make(map[string]*Client),
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

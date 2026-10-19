package chat

import (
	"errors"
	"fmt"
	"log"
	"sync"
)

var (
	ErrDuplicateUsername = errors.New("username already exists")
	ErrDuplicateRoom     = errors.New("room already exists")

	ErrClientNotFound = errors.New("client not found")
	ErrRoomNotFound   = errors.New("room not found")

	ErrUnregisteredClient = errors.New("client not registered")
	ErrAlreadyRegistered  = errors.New("client already registered")
	ErrInvalidUsername    = errors.New("username must not be empty")
	ErrNotInRoom          = errors.New("client not in a room")

	// internal server errors
	ErrUnknownClient     = errors.New("unknown client")
	ErrDuplicateClientID = errors.New("duplicate client id")
)

type Message struct {
	From    string
	Content string
}

type RoomID string

type clientState struct {
	connection Client
	username   string
	roomID     RoomID
	inRoom     bool
}

type Server struct {
	mu sync.RWMutex

	clients   map[ClientID]*clientState
	usernames map[string]ClientID

	memberships map[RoomID]map[ClientID]struct{}
}

func NewServer() *Server {
	return &Server{
		clients:     make(map[ClientID]*clientState),
		usernames:   make(map[string]ClientID),
		memberships: make(map[RoomID]map[ClientID]struct{}),
	}
}

func (s *Server) Connect(client Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.clientExists(client.ID()) {
		return fmt.Errorf("connect: %w", ErrDuplicateClientID)
	}

	s.clients[client.ID()] = &clientState{connection: client}
	return nil
}

func (s *Server) Register(clientID ClientID, username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	client, ok := s.clients[clientID]
	if !ok {
		return fmt.Errorf("register: %w", ErrUnknownClient)
	}
	if client.username != "" {
		return fmt.Errorf("register: %w", ErrAlreadyRegistered)
	}
	if username == "" {
		return fmt.Errorf("register: %w", ErrInvalidUsername)
	}
	if s.usernameExists(username) {
		return fmt.Errorf("register: %w", ErrDuplicateUsername)
	}

	client.username = username
	s.usernames[username] = clientID
	return nil
}

func (s *Server) IsRegistered(id ClientID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.isRegisteredLocked(id)
}

func (s *Server) isRegisteredLocked(id ClientID) bool {
	c, ok := s.clients[id]
	return ok && c.username != ""
}

func (s *Server) AddRoom(roomID RoomID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.roomExistsLocked(roomID) {
		return ErrDuplicateRoom
	}

	s.memberships[roomID] = make(map[ClientID]struct{})

	return nil
}

func (s *Server) RoomExists(id RoomID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.roomExistsLocked(id)
}

func (s *Server) roomExistsLocked(id RoomID) bool {
	_, ok := s.memberships[id]
	return ok
}

func (s *Server) Join(clientID ClientID, roomID RoomID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.clientExists(clientID) {
		return fmt.Errorf("join: %w", ErrClientNotFound)
	}
	if !s.isRegisteredLocked(clientID) {
		return fmt.Errorf("join: %w", ErrUnregisteredClient)
	}

	if !s.roomExistsLocked(roomID) {
		return fmt.Errorf("join: %w", ErrRoomNotFound)
	}

	client := s.clients[clientID]
	if client.inRoom {
		if client.roomID == roomID {
			return nil
		}
		s.leaveLocked(clientID)
	}

	s.memberships[roomID][clientID] = struct{}{}
	client.roomID = roomID
	client.inRoom = true
	return nil
}

func (s *Server) Leave(clientID ClientID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.clientExists(clientID) {
		return fmt.Errorf("leave: %w", ErrClientNotFound)
	}
	if !s.isRegisteredLocked(clientID) {
		return fmt.Errorf("leave: %w", ErrUnregisteredClient)
	}

	s.leaveLocked(clientID)
	return nil
}

// leaveLocked requires the write lock and an existing client.
func (s *Server) leaveLocked(clientID ClientID) {
	client := s.clients[clientID]
	if !client.inRoom {
		return
	}

	delete(s.memberships[client.roomID], clientID)
	client.roomID = ""
	client.inRoom = false
}

func (s *Server) usernameExists(username string) bool {
	_, ok := s.usernames[username]
	return ok
}

func (s *Server) clientExists(clientID ClientID) bool {
	_, ok := s.clients[clientID]
	return ok
}

func (s *Server) RoomOf(clientID ClientID) (RoomID, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.roomOfLocked(clientID)
}

func (s *Server) roomOfLocked(clientID ClientID) (RoomID, bool) {
	client, ok := s.clients[clientID]
	if !ok {
		return "", false
	}
	return client.roomID, client.inRoom
}

func (s *Server) Disconnect(clientID ClientID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	client, exists := s.clients[clientID]
	if !exists {
		return ErrClientNotFound
	}

	s.leaveLocked(clientID)

	delete(s.clients, clientID)
	if client.username != "" {
		delete(s.usernames, client.username)
	}

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
		result = append(result, c.connection)
	}
	return result
}

func (s *Server) HandleMessage(clientID ClientID, content string) error {
	recipients, response, err := s.prepareMessage(clientID, content)
	if err != nil {
		return fmt.Errorf("client deliver: %w", err)
	}

	for _, c := range recipients {
		if err := c.Deliver(*response); err != nil {
			log.Printf("client deliver: %v\n", err)
		}
	}

	return nil
}

func (s *Server) prepareMessage(clientID ClientID, content string) ([]Client, *Response, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sender, ok := s.clients[clientID]
	if !ok {
		return nil, nil, ErrUnknownClient
	}

	if !s.isRegisteredLocked(clientID) {
		return nil, nil, ErrUnregisteredClient
	}

	roomID, inRoom := s.roomOfLocked(clientID)
	if !inRoom {
		return nil, nil, ErrNotInRoom
	}

	clients := s.membersOf(roomID)
	msg := NewMsgResponse(
		string(roomID),
		sender.username,
		content,
	)

	return clients, &msg, nil
}

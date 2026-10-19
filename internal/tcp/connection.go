package tcp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
)

var (
	errInvalidCommand   = errors.New("invalid command")
	errInvalidArguments = errors.New("invalid arguments")
)

func HandleConnection(server *chat.Server, id chat.ClientID, conn net.Conn) (result error) {
	client := NewClient(id, conn)
	if err := server.Connect(client); err != nil {
		return errors.Join(err, client.Close())
	}

	writerDone := make(chan error, 1)
	go func() { writerDone <- client.WriteLoop() }()
	defer func() {
		select {
		case <-client.done:
			if errors.Is(result, net.ErrClosed) || errors.Is(result, io.ErrClosedPipe) {
				result = nil
			}
		default:
		}
		closeErr := client.Close()
		writerErr := <-writerDone
		disconnectErr := server.Disconnect(id)
		result = errors.Join(result, writerErr, closeErr, disconnectErr)
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if response := dispatchCommand(server, id, scanner.Text()); response != nil {
			if err := client.Deliver(*response); err != nil {
				return fmt.Errorf("connection response: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("connection read: %w", err)
	}
	return nil
}

func parseCommand(line string) (string, string, error) {
	line = strings.TrimLeft(line, " \t")
	name, argument := line, ""
	if separator := strings.IndexAny(line, " \t"); separator >= 0 {
		name = line[:separator]
		argument = strings.TrimLeft(line[separator:], " \t")
	}

	switch name {
	case "REGISTER", "JOIN":
		arguments := strings.Fields(argument)
		if len(arguments) != 1 {
			return "", "", fmt.Errorf("%w: %s requires one name", errInvalidArguments, name)
		}
		return name, arguments[0], nil
	case "LEAVE":
		if strings.TrimSpace(argument) != "" {
			return "", "", fmt.Errorf("%w: LEAVE takes no arguments", errInvalidArguments)
		}
		return name, "", nil
	case "MSG":
		if strings.TrimSpace(argument) == "" {
			return "", "", fmt.Errorf("%w: MSG requires message text", errInvalidArguments)
		}
		return name, argument, nil
	default:
		return "", "", errInvalidCommand
	}
}

func dispatchCommand(server *chat.Server, id chat.ClientID, line string) *chat.Response {
	name, argument, err := parseCommand(line)
	if err == nil {
		switch name {
		case "REGISTER":
			err = server.Register(id, argument)
		case "JOIN":
			err = server.Join(id, chat.RoomID(argument))
		case "LEAVE":
			err = server.Leave(id)
		case "MSG":
			err = server.HandleMessage(id, argument)
		}
	}
	if err != nil {
		response := commandErrorResponse(err)
		return &response
	}
	if name == "MSG" {
		return nil
	}
	response := chat.NewInfoResponse("OK " + name)
	return &response
}

func commandErrorResponse(err error) chat.Response {
	var code string
	switch {
	case errors.Is(err, errInvalidCommand):
		code = "INVALID_COMMAND"
	case errors.Is(err, errInvalidArguments), errors.Is(err, chat.ErrInvalidUsername):
		code = "INVALID_ARGUMENTS"
	case errors.Is(err, chat.ErrDuplicateUsername):
		code = "USERNAME_TAKEN"
	case errors.Is(err, chat.ErrAlreadyRegistered):
		code = "ALREADY_REGISTERED"
	case errors.Is(err, chat.ErrUnregisteredClient):
		code = "UNREGISTERED"
	case errors.Is(err, chat.ErrRoomNotFound):
		code = "ROOM_NOT_FOUND"
	case errors.Is(err, chat.ErrNotInRoom):
		code = "NOT_IN_ROOM"
	default:
		return chat.NewErrResponse("INTERNAL", "internal server error")
	}
	return chat.NewErrResponse(code, err.Error())
}

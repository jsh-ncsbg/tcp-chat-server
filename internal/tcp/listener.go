package tcp

import (
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
)

func Serve(server *chat.Server, listener net.Listener) error {
	defer listener.Close()

	var id chat.ClientID
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}

		id++
		go func(id chat.ClientID, conn net.Conn) {
			if err := HandleConnection(server, id, conn); err != nil {
				log.Printf("client %d: %v", id, err)
			}
		}(id, conn)
	}
}

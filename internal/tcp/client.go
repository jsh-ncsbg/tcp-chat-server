package tcp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/jsh-ncsbg/tcp-chat-server/internal/chat"
)

var (
	MsgBufferSize   = 64
	ErrQueueFull    = errors.New("queue is full")
	ErrClientClosed = errors.New("client is closed")
)

type client struct {
	id   chat.ClientID
	conn net.Conn

	msgQueue chan chat.Response
	done     chan struct{}

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func NewClient(id chat.ClientID, conn net.Conn) *client {
	return &client{
		id:       id,
		conn:     conn,
		msgQueue: make(chan chat.Response, MsgBufferSize),
		done:     make(chan struct{}),
	}
}

func (c *client) ID() chat.ClientID {
	return c.id
}

func (c *client) Deliver(msg chat.Response) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClientClosed
	}

	select {
	case c.msgQueue <- msg:
	default:
		return ErrQueueFull
	}
	return nil
}

func (c *client) WriteLoop() error {
	defer c.Close()

	for {
		select {
		case <-c.done:
			return nil
		case msg := <-c.msgQueue:
			select {
			case <-c.done:
				return nil
			default:
			}

			frame := []byte(msg.String() + "\n")
			n, err := c.conn.Write(frame)
			if err == nil && n != len(frame) {
				err = io.ErrShortWrite
			}
			if err != nil {
				select {
				case <-c.done:
					return nil
				default:
					return fmt.Errorf("client write: %w", err)
				}
			}
		}
	}
}

func (c *client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		close(c.done)
		c.mu.Unlock()

		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

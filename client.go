package main

import "errors"

type ClientID = uint64

var (
	MsgBufferSize = 64
	ErrQueueFull  = errors.New("queue is full")
)

type Client interface {
	ID() ClientID

	// Deliver queues a response without blocking and supports concurrent calls.
	// A nil error means the response was queued, not received by the recipient.
	// It returns an error if the queue is full or the client is closed.
	Deliver(Response) error
}

type client struct {
	id ClientID

	msgQueue chan Response
}

func NewClient(id ClientID) Client {
	return &client{
		id:       id,
		msgQueue: make(chan Response, MsgBufferSize),
	}
}

func (c *client) ID() ClientID {
	return c.id
}

func (c *client) Deliver(msg Response) error {
	select {
	case c.msgQueue <- msg:
	default:
		return ErrQueueFull
	}
	return nil
}

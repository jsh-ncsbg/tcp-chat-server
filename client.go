package main

type Client interface {
	Username() string
	Deliver(Message) error
}

type client struct {
	username string
}

func NewClient(username string) Client {
	return &client{
		username: username,
	}
}

func (c *client) Username() string {
	return c.username
}

func (c *client) Deliver(msg Message) error {
	// TODO
	return nil
}

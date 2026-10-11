package main

type ClientID = uint64

type Client interface {
	ID() ClientID
	Username() string
	Deliver(Response) error
}

type client struct {
	id       ClientID
	username string
}

func NewClient(id ClientID, username string) Client {
	return &client{
		id:       id,
		username: username,
	}
}

func (c *client) ID() ClientID {
	return c.id
}

func (c *client) Username() string {
	return c.username
}

func (c *client) Deliver(msg Response) error {
	// TODO
	return nil
}

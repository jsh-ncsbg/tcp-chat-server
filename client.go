package main

type Client interface {
	Username() string
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

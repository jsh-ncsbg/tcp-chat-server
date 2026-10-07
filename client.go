package main

type Client struct {
	username string
}

func NewClient(username string) *Client {
	return &Client{
		username: username,
	}
}

func (c *Client) Username() string {
	return c.username
}

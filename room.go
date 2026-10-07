package main

type Room struct {
	name string
}

func NewRoom(name string) *Room {
	return &Room{
		name: name,
	}
}

func (r *Room) Name() string {
	return r.name
}

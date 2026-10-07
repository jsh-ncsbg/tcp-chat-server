package main

type RoomID string

type Room struct {
	id RoomID
}

func NewRoom(id RoomID) *Room {
	return &Room{
		id: id,
	}
}

func (r *Room) ID() RoomID {
	return r.id
}

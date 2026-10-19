# WIP: tcp-chat-server

This project is a simple TCP chat server that handles concurrent connections. It extends _CSNETWK Laboratory #3: Introduction to Socket Programming_.

## Run

Requires Go 1.26.4 or newer.

```sh
make server
# or: go run ./cmd/server
```

Connect from another terminal using netcat:

```sh
nc 127.0.0.1 8080
```

Default rooms: `general` and `breakout-1` through `breakout-5`.

## Commands

Commands are uppercase, one per line. Register a unique username before joining a room or sending messages. Usernames and room names cannot contain whitespace.

| Command | Action |
| --- | --- |
| `REGISTER <username>` | Register once per connection. |
| `JOIN <room>` | Join an existing room or switch rooms. |
| `MSG <text>` | Broadcast to everyone in your room, including yourself. |
| `LEAVE` | Leave your current room. |
| `QUIT` | Disconnect. |

Example exchange (`>` marks what you type):

```text
> REGISTER Josh
INFO OK REGISTER
> JOIN general
INFO OK JOIN
> MSG hello
MSG general Josh hello
> LEAVE
INFO OK LEAVE
> QUIT
```

Errors use `ERR <code> <description>`. Disconnecting releases your username and room membership.

## Test

```sh
make test
go test -race ./...
```

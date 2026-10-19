.PHONY: test
test:
	go test ./... -v

.PHONY: server
server:
	go run ./cmd/server
package chat

type ClientID = uint64

type Client interface {
	ID() ClientID

	// Deliver queues a response without blocking and supports concurrent calls.
	// A nil error means the response was queued, not received by the recipient.
	// It returns an error if the queue is full or the client is closed.
	Deliver(Response) error
}

package main

import (
	"strings"
)

type ResponseType string

const (
	ErrorResponse   ResponseType = "ERR"
	MessageResponse ResponseType = "MSG"
	InfoResponse    ResponseType = "INFO"
)

type Response struct {
	Type    ResponseType
	Content string
}

func NewErrResponse(code, message string) Response {
	return Response{
		Type:    ErrorResponse,
		Content: code + " " + message,
	}
}

func NewMsgResponse(room, sender, content string) Response {
	return Response{
		Type:    MessageResponse,
		Content: strings.Join([]string{room, sender, content}, " "),
	}
}

func NewInfoResponse(content string) Response {
	return Response{
		Type:    InfoResponse,
		Content: content,
	}
}

func (r *Response) String() string {
	return string(r.Type) + " " + r.Content
}

package service

import (
	"context"
	"strings"
)

// EchoService is intentionally small. Replace it with the API, database, or
// automation service your agent-facing CLI exposes. Keeping it outside the
// command package makes the business logic easy to test and reuse from MCP.
type EchoService struct{}

type EchoResult struct {
	Message string `json:"message"`
	Length  int    `json:"length"`
}

type Echoer interface {
	Echo(context.Context, string, bool) (EchoResult, error)
}

func (EchoService) Echo(ctx context.Context, message string, upper bool) (EchoResult, error) {
	if err := ctx.Err(); err != nil {
		return EchoResult{}, err
	}
	if upper {
		message = strings.ToUpper(message)
	}
	return EchoResult{Message: message, Length: len([]rune(message))}, nil
}

package spark

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"time"

	"campfire/internal/model"
)

// CommandName identifies an explicitly supported agent operation.
type CommandName string

// ConfigureServer stores desired server configuration without executing it.
const ConfigureServer CommandName = "server.configure"

// ServerAction requests a lifecycle operation or status inspection.
const ServerAction CommandName = "server.action"

// ServerFiles invokes contained game data file management.
const ServerFiles CommandName = "server.files"

// Command is a correlated, bounded-lifetime request.
type Command struct {
	ID         string                `json:"id"`
	Name       CommandName           `json:"name"`
	Deadline   time.Time             `json:"deadline"`
	Deployment *model.Deployment     `json:"deployment,omitempty"`
	Runtime    *model.RuntimeRequest `json:"runtime,omitempty"`
	Files      *model.FileRequest    `json:"files,omitempty"`
}

// CommandError describes a failed operation independently of transport failure.
type CommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *CommandError) Error() string { return e.Code + ": " + e.Message }

// CommandResponse completes exactly one request on the originating session.
type CommandResponse struct {
	ID      string                      `json:"id"`
	Receipt *model.ConfigurationReceipt `json:"receipt,omitempty"`
	Error   *CommandError               `json:"error,omitempty"`
	Status  *model.RuntimeStatus        `json:"status,omitempty"`
	Files   *model.FileResult           `json:"files,omitempty"`
}

// ErrDisconnected means the assigned Spark has no usable authenticated session.
var ErrDisconnected = errors.New("spark disconnected")

func validResponse(response CommandResponse) bool {
	count := 0
	if response.Files != nil {
		count++
	}

	if response.Error != nil {
		count++
	}

	if response.Receipt != nil {
		count++
	}

	if response.Status != nil {
		count++
	}

	return count == 1
}

// Files dispatches file management over the existing authenticated agent session.
func (h *Hub) Files(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	response, err := h.Execute(ctx, id, Command{Name: ServerFiles, Files: &request})
	if err != nil {
		var failure *CommandError
		if errors.As(err, &failure) {
			return model.FileResult{}, &model.FileError{Code: failure.Code, Message: failure.Message}
		}

		return model.FileResult{}, err
	}

	if response.Files == nil || response.Files.ServerID != request.ServerID {
		return model.FileResult{}, errors.New("invalid file response")
	}

	return *response.Files, nil
}

// Action dispatches a typed lifecycle request to the assigned remote Spark.
func (h *Hub) Action(ctx context.Context, id string, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	response, err := h.Execute(ctx, id, Command{Name: ServerAction, Runtime: &request})
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	if response.Status == nil || response.Status.ServerID != request.ServerID {
		return model.RuntimeStatus{}, errors.New("invalid runtime response")
	}

	return *response.Status, nil
}

// Execute sends a command and waits for its matching response, bounded to 30 seconds.
func (h *Hub) Execute(ctx context.Context, id string, command Command) (CommandResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	command.ID = rand.Text()
	command.Deadline, _ = ctx.Deadline()
	message := Message{Type: "command", Protocol: ProtocolVersion, Command: &command}

	encoded, err := json.Marshal(message)
	if err != nil || len(encoded) > 2<<20 {
		return CommandResponse{}, errors.New("command exceeds protocol limits")
	}

	h.mu.RLock()
	current := h.peers[id]
	h.mu.RUnlock()

	if current == nil {
		return CommandResponse{}, ErrDisconnected
	}

	response := make(chan CommandResponse, 1)

	current.mu.Lock()
	if len(current.pending) >= 64 {
		current.mu.Unlock()
		return CommandResponse{}, errors.New("spark command queue is full")
	}

	current.pending[command.ID] = response

	current.mu.Unlock()
	defer func() {
		current.mu.Lock()
		delete(current.pending, command.ID)
		current.mu.Unlock()
	}()

	select {
	case current.outbound <- message:
	case <-current.done:
		return CommandResponse{}, ErrDisconnected
	case <-ctx.Done():
		return CommandResponse{}, ctx.Err()
	}

	select {
	case result := <-response:
		if result.Error != nil {
			return result, result.Error
		}

		return result, nil
	case <-current.done:
		return CommandResponse{}, ErrDisconnected
	case <-ctx.Done():
		return CommandResponse{}, ctx.Err()
	}
}

// Deploy delivers a configuration and requires an acknowledgement for that server.
func (h *Hub) Deploy(ctx context.Context, id string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	result, err := h.Execute(ctx, id, Command{Name: ConfigureServer, Deployment: &deployment})
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if result.Receipt == nil || result.Receipt.ServerID != deployment.ServerID || len(result.Receipt.Revision) != 64 {
		return model.ConfigurationReceipt{}, errors.New("invalid configuration acknowledgement")
	}

	return *result.Receipt, nil
}

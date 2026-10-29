package agents

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

// ErrDisconnected means the assigned Spark has no usable authenticated session.
var ErrDisconnected = errors.New("spark disconnected")

func validResponse(response protocol.CommandResponse) bool {
	count := 0
	if response.Prune != nil {
		count++
	}

	if response.SparkVersion != "" {
		count++
	}

	if response.Transfer != nil {
		count++
	}

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

// PruneSpark requests bounded cleanup of unused Campfire Docker resources.
func (h *Hub) PruneSpark(ctx context.Context, id string) (model.PruneResult, error) {
	response, err := h.Execute(ctx, id, protocol.Command{Name: protocol.PruneSpark})
	if err != nil {
		return model.PruneResult{}, err
	}

	if response.Prune == nil || response.Prune.Containers < 0 || response.Prune.Images < 0 || response.Prune.Volumes < 0 {
		return model.PruneResult{}, errors.New("invalid Spark prune acknowledgement")
	}

	return *response.Prune, nil
}

// UpdateSpark requests a verified client replacement; reconnection confirms the running version.
func (h *Hub) UpdateSpark(ctx context.Context, id, version string) error {
	response, err := h.Execute(ctx, id, protocol.Command{Name: protocol.UpdateSpark, SparkVersion: version})
	if err != nil {
		return err
	}

	if response.SparkVersion != version {
		return errors.New("invalid Spark update acknowledgement")
	}

	return nil
}

// ReinstallSpark waits for the replaced process to disconnect before acknowledging.
func (h *Hub) ReinstallSpark(ctx context.Context, id, version string) error {
	ctx, cancel := context.WithTimeout(ctx, protocol.SparkUpdateTimeout)
	defer cancel()

	h.mu.RLock()
	current := h.peers[id]
	h.mu.RUnlock()

	if current == nil {
		return ErrDisconnected
	}

	response, err := h.execute(ctx, id, protocol.Command{Name: protocol.ReinstallSpark, SparkVersion: version}, current)
	if err != nil {
		return err
	}

	if response.SparkVersion != version {
		return errors.New("invalid Spark reinstall acknowledgement")
	}

	select {
	case <-current.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Transfer dispatches one bounded transfer step to its authenticated Spark.
func (h *Hub) Transfer(ctx context.Context, id string, request model.TransferRequest) (model.TransferResult, error) {
	response, err := h.Execute(ctx, id, protocol.Command{Name: protocol.ServerTransfer, Transfer: &request})
	if err != nil {
		return model.TransferResult{}, err
	}

	if response.Transfer == nil || response.Transfer.ID != request.ID || response.Transfer.ServerID != request.ServerID || len(response.Transfer.Data) > model.FileLimit || response.Transfer.Size < 0 || response.Transfer.Size > 100<<30 || response.Transfer.Offset < 0 || response.Transfer.Offset > response.Transfer.Size || len(response.Transfer.Error) > 1024 {
		return model.TransferResult{}, errors.New("invalid transfer response")
	}

	return *response.Transfer, nil
}

// Files dispatches file management over the existing authenticated agent session.
func (h *Hub) Files(ctx context.Context, id string, request model.FileRequest) (model.FileResult, error) {
	response, err := h.Execute(ctx, id, protocol.Command{Name: protocol.ServerFiles, Files: &request})
	if err != nil {
		var failure *protocol.CommandError
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
	response, err := h.Execute(ctx, id, protocol.Command{Name: protocol.ServerAction, Runtime: &request})
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	if response.Status == nil || response.Status.ServerID != request.ServerID {
		return model.RuntimeStatus{}, errors.New("invalid runtime response")
	}

	return *response.Status, nil
}

// Execute waits for a matching response within the command's bounded time budget.
func (h *Hub) Execute(ctx context.Context, id string, command protocol.Command) (protocol.CommandResponse, error) {
	return h.execute(ctx, id, command, nil)
}

func (h *Hub) execute(ctx context.Context, id string, command protocol.Command, expected *peer) (protocol.CommandResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, protocol.CommandTimeout(command))
	defer cancel()

	command.ID = rand.Text()
	command.Deadline, _ = ctx.Deadline()

	h.mu.RLock()
	current := h.peers[id]
	h.mu.RUnlock()

	if current == nil || (expected != nil && current != expected) {
		return protocol.CommandResponse{}, ErrDisconnected
	}

	if err := protocol.Require(current.info.Capabilities, protocol.RequiredCapabilities(command)...); err != nil {
		return protocol.CommandResponse{}, err
	}

	message := protocol.Message{Type: "command", Protocol: current.info.Protocol, Command: &command}

	encoded, err := json.Marshal(message)
	if err != nil || len(encoded) > 2<<20 {
		return protocol.CommandResponse{}, errors.New("command exceeds protocol limits")
	}

	response := make(chan protocol.CommandResponse, 1)

	current.mu.Lock()
	if len(current.pending) >= 64 {
		current.mu.Unlock()
		return protocol.CommandResponse{}, errors.New("spark command queue is full")
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
		return protocol.CommandResponse{}, ErrDisconnected
	case <-ctx.Done():
		return protocol.CommandResponse{}, ctx.Err()
	}

	var result protocol.CommandResponse
	select {
	case result = <-response:
	case <-current.done:
		// A restarting Spark can disconnect immediately after its acknowledgement.
		select {
		case result = <-response:
		default:
			return protocol.CommandResponse{}, ErrDisconnected
		}
	case <-ctx.Done():
		return protocol.CommandResponse{}, ctx.Err()
	}

	if result.Error != nil {
		return result, result.Error
	}

	return result, nil
}

// Deploy delivers a configuration and requires an acknowledgement for that server.
func (h *Hub) Deploy(ctx context.Context, id string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	result, err := h.Execute(ctx, id, protocol.Command{Name: protocol.ConfigureServer, Deployment: &deployment})
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	if result.Receipt == nil || result.Receipt.ServerID != deployment.ServerID || len(result.Receipt.Revision) != 64 {
		return model.ConfigurationReceipt{}, errors.New("invalid configuration acknowledgement")
	}

	return *result.Receipt, nil
}

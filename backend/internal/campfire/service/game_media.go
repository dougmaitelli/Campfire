package service

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg" // Register supported screenshot decoders.
	_ "image/png"
	"io"
	"net/http"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

// GameMediaService bounds concurrent uploads and verifies bytes before storage.
type GameMediaService struct {
	Store *repository.GameMediaRepository
	slots chan struct{}
}

// NewGameMediaService permits two bounded in-flight files across all Sparks.
func NewGameMediaService(store *repository.GameMediaRepository) *GameMediaService {
	return &GameMediaService{Store: store, slots: make(chan struct{}, 2)}
}

// Receive is synchronous: success means both metadata and bytes committed.
func (s *GameMediaService) Receive(ctx context.Context, sparkID, serverID, eventID string, media model.GameMedia, body io.Reader) error {
	if !media.Valid() || eventID == "" || len(eventID) > 128 {
		return model.ErrGameMediaInvalid
	}

	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return model.ErrGameMediaBusy
	}

	data, err := io.ReadAll(io.LimitReader(body, model.GameMediaFileLimit+1))
	if err != nil {
		return err
	}

	if len(data) > model.GameMediaFileLimit {
		return model.ErrGameMediaLimit
	}

	if len(data) == 0 || http.DetectContentType(data) != media.ContentType {
		return model.ErrGameMediaInvalid
	}

	if media.ContentType == "image/png" || media.ContentType == "image/jpeg" {
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 40_000_000 {
			return model.ErrGameMediaInvalid
		}
	}

	media.Size = int64(len(data))

	return s.Store.Append(ctx, sparkID, serverID, eventID, media, data)
}

package service

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestGameMediaRejectsUnsafeOrOversizedContentBeforeStorage(t *testing.T) {
	s := NewGameMediaService(nil)

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}

	valid := model.GameMedia{ID: "shot", Name: "world.png", ContentType: "image/png"}
	for _, test := range []struct {
		name  string
		media model.GameMedia
		data  []byte
		want  error
	}{
		{"html spoof", valid, []byte("<script>alert(1)</script>"), model.ErrGameMediaInvalid},
		{"invalid png", valid, []byte("\x89PNG\r\n\x1a\n"), model.ErrGameMediaInvalid},
		{"filename traversal", model.GameMedia{ID: "shot", Name: "../world.png", ContentType: "image/png"}, pngData.Bytes(), model.ErrGameMediaInvalid},
		{"active content", model.GameMedia{ID: "shot", Name: "world.svg", ContentType: "image/svg+xml"}, []byte("<svg/>"), model.ErrGameMediaInvalid},
		{"mime mismatch", model.GameMedia{ID: "shot", Name: "world.jpg", ContentType: "image/jpeg"}, pngData.Bytes(), model.ErrGameMediaInvalid},
		{"empty", valid, nil, model.ErrGameMediaInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := s.Receive(t.Context(), "spark", "server", "event", test.media, bytes.NewReader(test.data)); !errors.Is(err, test.want) {
				t.Fatal(err)
			}
		})
	}

	if err := s.Receive(t.Context(), "spark", "server", "event", valid, strings.NewReader(strings.Repeat("x", model.GameMediaFileLimit+1))); !errors.Is(err, model.ErrGameMediaLimit) {
		t.Fatal("file cap ignored", err)
	}

	for range cap(s.slots) {
		s.slots <- struct{}{}
	}

	if err := s.Receive(t.Context(), "spark", "server", "event", valid, bytes.NewReader(pngData.Bytes())); !errors.Is(err, model.ErrGameMediaBusy) {
		t.Fatal("concurrency cap ignored", err)
	}
}

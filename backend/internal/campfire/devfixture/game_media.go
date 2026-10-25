//go:build !production

package devfixture

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

// GameScreenshot is a synthetic scene shared by mock publishers and browser tests.
func GameScreenshot() ([]byte, error) {
	frame := image.NewRGBA(image.Rect(0, 0, 320, 180))
	draw.Draw(frame, frame.Bounds(), image.NewUniform(color.RGBA{R: 114, G: 179, B: 217, A: 255}), image.Point{}, draw.Src)
	draw.Draw(frame, image.Rect(0, 112, 320, 180), image.NewUniform(color.RGBA{R: 71, G: 115, B: 62, A: 255}), image.Point{}, draw.Src)
	draw.Draw(frame, image.Rect(225, 70, 241, 140), image.NewUniform(color.RGBA{R: 105, G: 69, B: 43, A: 255}), image.Point{}, draw.Src)
	draw.Draw(frame, image.Rect(201, 30, 265, 86), image.NewUniform(color.RGBA{R: 40, G: 83, B: 51, A: 255}), image.Point{}, draw.Src)

	var output bytes.Buffer

	err := png.Encode(&output, frame)

	return output.Bytes(), err
}

// GameClip is a one-second synthetic 64x48 VP9/WebM clip (four green frames).
// Generated with FFmpeg's color source; contains no external game assets.
func GameClip() ([]byte, error) {
	return base64.StdEncoding.DecodeString("GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwH/////////EU2bdKtNu4tTq4QVSalmU6yBoU27i1OrhBZUrmtTrIHLTbuMU6uEElTDZ1OsggEY7AEAAAAAAABoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmpSrXsYMPQkBNgIxMYXZmNjMuMS4xMDJXQYxMYXZmNjMuMS4xMDIWVK5ryK4BAAAAAAAAP9eBAXPFiEN4nyBWJZAbnIEAIrWcg3VuZIiBAIaFVl9WUDmDgQEj44OEDuaygOCQsIFAuoEwmoECVbCEVbmBARJUw2fac3OfY8CAZ8iZRaOHRU5DT0RFUkSHjExhdmY2My4xLjEwMnNztWPAi2PFiEN4nyBWJZAbZ8ikRaOHRU5DT0RFUkSHl0xhdmM2My4xLjEwMiBsaWJ2cHgtdnA5H0O2dUCm54EAo+KBAACAgkmDQgAD8AL2BjgkHBhKAAAgQABrQ///sw4WTxAsBdpuYEctIOU/13Co4ix2HNm49BDQ//8TnqOerkDh/zGOyILaqZZ6EgqrjRJRBfYQTkgC///1vv///5cjo8OiAKOTgQD6AIYAQJKcSFAAAANwAABUcKOTgQH0AIYAQJKcQE7gAANwAABUcKOTgQLuAIYAQJKcSFAAAANwAABUcA==")
}

// SeedGameMedia attaches deterministic media to the existing advancement fixture.
func SeedGameMedia(ctx context.Context, db *gorm.DB) error {
	for _, fixture := range []struct {
		id, name, contentType string
		bytes                 func() ([]byte, error)
	}{
		{"screenshot", "mock-world.png", "image/png", GameScreenshot},
		{"clip", "mock-clip.webm", "video/webm", GameClip},
	} {
		data, err := fixture.bytes()
		if err != nil {
			return err
		}

		media := model.GameMedia{ID: fixture.id, Name: fixture.name, ContentType: fixture.contentType, Size: int64(len(data))}
		if err := repository.NewGameMediaRepository(db).Append(ctx, "mock-europe", "survival", "fixture-advancement", media, data); err != nil {
			return err
		}
	}

	return nil
}

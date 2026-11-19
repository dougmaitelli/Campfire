//go:build !production

package integrations

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"campfire/internal/campfire/service/integrations/discord"
	"campfire/internal/shared/model"
)

type discordMessageInput struct {
	Nonce   string           `json:"nonce"`
	Content string           `json:"content"`
	Embeds  []map[string]any `json:"embeds"`
}

type discordAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	URL         string `json:"url"`
	Data        []byte `json:"-"`
}

func readDiscordMessage(w http.ResponseWriter, r *http.Request) (discordMessageInput, []discordAttachment, error) {
	var payload discordMessageInput
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
		err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&payload)
		return payload, nil, err
	}

	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)

	reader, err := r.MultipartReader()
	if err != nil {
		return payload, nil, err
	}

	var attachments []discordAttachment

	var receivedPayload bool

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return payload, nil, err
		}

		if part.FormName() == "payload_json" && !receivedPayload {
			if err := json.NewDecoder(io.LimitReader(part, (64<<10)+1)).Decode(&payload); err != nil {
				return payload, nil, err
			}

			receivedPayload = true

			continue
		}

		index := strconv.Itoa(len(attachments))
		if part.FormName() != "files["+index+"]" || len(attachments) >= model.GameMediaEventLimit {
			return payload, nil, errors.New("invalid mock Discord multipart field")
		}

		data, err := io.ReadAll(io.LimitReader(part, discord.AttachmentFileLimit+1))
		if err != nil {
			return payload, nil, err
		}

		media := model.GameMedia{ID: index, Name: part.FileName(), ContentType: part.Header.Get("Content-Type"), Size: int64(len(data))}
		if !media.Valid() || len(data) == 0 || len(data) > discord.AttachmentFileLimit {
			return payload, nil, errors.New("invalid mock Discord attachment")
		}

		attachments = append(attachments, discordAttachment{ID: index, Filename: media.Name, ContentType: media.ContentType, Size: len(data), Data: data})
	}

	if !receivedPayload {
		return payload, nil, errors.New("missing mock Discord payload_json")
	}

	return payload, attachments, nil
}

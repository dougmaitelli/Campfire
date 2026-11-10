package discord

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strconv"

	"campfire/internal/shared/model"
)

// AttachmentFileLimit uses Discord's default per-file allowance. Larger game
// uploads remain in Activity but cannot be assumed deliverable to every guild.
const AttachmentFileLimit = 20 << 20

// Attachment contains validated retained media, never a URL for Discord to fetch.
type Attachment struct {
	Media model.GameMedia
	Data  []byte
}

// multipartBody can be replayed byte-for-byte with the same enforced nonce.
type multipartBody struct {
	data        []byte
	contentType string
}

func attachmentBody(payload any, attachments []Attachment) (multipartBody, error) {
	if len(attachments) > model.GameMediaEventLimit {
		return multipartBody{}, &Error{"Too many Discord attachments."}
	}

	var size int

	for _, file := range attachments {
		if !file.Media.Valid() || len(file.Data) == 0 || int64(len(file.Data)) != file.Media.Size || len(file.Data) > AttachmentFileLimit {
			return multipartBody{}, &Error{"Invalid Discord attachment or file exceeds the 20 MiB upload limit."}
		}

		size += len(file.Data)
	}

	// Leave room for JSON and multipart headers beneath Discord's 25 MiB request limit.
	if size > 24<<20 {
		return multipartBody{}, &Error{"Discord attachments exceed the message upload limit."}
	}

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	data, err := json.Marshal(payload)
	if err != nil {
		return multipartBody{}, err
	}

	if err := writer.WriteField("payload_json", string(data)); err != nil {
		return multipartBody{}, err
	}

	for index, file := range attachments {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "files[" + strconv.Itoa(index) + "]", "filename": file.Media.Name}))
		header.Set("Content-Type", file.Media.ContentType)

		part, err := writer.CreatePart(header)
		if err != nil {
			return multipartBody{}, err
		}

		if _, err := part.Write(file.Data); err != nil {
			return multipartBody{}, err
		}
	}

	if err := writer.Close(); err != nil {
		return multipartBody{}, err
	}

	return multipartBody{data: body.Bytes(), contentType: writer.FormDataContentType()}, nil
}

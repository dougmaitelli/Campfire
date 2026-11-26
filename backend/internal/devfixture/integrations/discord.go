//go:build !production

// Package integrations provides isolated external-service fixtures.
package integrations

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"campfire/internal/campfire/service/integrations/discord"
)

// DiscordToken is a deliberately fake credential for isolated development/tests.
const DiscordToken = "campfire-test-discord-token"

// Discord keeps a simulated provider's messages and channel failures in memory.
// Its HTTP API and viewer share this state; neither is registered in production.
type Discord struct {
	mu          sync.Mutex
	messages    []map[string]any
	nonces      map[string]map[string]any
	attempts    map[string]int
	failures    map[string]string
	sequence    int
	attachments map[string][]discordAttachment
	mediaBytes  int
}

// NewDiscord creates a fresh disposable provider.
func NewDiscord() *Discord {
	d := &Discord{}
	d.Reset()

	return d
}

// Reset clears provider state between isolated browser-test cases.
func (d *Discord) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.messages = []map[string]any{}
	d.nonces = map[string]map[string]any{}
	d.attempts = map[string]int{}
	d.failures = map[string]string{}
	d.sequence = 499
	d.attachments = map[string][]discordAttachment{}
	d.mediaBytes = 0
}

// DiscordHandler preserves the small HTTP-fixture entry point for service tests.
func DiscordHandler() http.Handler { return NewDiscord().Handler() }

// Handler exposes only the simulated Discord API, without inspection controls.
func (d *Discord) Handler() http.Handler { return http.HandlerFunc(d.serveAPI) }

func (d *Discord) serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bot ")
	if credential != DiscordToken && credential != DiscordToken+"-rotated" && credential != DiscordToken+"-retry-card" && credential != DiscordToken+"-unconfirmed-card" {
		discordError(w, http.StatusUnauthorized, 0, "Invalid token")
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "channels" {
		switch d.failures[parts[1]] {
		case "forbidden":
			discordError(w, http.StatusForbidden, 50013, "Missing permissions")
			return
		case "rate-limit":
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"retry_after": 5, "global": false})

			return
		case "unavailable":
			discordError(w, http.StatusServiceUnavailable, 0, "Mock Discord unavailable")
			return
		}
	}

	var value any

	switch r.Method + " " + r.URL.Path {
	case "GET /users/@me":
		value = map[string]any{"id": "100", "username": "Campfire Bot", "bot": true}
	case "GET /users/@me/guilds":
		value = []map[string]any{{"id": "200", "name": "Campfire Community", "owner": false}}
	case "GET /guilds/200/roles":
		value = []map[string]any{{"id": "200", "permissions": strconv.FormatUint(discord.RequiredBotPermissions, 10)}}
	case "GET /guilds/200/members/100":
		value = map[string]any{"roles": []string{}}
	case "GET /guilds/200/channels":
		channels := []map[string]any{}

		for _, channel := range discordChannels {
			overrides := []map[string]any{}
			if channel.ID == "401" || d.failures[channel.ID] == "forbidden" {
				overrides = append(overrides, map[string]any{"id": "200", "type": 0, "allow": "0", "deny": "2048"})
			}

			channels = append(channels, map[string]any{"id": channel.ID, "guild_id": "200", "type": 0, "name": channel.Name, "position": len(channels), "permission_overwrites": overrides})
		}

		value = channels
	case "POST /channels/400/messages", "POST /channels/402/messages":
		value = d.createMessage(w, r, parts[1], credential)
		if value == nil {
			return
		}
	case "GET /channels/400/messages", "GET /channels/402/messages":
		page := []map[string]any{}
		before, _ := strconv.Atoi(r.URL.Query().Get("before"))

		for i := len(d.messages) - 1; i >= 0 && len(page) < 100; i-- {
			message := d.messages[i]
			if message["channel_id"] != parts[1] {
				continue
			}

			rawID, _ := message["id"].(string)

			id, _ := strconv.Atoi(rawID)
			if before == 0 || id < before {
				page = append(page, message)
			}
		}

		value = page
	default:
		if len(parts) != 4 || parts[0] != "channels" || parts[2] != "messages" || (parts[1] != "400" && parts[1] != "402") {
			discordError(w, http.StatusNotFound, 10003, "Unknown fixture channel")
			return
		}

		found := d.findMessage(parts[1], parts[3])
		if found < 0 {
			discordError(w, http.StatusNotFound, 10008, "Unknown fixture message")
			return
		}

		switch r.Method {
		case http.MethodGet:
			value = d.messages[found]
		case http.MethodPatch:
			var payload struct {
				Embeds []map[string]any `json:"embeds"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&payload) != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			d.messages[found]["embeds"] = payload.Embeds
			d.messages[found]["edited_timestamp"] = time.Now().UTC()
			value = d.messages[found]
		case http.MethodDelete:
			d.deleteMessage(found)
			w.WriteHeader(http.StatusNoContent)

			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}

	_ = json.NewEncoder(w).Encode(value)
}

func (d *Discord) createMessage(w http.ResponseWriter, r *http.Request, channelID, credential string) any {
	payload, attachments, err := readDiscordMessage(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil
	}

	key := channelID + ":" + payload.Nonce
	d.attempts[key]++

	title := ""
	if len(payload.Embeds) > 0 {
		title, _ = payload.Embeds[0]["title"].(string)
		footer, _ := payload.Embeds[0]["footer"].(map[string]any)

		marker, _ := footer["text"].(string)
		if strings.HasPrefix(marker, "Campfire server · ") {
			if credential == DiscordToken+"-unconfirmed-card" {
				w.WriteHeader(http.StatusInternalServerError)
				return nil
			}

			if credential == DiscordToken+"-retry-card" && d.attempts[key] == 1 {
				w.WriteHeader(http.StatusBadRequest)
				return nil
			}
		}
	}

	if strings.Contains(title, "Fixture rejected notification") && d.attempts[key] == 1 {
		discordError(w, http.StatusBadRequest, 0, "private provider body must not escape")
		return nil
	}

	if strings.Contains(title, "Fixture unconfirmed notification") {
		discordError(w, http.StatusInternalServerError, 0, "unconfirmed")
		return nil
	}

	if previous, exists := d.nonces[key]; payload.Nonce != "" && exists {
		return previous
	}

	d.sequence++

	message := map[string]any{"id": strconv.Itoa(d.sequence), "channel_id": channelID, "timestamp": time.Now().UTC(), "author": map[string]string{"id": "100"}, "content": payload.Content, "embeds": payload.Embeds}
	if len(attachments) > 0 {
		id := strconv.Itoa(d.sequence)
		for index := range attachments {
			attachments[index].URL = "/mock/discord/messages/" + id + "/attachments/" + attachments[index].ID
			d.mediaBytes += len(attachments[index].Data)
		}

		d.attachments[id] = attachments
		message["attachments"] = attachments
	}

	d.messages = append(d.messages, message)
	if payload.Nonce != "" {
		d.nonces[key] = message
	}
	// Keep long-running development sessions bounded, like a retained channel history.
	for len(d.messages) > 2000 || d.mediaBytes > 64<<20 {
		d.deleteMessage(0)
	}

	return message
}

func (d *Discord) findMessage(channelID, id string) int {
	for index, message := range d.messages {
		if message["id"] == id && (channelID == "" || message["channel_id"] == channelID) {
			return index
		}
	}

	return -1
}

func (d *Discord) deleteMessage(index int) {
	id := d.messages[index]["id"]
	if id, ok := id.(string); ok {
		for _, attachment := range d.attachments[id] {
			d.mediaBytes -= len(attachment.Data)
		}

		delete(d.attachments, id)
	}

	for key, message := range d.nonces {
		if message["id"] == id {
			delete(d.nonces, key)
			delete(d.attempts, key)
		}
	}

	d.messages = append(d.messages[:index], d.messages[index+1:]...)
}

func discordError(w http.ResponseWriter, status, code int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message})
}

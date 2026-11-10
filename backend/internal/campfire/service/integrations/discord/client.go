// Package discord implements Discord-specific configuration and HTTP transport.
package discord

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	viewChannel   = uint64(1 << 10)
	sendMessages  = uint64(1 << 11)
	embedLinks    = uint64(1 << 14)
	attachFiles   = uint64(1 << 15)
	readHistory   = uint64(1 << 16)
	administrator = uint64(1 << 3)
	// RequiredBotPermissions is the single permission policy for setup and delivery.
	RequiredBotPermissions = viewChannel | sendMessages | embedLinks | attachFiles | readHistory
)

// Error reports safe actionable failures without echoing Discord response bodies.
type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// User identifies a validated bot account.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

// Guild identifies a Discord server accessible to the bot.
type Guild struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner bool   `json:"owner"`
}

// Preview allows administrators to choose a guild without entering its ID.
type Preview struct {
	Bot       User    `json:"bot"`
	Guilds    []Guild `json:"guilds"`
	InviteURL string  `json:"invite_url"`
}

// Channel reports effective permissions for a supported text destination.
type Channel struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Available bool     `json:"available"`
	Missing   []string `json:"missing"`
}

// Access is a checked bot/guild connection and its channel permission results.
type Access struct {
	Bot      User      `json:"bot"`
	Guild    Guild     `json:"guild"`
	Channels []Channel `json:"channels"`
}

// Configuration is provider-owned public data; it never contains the bot token.
type Configuration struct {
	BotID     string `json:"bot_id"`
	BotName   string `json:"bot_name"`
	GuildID   string `json:"guild_id"`
	GuildName string `json:"guild_name"`
}

// Client uses a fixed production API origin; tests can inject an internal origin.
type Client struct {
	base   string
	http   *http.Client
	rates  rateLimits
	access accessCache
}

// NewClient builds bounded HTTP transport and forbids credential-bearing redirects.
func NewClient(base string, client *http.Client) *Client {
	if base == "" {
		base = "https://discord.com/api/v10"
	}

	transport := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		transport = *client
	}

	if transport.Timeout <= 0 {
		transport.Timeout = 10 * time.Second
	}

	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	return &Client{base: strings.TrimRight(base, "/"), http: &transport}
}

// Preview validates bot identity and follows guild-list pagination.
func (c *Client) Preview(ctx context.Context, token string) (Preview, error) {
	result := Preview{Guilds: []Guild{}}
	if err := c.request(ctx, token, "GET", "/users/@me", nil, &result.Bot); err != nil {
		return result, err
	}

	if !result.Bot.Bot || !ValidID(result.Bot.ID) {
		return result, &Error{"Discord integration requires a bot account."}
	}

	result.InviteURL = BotInviteURL(result.Bot.ID, "")

	after := ""

	for {
		var page []Guild

		if err := c.request(ctx, token, "GET", "/users/@me/guilds?limit=200&after="+url.QueryEscape(after), nil, &page); err != nil {
			return result, err
		}

		result.Guilds = append(result.Guilds, page...)
		if len(page) < 200 {
			break
		}

		next := page[len(page)-1].ID
		if !ValidID(next) || next == after || len(result.Guilds) > 10000 {
			return result, &Error{"Discord returned an invalid server list."}
		}

		after = next
	}

	slices.SortFunc(result.Guilds, func(a, b Guild) int { return strings.Compare(a.Name, b.Name) })

	return result, nil
}

// BotInviteURL requests only the permissions used by setup and delivery. Discord
// identifies the bot application by its public bot-user ID; tokens stay private.
func BotInviteURL(botID, guildID string) string {
	if !ValidID(botID) {
		return ""
	}

	query := url.Values{"client_id": {botID}, "scope": {"bot"}, "permissions": {strconv.FormatUint(RequiredBotPermissions, 10)}}
	if ValidID(guildID) {
		query.Set("guild_id", guildID)
	}

	return "https://discord.com/oauth2/authorize?" + query.Encode()
}

type role struct {
	ID          string `json:"id"`
	Permissions string `json:"permissions"`
}

type overwrite struct {
	ID    string `json:"id"`
	Type  int    `json:"type"`
	Allow string `json:"allow"`
	Deny  string `json:"deny"`
}

type channel struct {
	ID         string      `json:"id"`
	GuildID    string      `json:"guild_id"`
	Name       string      `json:"name"`
	Type       int         `json:"type"`
	Position   int         `json:"position"`
	Overwrites []overwrite `json:"permission_overwrites"`
}

// Check validates guild membership and computes each text channel's permissions.
func (c *Client) Check(ctx context.Context, token, guildID string) (Access, error) {
	key := accessKey(token, guildID)

	c.access.mu.Lock()
	generation := c.access.generation
	c.access.mu.Unlock()
	result, err := c.check(ctx, token, guildID)
	c.access.mu.Lock()
	defer c.access.mu.Unlock()

	if err != nil {
		delete(c.access.entries, key)
	} else if generation == c.access.generation {
		if c.access.entries == nil || len(c.access.entries) >= 64 {
			c.access.entries = make(map[string]accessEntry)
		}

		c.access.entries[key] = accessEntry{value: cloneAccess(result), expires: time.Now().Add(30 * time.Second)}
	}

	return result, err
}

func (c *Client) check(ctx context.Context, token, guildID string) (Access, error) {
	result := Access{Channels: []Channel{}}
	if !ValidID(guildID) {
		return result, &Error{"Choose a Discord server accessible to the bot."}
	}

	preview, err := c.Preview(ctx, token)
	if err != nil {
		return result, err
	}

	index := slices.IndexFunc(preview.Guilds, func(item Guild) bool { return item.ID == guildID })
	if index < 0 {
		return result, &Error{"The bot does not have access to the selected Discord server."}
	}

	result.Bot, result.Guild = preview.Bot, preview.Guilds[index]

	var roles []role

	var member struct {
		Roles []string `json:"roles"`
	}

	var channels []channel

	for _, endpoint := range []struct {
		path   string
		output any
	}{
		{"/guilds/" + guildID + "/roles", &roles},
		{"/guilds/" + guildID + "/members/" + result.Bot.ID, &member},
		{"/guilds/" + guildID + "/channels", &channels},
	} {
		if err := c.request(ctx, token, "GET", endpoint.path, nil, endpoint.output); err != nil {
			return result, err
		}
	}

	base := uint64(0)

	for _, item := range roles {
		if item.ID == guildID || slices.Contains(member.Roles, item.ID) {
			bits, err := permissions(item.Permissions)
			if err != nil {
				return result, err
			}

			base |= bits
		}
	}

	slices.SortFunc(channels, func(a, b channel) int { return a.Position - b.Position })

	for _, item := range channels {
		if (item.Type != 0 && item.Type != 5) || item.GuildID != guildID || !ValidID(item.ID) {
			continue
		}

		bits, err := effectivePermissions(base, guildID, result.Bot.ID, member.Roles, item.Overwrites, result.Guild.Owner)
		if err != nil {
			return result, err
		}

		result.Channels = append(result.Channels, Channel{ID: item.ID, Name: item.Name, Available: bits&RequiredBotPermissions == RequiredBotPermissions, Missing: missingPermissions(bits)})
	}

	return result, nil
}

// TestMessage checks the selected channel again immediately before sending.
func (c *Client) TestMessage(ctx context.Context, token, guildID, channelID string) error {
	if !ValidID(channelID) {
		return &Error{"Choose a Discord text channel."}
	}

	access, err := c.Check(ctx, token, guildID)
	if err != nil {
		return err
	}

	if !slices.ContainsFunc(access.Channels, func(item Channel) bool { return item.ID == channelID && item.Available }) {
		return &Error{"The selected channel is outside this Discord server or is missing required bot permissions."}
	}

	payload := map[string]any{"content": "Campfire integration is ready. This is a test message.", "allowed_mentions": map[string]any{"parse": []string{}}}

	return c.request(ctx, token, "POST", "/channels/"+channelID+"/messages", payload, nil)
}

// ValidID accepts canonical Discord snowflakes without lossy numeric conversion.
func ValidID(value string) bool {
	id, err := strconv.ParseUint(value, 10, 64)
	return err == nil && id != 0 && strconv.FormatUint(id, 10) == value
}

func (c *Client) request(ctx context.Context, token, method, path string, payload, output any) error {
	return c.do(ctx, token, method, path, payload, output)
}

func permissions(value string) (uint64, error) {
	bits, ok := new(big.Int).SetString(value, 10)
	if !ok || bits.Sign() < 0 {
		return 0, &Error{"Discord returned invalid permissions."}
	}

	return bits.Uint64(), nil
}

func effectivePermissions(base uint64, guildID, botID string, roles []string, overwrites []overwrite, owner bool) (uint64, error) {
	if owner || base&administrator != 0 {
		return RequiredBotPermissions, nil
	}

	for _, stage := range []int{0, 1, 2} {
		allow, deny := uint64(0), uint64(0)

		for _, item := range overwrites {
			applies := stage == 0 && item.Type == 0 && item.ID == guildID || stage == 1 && item.Type == 0 && slices.Contains(roles, item.ID) && item.ID != guildID || stage == 2 && item.Type == 1 && item.ID == botID
			if !applies {
				continue
			}

			a, err := permissions(item.Allow)
			if err != nil {
				return 0, err
			}

			d, err := permissions(item.Deny)
			if err != nil {
				return 0, err
			}

			allow, deny = allow|a, deny|d
		}

		base = base&^deny | allow
	}

	return base, nil
}

func missingPermissions(bits uint64) []string {
	missing := []string{}

	for _, permission := range []struct {
		bit  uint64
		name string
	}{{viewChannel, "View Channel"}, {sendMessages, "Send Messages"}, {embedLinks, "Embed Links"}, {attachFiles, "Attach Files"}, {readHistory, "Read Message History"}} {
		if RequiredBotPermissions&permission.bit != 0 && bits&permission.bit == 0 {
			missing = append(missing, permission.name)
		}
	}

	return missing
}

// RequiredPermissionNames derives setup guidance from the required permission mask.
func RequiredPermissionNames() []string { return missingPermissions(0) }

// IsError allows the HTTP boundary to distinguish expected provider failures.
func IsError(err error) bool {
	var provider *Error
	return errors.As(err, &provider)
}

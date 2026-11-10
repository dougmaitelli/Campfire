package integrations

import (
	"errors"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"campfire/internal/shared/model"
)

// CardField contains only an explicitly selected, current game value.
type CardField struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Available bool   `json:"available"`
}

// ServerCard is a provider-independent view, without deployment or secret data.
type ServerCard struct {
	Name      string             `json:"name"`
	State     model.RuntimeState `json:"state"`
	Addresses []string           `json:"addresses"`
	Fields    []CardField        `json:"fields"`
	ExpiresAt int64              `json:"expires_at"`
}

// Validate keeps card selection independent of notification subscriptions.
func (c CardSettings) Validate() error {
	if !validNames(c.Fields, 32) {
		return errors.New("invalid server card fields")
	}

	for _, field := range c.Fields {
		if field != "players" && field != "capacity" && (!strings.HasPrefix(field, "field.") || !model.ValidGameEventType(strings.TrimPrefix(field, "field."))) {
			return errors.New("invalid server card field")
		}
	}

	return nil
}

// BuildServerCard accepts only a fresh game snapshot from the runtime source.
// Missing values remain unavailable; unselected fields never enter the card.
func BuildServerCard(server model.Server, state model.RuntimeState, game *model.GameState, settings CardSettings) ServerCard {
	card := ServerCard{Name: server.Name, State: state, Addresses: cardAddresses(server), Fields: []CardField{}}
	if state == "" {
		card.State = model.RuntimeStateUnknown
	}

	if card.State != model.RuntimeStateRunning {
		game = nil
	}

	for _, id := range settings.Fields {
		field := CardField{ID: id, Name: strings.TrimPrefix(id, "field.")}
		switch id {
		case "players":
			if game != nil && game.Players != nil {
				field.Value, field.Available = strconv.Itoa(*game.Players), true
			}
		case "capacity":
			if game != nil && game.Capacity != nil {
				field.Value, field.Available = strconv.Itoa(*game.Capacity), true
			}
		default:
			if game != nil {
				field.Value, field.Available = ScalarText(game.Fields[strings.TrimPrefix(id, "field.")])
				field.Available = field.Available && strings.TrimSpace(field.Value) != ""
			}
		}

		card.Fields = append(card.Fields, field)
	}

	return card
}

func cardAddresses(server model.Server) []string {
	addresses := []string{}

	for _, port := range server.Ports {
		host := strings.TrimSpace(server.GameAddress)
		if host == "" {
			host = port.BindIP
		}

		if host == "" || port.HostPort < 1 || port.HostPort > 65535 {
			continue
		}

		if ip, err := netip.ParseAddr(host); err == nil && (ip.IsUnspecified() || ip.IsMulticast()) {
			continue
		}

		address := net.JoinHostPort(host, strconv.Itoa(port.HostPort)) + " / " + strings.ToUpper(string(port.Protocol))
		if !slices.Contains(addresses, address) {
			addresses = append(addresses, address)
		}
	}

	if len(server.Ports) == 0 && strings.TrimSpace(server.GameAddress) != "" {
		addresses = append(addresses, strings.TrimSpace(server.GameAddress))
	}

	return addresses
}

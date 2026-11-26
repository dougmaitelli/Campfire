package service

import (
	"context"
	"time"

	"campfire/internal/campfire/database/repository"
	core "campfire/internal/campfire/integrations"
	accessperm "campfire/internal/campfire/permissions"
	integrationservice "campfire/internal/campfire/service/integrations"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

// StatusPageService separates administrator publication from anonymous availability.
type StatusPageService struct {
	Store      *repository.StatusPageRepository
	Deployment *DeploymentService
	Discord    *integrationservice.DiscordIntegrationService
}

// List requires current administrator authority and unrestricted token scope.
func (s *StatusPageService) List(ctx context.Context, p Principal) ([]statuspage.Page, error) {
	if !p.Admin || !p.Allows(accessperm.AllAccess) {
		return nil, repository.ErrAccessDenied
	}

	return s.Store.List(ctx)
}

// Save validates a publication before committing its membership atomically.
func (s *StatusPageService) Save(ctx context.Context, p Principal, id string, input statuspage.Input) (statuspage.Page, error) {
	if !p.Admin || !p.Allows(accessperm.AllAccess) {
		return statuspage.Page{}, repository.ErrAccessDenied
	}

	if err := input.Normalize(); err != nil {
		return statuspage.Page{}, err
	}

	if input.DiscordChannelID == nil || *input.DiscordChannelID == "" {
		return s.Store.Save(ctx, id, input)
	}

	if s.Discord == nil {
		return statuspage.Page{}, repository.ErrIntegrationDisabled
	}

	connection, err := s.Discord.PageCardConnection(ctx, p, *input.DiscordChannelID)
	if err != nil {
		return statuspage.Page{}, err
	}

	return s.Store.SaveWithDiscord(ctx, id, input, connection)
}

// Delete revokes a page's public URL.
func (s *StatusPageService) Delete(ctx context.Context, p Principal, id string) error {
	if !p.Admin || !p.Allows(accessperm.AllAccess) {
		return repository.ErrAccessDenied
	}

	return s.Store.Delete(ctx, id)
}

// Public projects only explicit publication metadata and fresh cached availability.
func (s *StatusPageService) Public(ctx context.Context, slug string) (statuspage.PublicPage, error) {
	result := statuspage.PublicPage{Entries: []statuspage.PublicEntry{}, PollSeconds: statuspage.PollSeconds}
	if !statuspage.ValidSlug(slug) {
		return result, statuspage.ErrNotFound
	}

	title, targets, err := s.Store.Published(ctx, slug)
	if err != nil {
		return result, err
	}

	result.Title = title

	observed := time.Now()

	observations := s.pageObservations(targets)

	gameRequests := []model.ResourceTarget{}

	for index, target := range targets {
		if target.GameIntegration && !target.Disabled && !observations[index].Observed.IsZero() && publicAvailability(observations[index].Status) == statuspage.Online {
			gameRequests = append(gameRequests, model.ResourceTarget{ID: target.ServerID, SparkID: target.SparkID})
		}
	}

	games := map[string]gameStateObservation{}

	if s.Deployment != nil {
		states, err := s.Deployment.cachedGameStates(ctx, gameRequests)
		if err != nil {
			return result, err
		}

		for index, request := range gameRequests {
			games[request.ID] = states[index]
		}
	}

	histories := make(map[string]model.AvailabilityHistory, len(targets))
	if s.Deployment != nil && s.Deployment.History != nil {
		ids := make([]string, 0, len(targets))
		for _, target := range targets {
			ids = append(ids, target.ServerID)
		}

		histories, err = s.Deployment.History.Store.Availability(ctx, ids, observed.Unix())
		if err != nil {
			return result, err
		}
	}

	for index, target := range targets {
		history, ok := histories[target.ServerID]
		if !ok {
			history = model.EmptyAvailabilityHistory(observed.Unix())
		}

		entry := statuspage.PublicEntry{History: history, ID: target.ID, Name: target.Name, State: statuspage.Unknown, ArtURL: statuspage.PublicArtURL(target.ArtURL), GameAddress: target.GameAddress}

		entry.State, entry.ObservedAt = pageAvailability(target, observations[index], observed)

		if entry.State == statuspage.Online {
			entry.GameState = games[target.ServerID].View
		}

		result.Entries = append(result.Entries, entry)
	}

	result.CollectedAt = time.Now().Unix()

	return result, nil
}

func (s *StatusPageService) pageObservations(targets []repository.PublishedTarget) []runtimeStatusObservation {
	if s.Deployment == nil {
		return make([]runtimeStatusObservation, len(targets))
	}

	requests := make([]model.ResourceTarget, 0, len(targets))
	for _, target := range targets {
		requests = append(requests, model.ResourceTarget{ID: target.ServerID, SparkID: target.SparkID})
	}

	return s.Deployment.cachedRuntimeStatuses(requests)
}

func pageAvailability(target repository.PublishedTarget, observation runtimeStatusObservation, readAt time.Time) (statuspage.State, int64) {
	if target.Disabled || observation.Observed.IsZero() {
		return statuspage.Unknown, 0
	}

	if observation.ValidFor <= time.Since(readAt) {
		return statuspage.Unknown, observation.Observed.Unix()
	}

	return publicAvailability(observation.Status), observation.Observed.Unix()
}

// StatusPageCard contains only the public title, ordered display names and availability.
func (s *StatusPageService) StatusPageCard(ctx context.Context, id string) (core.LiveCard, error) {
	title, targets, err := s.Store.PublishedByID(ctx, id)
	if err != nil {
		return core.LiveCard{}, err
	}

	return s.pageCard(title, targets), nil
}

// PreviewCard renders a proposed selection using the same observations as the saved page.
func (s *StatusPageService) PreviewCard(ctx context.Context, title string, entries []statuspage.Selection) (core.LiveCard, error) {
	input := statuspage.Input{Title: title, Slug: "preview", Entries: entries}
	if err := input.Normalize(); err != nil {
		return core.LiveCard{}, err
	}

	targets, err := s.Store.DraftTargets(ctx, input.Entries)
	if err != nil {
		return core.LiveCard{}, err
	}

	return s.pageCard(input.Title, targets), nil
}

func (s *StatusPageService) pageCard(title string, targets []repository.PublishedTarget) core.LiveCard {
	now := time.Now()
	observations := s.pageObservations(targets)
	card := core.LiveCard{Name: title, StatusPage: true}

	for i, target := range targets {
		state, _ := pageAvailability(target, observations[i], now)
		card.Fields = append(card.Fields, core.CardField{Name: target.Name, Value: string(state), Available: true})
	}

	return card
}

func publicAvailability(status model.RuntimeStatus) statuspage.State {
	switch status.State {
	case model.RuntimeStateRunning:
		if status.Ready {
			return statuspage.Online
		}

		return statuspage.Offline
	case model.RuntimeStateStopped, model.RuntimeStateNotInstalled, model.RuntimeStateInstalling, model.RuntimeStateUpdating, model.RuntimeStateReinstalling, model.RuntimeStateStarting, model.RuntimeStateStopping, model.RuntimeStateRestarting, model.RuntimeStateCreated, model.RuntimeStatePaused, model.RuntimeStateExited, model.RuntimeStateDead, model.RuntimeStateMissing, model.RuntimeStateDeleting, model.RuntimeStateDeleted, model.RuntimeStateRemoving:
		return statuspage.Offline
	default:
		// Agent offline and failed/unknown observations are not proof of game downtime.
		return statuspage.Unknown
	}
}

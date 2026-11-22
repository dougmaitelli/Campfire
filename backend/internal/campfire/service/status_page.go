package service

import (
	"context"
	"time"

	"campfire/internal/campfire/database/repository"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

// StatusPageService separates administrator publication from anonymous availability.
type StatusPageService struct {
	Store      *repository.StatusPageRepository
	Deployment *DeploymentService
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

	return s.Store.Save(ctx, id, input)
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

	observations := make([]runtimeStatusObservation, len(targets))
	if s.Deployment != nil {
		requests := make([]model.ResourceTarget, 0, len(targets))
		for _, target := range targets {
			requests = append(requests, model.ResourceTarget{ID: target.ServerID, SparkID: target.SparkID})
		}

		observations = s.Deployment.cachedRuntimeStatuses(requests)
	}

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

		observation := observations[index]
		if !target.Disabled && !observation.Observed.IsZero() {
			entry.ObservedAt = observation.Observed.Unix()

			entry.State = publicAvailability(observation.Status)
			if entry.State != statuspage.Unknown {
				if observation.ValidFor <= time.Since(observed) {
					entry.State = statuspage.Unknown
				}
			}
		}

		if entry.State == statuspage.Online {
			entry.GameState = games[target.ServerID].View
		}

		result.Entries = append(result.Entries, entry)
	}

	result.CollectedAt = time.Now().Unix()

	return result, nil
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

package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

type jobCapabilityRouter struct {
	*transferRouter
	missing    string
	deliveries int
}

func (r *jobCapabilityRouter) CheckCapabilities(spark string, required ...string) error {
	available := protocol.SupportedCapabilities()
	if spark == "target" {
		available = slices.DeleteFunc(available, func(value string) bool { return value == r.missing })
	}

	return protocol.Require(available, required...)
}

func (r *jobCapabilityRouter) Deploy(ctx context.Context, spark string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	r.deliveries++
	if err := r.CheckCapabilities(spark, protocol.RequiredCapabilities(protocol.Command{Name: protocol.ConfigureServer, Deployment: &deployment})...); err != nil {
		return model.ConfigurationReceipt{}, err
	}

	return r.transferRouter.Deploy(ctx, spark, deployment)
}

func enableJobFeature(t *testing.T, f *transferFixture, capability string) {
	t.Helper()

	servers := f.service.Deployment.Servers
	if capability == protocol.CapabilityGameIntegration {
		input := f.server.ServerConfiguration
		input.GameIntegration = true

		if _, err := servers.Update(t.Context(), f.server.ID, input); err != nil {
			t.Fatal(err)
		}

		return
	}

	embers := f.service.Deployment.Embers

	ember, err := embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	ember.LogAlerts = []model.LogAlert{{Title: "Problem", Message: "Check settings", Severity: model.LogError, Any: []model.LogMatch{{Value: "fatal"}}}}
	if _, err := embers.Replace(t.Context(), ember.ID, ember); err != nil {
		t.Fatal(err)
	}

	if _, err := servers.ApplyEmber(t.Context(), f.server.ID, ember.ID, f.server.ServerConfiguration); err != nil {
		t.Fatal(err)
	}
}

func TestJobPreflightRejectsUnsupportedDeploymentsWithoutReservations(t *testing.T) {
	for _, capability := range []string{protocol.CapabilityGameIntegration, protocol.CapabilityLogAlerts} {
		for _, workflow := range []model.JobType{model.JobCopy, model.JobTransfer} {
			t.Run(capability+"/"+string(workflow), func(t *testing.T) {
				f, copies, _ := newCopyFixture(t)
				enableJobFeature(t, f, capability)
				router := &jobCapabilityRouter{transferRouter: f.router, missing: capability}
				copies.Deployment.Remote = router

				var err error
				if workflow == model.JobCopy {
					_, err = copies.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
				} else {
					_, err = f.service.Begin(t.Context(), f.server.ID, "target", f.ports)
				}

				var failure *protocol.CommandError
				if !errors.As(err, &failure) || failure.Code != protocol.ErrorUnsupportedCapability || router.deliveries != 0 {
					t.Fatal("incompatible deployment was not rejected before delivery", err, router.deliveries)
				}

				servers, err := copies.Deployment.Servers.List(t.Context())
				if err != nil || len(servers) != 1 {
					t.Fatal("preflight left a destination server", servers, err)
				}

				if err := copies.Store.Guard(t.Context(), f.server.ID); err != nil {
					t.Fatal("preflight retained a job reservation", err)
				}
			})
		}
	}
}

func TestCopyCanCancelAfterDestinationLosesDeploymentCapability(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		name := "not-delivered"
		if delivered {
			name = "delivered"
		}

		t.Run(name, func(t *testing.T) {
			f, copies, _ := newCopyFixture(t)
			enableJobFeature(t, f, protocol.CapabilityGameIntegration)
			router := &jobCapabilityRouter{transferRouter: f.router}
			copies.Deployment.Remote = router

			target, err := copies.Create(t.Context(), f.server.ID, model.ServerConfiguration{SparkID: "target"}, 1, true, true, nil)
			if err != nil {
				t.Fatal(err)
			}

			job := copyJob(t, copies, target.ID)
			claimJob(t, copies.Store.Jobs(), job.ID)

			if !delivered {
				router.missing = protocol.CapabilityGameIntegration
			}

			_, err = copies.step(t.Context(), job)
			if !delivered && err == nil || delivered && err != nil {
				t.Fatal("unexpected preparation outcome", err)
			}

			if !delivered {
				if err := copies.Store.Advance(t.Context(), job.ID, job.Phase, repository.CopyCheckpoint{Phase: model.CopyFailed}); err != nil {
					t.Fatal(err)
				}
			}

			router.missing = protocol.CapabilityGameIntegration
			before := router.deliveries

			if err := copies.Store.Change(t.Context(), job.ID, true); err != nil {
				t.Fatal(err)
			}

			runCopyUntil(t, copies, job.ID, model.CopyCancelled)

			if router.deliveries != before {
				t.Fatal("cancellation attempted to deliver an unsupported definition")
			}

			if _, err := copies.Deployment.Servers.Get(t.Context(), target.ID); !errors.Is(err, ErrServerNotFound) {
				t.Fatal("cancelled destination was retained", err)
			}

			if err := copies.Store.Guard(t.Context(), f.server.ID); err != nil {
				t.Fatal("cancellation retained the source reservation", err)
			}
		})
	}
}

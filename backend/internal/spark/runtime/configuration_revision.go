package runtime

import "campfire/internal/shared/model"

// RememberContainerConfiguration preserves the baseline for containers created
// before revision tracking existed, without consulting or changing Docker.
func (m *Manager) RememberContainerConfiguration(previous model.Deployment) error {
	e, err := m.load(previous.ServerID)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Revision != "" {
		return nil
	}

	e.Revision = model.ContainerRevision(previous)

	return m.persist(previous.ServerID, e)
}

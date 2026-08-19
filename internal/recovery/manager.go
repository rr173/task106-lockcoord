package recovery

import (
	"task106/internal/model"
	"time"
)

func NewManager(store Store, resources ResourceReader, leases LeaseReader) *Manager {
	return &Manager{store: store, resources: resources, leases: leases}
}

func (m *Manager) Start() error {
	_, err := m.Run("startup")
	return err
}

func (m *Manager) Run(scope string) (*model.RecoveryCheckpoint, error) {
	checkpoint := &model.RecoveryCheckpoint{Scope: scope, Status: "running", StartedAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}
	if err := m.store.CreateRecoveryCheckpoint(checkpoint); err != nil {
		return nil, err
	}
	issues, err := m.scanIssues()
	if err != nil {
		// Persist the failure result and the inspection event together; if the
		// event write fails the checkpoint is rolled back to "running" so the
		// next inspection resumes from a correct state. The scan error remains
		// the primary failure reported to the caller.
		finished := time.Now().UTC()
		_ = m.store.FinishRecoveryCheckpointWithEvent(checkpoint.ID, "failed", []string{err.Error()}, finished, "recovery_checkpoint", scope, "", "failed")
		return nil, err
	}
	status := "healthy"
	if len(issues) > 0 {
		status = "attention"
	}
	finished := time.Now().UTC()
	if err := m.store.FinishRecoveryCheckpointWithEvent(checkpoint.ID, status, issues, finished, "recovery_checkpoint", scope, "", status); err != nil {
		return nil, err
	}
	checkpoint.Status = status
	checkpoint.Issues = issues
	checkpoint.FinishedAt = &finished
	return checkpoint, nil
}

func (m *Manager) Get(id int64) (*model.RecoveryCheckpoint, error) {
	item, err := m.store.GetRecoveryCheckpoint(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrCheckpointNotFound
	}
	return item, nil
}

func (m *Manager) List(scope string, limit int) ([]model.RecoveryCheckpoint, error) {
	return m.store.ListRecoveryCheckpoints(scope, limit)
}

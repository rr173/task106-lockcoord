package recovery

import (
	"task106/internal/model"
	"time"
)

type Store interface {
	CreateRecoveryCheckpoint(*model.RecoveryCheckpoint) error
	FinishRecoveryCheckpointWithEvent(id int64, status string, issues []string, finished time.Time, eventType, resourcePath, holder, detail string) error
	GetRecoveryCheckpoint(int64) (*model.RecoveryCheckpoint, error)
	ListRecoveryCheckpoints(string, int) ([]model.RecoveryCheckpoint, error)
	RecordCoordinationEvent(string, string, string, string) error
}

type ResourceReader interface {
	Get(string) (*model.Resource, error)
	List(string) ([]model.Resource, error)
}

type LeaseReader interface {
	ListActiveLeases() ([]model.Lease, error)
}

type Manager struct {
	store     Store
	resources ResourceReader
	leases    LeaseReader
}

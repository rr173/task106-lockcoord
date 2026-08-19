package recovery

import (
	"errors"
	"fmt"
	"task106/internal/model"
	resourcepkg "task106/internal/resource"
)

func (m *Manager) scanIssues() ([]string, error) {
	leases, err := m.leases.ListActiveLeases()
	if err != nil {
		return nil, err
	}
	issues := make([]string, 0)
	for _, lease := range leases {
		resource, err := m.resources.Get(lease.LockName)
		if err != nil && !errors.Is(err, resourcepkg.ErrNotFound) {
			return nil, err
		}
		if errors.Is(err, resourcepkg.ErrNotFound) { resource = nil }
		if resource == nil {
			issues = append(issues, fmt.Sprintf("active lease %s has no registered resource", lease.LockName))
			continue
		}
		if resource.State == model.ResourceRetired {
			issues = append(issues, fmt.Sprintf("retired resource %s still has active lease", lease.LockName))
		}
		if lease.ExpiresAt.IsZero() {
			issues = append(issues, fmt.Sprintf("active lease %s has no expiry", lease.LockName))
		}
	}
	return issues, nil
}

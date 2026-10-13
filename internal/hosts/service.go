package hosts

// Service exposes the host store to the frontend.
type Service struct {
	store *Store
}

// NewService wraps store for binding.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// List returns all saved hosts.
func (s *Service) List() ([]Host, error) {
	return s.store.List()
}

// Save creates or updates a host. Validation failures come back as a
// ValidationError naming the offending field.
func (s *Service) Save(h Host) (Host, error) {
	return s.store.Save(h)
}

// Delete removes a host.
func (s *Service) Delete(id int64) error {
	return s.store.Delete(id)
}

// ServiceShutdown closes the database when the app quits.
func (s *Service) ServiceShutdown() error {
	return s.store.Close()
}

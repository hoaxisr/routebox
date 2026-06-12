package updates

// Service bundles the checker, updater and configured targets for the API.
type Service struct {
	Checker *Checker
	Updater *Updater
	Targets []Target
}

// Target finds a configured target by name.
func (s *Service) Target(name string) (Target, bool) {
	for _, t := range s.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}

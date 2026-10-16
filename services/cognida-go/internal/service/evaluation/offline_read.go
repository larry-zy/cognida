package evaluation

import "context"

func (s *Service) WithOfflineReadAuthorizer(fn func(context.Context, string) error) *Service {
	s.offlineReadAuthorizer = fn
	return s
}

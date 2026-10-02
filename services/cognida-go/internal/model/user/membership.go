package user

import "context"

// Membership is a user's explicit, active tenant membership. The home tenant is
// a login default, never an alternative authorization mechanism.
type Membership struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}
type MembershipRepository interface {
	ListMemberships(context.Context, int64, string, int) ([]*Membership, string, error)
	FindMembership(context.Context, int64, int64) (*Membership, error)
	AddMembership(context.Context, int64, int64, string) error
}

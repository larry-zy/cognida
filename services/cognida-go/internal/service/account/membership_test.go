package account

import (
	"cognida/internal/config"
	"cognida/internal/model/user"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type membershipUsers struct {
	user.UserRepository
	allowed map[int64]bool
}

func (r *membershipUsers) FindByID(context.Context, int64) (*user.User, error) {
	return &user.User{ID: 7, TenantID: 1, Status: 1, Username: "alice"}, nil
}
func (r *membershipUsers) FindMembership(_ context.Context, _ int64, tenantID int64) (*user.Membership, error) {
	if !r.allowed[tenantID] {
		return nil, errors.New("denied")
	}
	return &user.Membership{ID: tenantID}, nil
}
func (r *membershipUsers) ListMemberships(context.Context, int64, string, int) ([]*user.Membership, string, error) {
	return nil, "", nil
}
func (r *membershipUsers) AddMembership(context.Context, int64, int64, string) error { return nil }

type membershipTokens struct {
	user.RefreshTokenRepository
	token   *user.RefreshToken
	deletes int
}

func (r *membershipTokens) Create(_ context.Context, t *user.RefreshToken) error {
	t.ID = 11
	r.token = t
	return nil
}
func (r *membershipTokens) FindByHash(_ context.Context, h string) (*user.RefreshToken, error) {
	if r.token == nil || r.token.TokenHash != h {
		return nil, errors.New("missing")
	}
	return r.token, nil
}
func (r *membershipTokens) Delete(context.Context, int64) error {
	r.deletes++
	r.token = nil
	return nil
}
func TestSwitchTenantRefreshPreservesMembershipAndRejectsRevocation(t *testing.T) {
	users := &membershipUsers{allowed: map[int64]bool{1: true, 2: true}}
	tokens := &membershipTokens{}
	auth := &authService{userRepo: users, refreshTokenRepo: tokens, jwtConfig: &config.JWTConfig{Secret: testSecret, AccessTokenExpire: 900, RefreshTokenExpire: 3600}}
	service := &AccountService{authService: auth}
	ctx := context.Background()
	response, err := service.SwitchTenant(ctx, 7, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), response.TenantID)
	require.Equal(t, int64(2), tokens.token.TenantID)
	refreshed, err := service.RefreshToken(ctx, &RefreshTokenRequest{RefreshToken: response.RefreshToken})
	require.NoError(t, err)
	require.Equal(t, int64(2), refreshed.User.TenantID)
	require.NotEqual(t, response.RefreshToken, refreshed.RefreshToken)
	require.Equal(t, 1, tokens.deletes)
	_, err = service.RefreshToken(ctx, &RefreshTokenRequest{RefreshToken: response.RefreshToken})
	require.ErrorIs(t, err, ErrInvalidToken)
	users.allowed[2] = false
	_, err = service.RefreshToken(ctx, &RefreshTokenRequest{RefreshToken: refreshed.RefreshToken})
	require.ErrorIs(t, err, ErrTenantAccessDenied)
	_, err = service.SwitchTenant(ctx, 7, 999)
	require.ErrorIs(t, err, ErrTenantAccessDenied)
}
func TestRefreshRejectsExpiredStoredToken(t *testing.T) {
	users := &membershipUsers{allowed: map[int64]bool{1: true}}
	tokens := &membershipTokens{}
	auth := &authService{userRepo: users, refreshTokenRepo: tokens, jwtConfig: &config.JWTConfig{Secret: testSecret, AccessTokenExpire: 900, RefreshTokenExpire: 3600}}
	response, err := auth.issueForTenant(context.Background(), &user.User{ID: 7, TenantID: 1}, 1)
	require.NoError(t, err)
	tokens.token.ExpiresAt = time.Now().Add(-time.Minute)
	_, err = auth.RefreshToken(context.Background(), &RefreshTokenRequest{RefreshToken: response.RefreshToken})
	require.ErrorIs(t, err, ErrInvalidToken)
}

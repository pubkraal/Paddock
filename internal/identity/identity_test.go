package identity_test

import (
	"testing"

	"github.com/pubkraal/paddock/internal/identity"
)

func TestRole_IsAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role identity.Role
		want bool
	}{
		{identity.RolePressOfficer, true},
		{identity.RoleSeasonAdmin, true},
		{identity.RoleFinance, true},
		{identity.RoleConsumer, false},
		{identity.Role("bogus"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			t.Parallel()

			if got := tt.role.IsAdmin(); got != tt.want {
				t.Errorf("Role(%q).IsAdmin() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestUser_IsActive(t *testing.T) {
	t.Parallel()

	active := identity.User{Status: identity.StatusActive}
	if !active.IsActive() {
		t.Error("active user reported not active")
	}

	disabled := identity.User{Status: identity.StatusDisabled}
	if disabled.IsActive() {
		t.Error("disabled user reported active")
	}
}

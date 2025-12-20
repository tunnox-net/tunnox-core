package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPortMapping_IsExpired(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	tests := []struct {
		name        string
		mapping     *PortMapping
		wantExpired bool
	}{
		{
			name: "not expired",
			mapping: &PortMapping{
				ExpiresAt: &future,
			},
			wantExpired: false,
		},
		{
			name: "expired",
			mapping: &PortMapping{
				ExpiresAt: &past,
			},
			wantExpired: true,
		},
		{
			name: "no expiration time",
			mapping: &PortMapping{
				ExpiresAt: nil,
			},
			wantExpired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantExpired, tt.mapping.IsExpired())
		})
	}
}

func TestPortMapping_IsValid(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	tests := []struct {
		name      string
		mapping   *PortMapping
		wantValid bool
	}{
		{
			name: "valid mapping",
			mapping: &PortMapping{
				Status:    MappingStatusActive,
				IsRevoked: false,
				ExpiresAt: &future,
			},
			wantValid: true,
		},
		{
			name: "revoked",
			mapping: &PortMapping{
				Status:    MappingStatusActive,
				IsRevoked: true,
				ExpiresAt: &future,
			},
			wantValid: false,
		},
		{
			name: "expired",
			mapping: &PortMapping{
				Status:    MappingStatusActive,
				IsRevoked: false,
				ExpiresAt: &past,
			},
			wantValid: false,
		},
		{
			name: "inactive status",
			mapping: &PortMapping{
				Status:    MappingStatusInactive,
				IsRevoked: false,
				ExpiresAt: &future,
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantValid, tt.mapping.IsValid())
		})
	}
}

func TestPortMapping_CanBeAccessedBy(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name       string
		mapping    *PortMapping
		clientID   int64
		wantAccess bool
	}{
		{
			name: "listen client can access",
			mapping: &PortMapping{
				ListenClientID: 100,
				Status:         MappingStatusActive,
				IsRevoked:      false,
				ExpiresAt:      &future,
			},
			clientID:   100,
			wantAccess: true,
		},
		{
			name: "source client can access (backward compatibility)",
			mapping: &PortMapping{
				ListenClientID: 100,
				Status:         MappingStatusActive,
				IsRevoked:      false,
				ExpiresAt:      &future,
			},
			clientID:   100,
			wantAccess: true,
		},
		{
			name: "other client cannot access",
			mapping: &PortMapping{
				ListenClientID: 100,
				Status:         MappingStatusActive,
				IsRevoked:      false,
				ExpiresAt:      &future,
			},
			clientID:   200,
			wantAccess: false,
		},
		{
			name: "revoked mapping cannot be accessed",
			mapping: &PortMapping{
				ListenClientID: 100,
				Status:         MappingStatusActive,
				IsRevoked:      true,
				ExpiresAt:      &future,
			},
			clientID:   100,
			wantAccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantAccess, tt.mapping.CanBeAccessedBy(tt.clientID))
		})
	}
}

func TestPortMapping_CanBeRevokedBy(t *testing.T) {
	tests := []struct {
		name          string
		mapping       *PortMapping
		clientID      int64
		wantCanRevoke bool
	}{
		{
			name: "listen client can revoke",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
			},
			clientID:      100,
			wantCanRevoke: true,
		},
		{
			name: "target client can revoke",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
			},
			clientID:      200,
			wantCanRevoke: true,
		},
		{
			name: "other client cannot revoke",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
			},
			clientID:      300,
			wantCanRevoke: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCanRevoke, tt.mapping.CanBeRevokedBy(tt.clientID))
		})
	}
}

func TestPortMapping_Revoke(t *testing.T) {
	tests := []struct {
		name      string
		mapping   *PortMapping
		clientID  int64
		revokedBy string
		wantErr   bool
	}{
		{
			name: "successful revoke by listen client",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
				IsRevoked:      false,
				Status:         MappingStatusActive,
			},
			clientID:  100,
			revokedBy: "client-100",
			wantErr:   false,
		},
		{
			name: "successful revoke by target client",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
				IsRevoked:      false,
				Status:         MappingStatusActive,
			},
			clientID:  200,
			revokedBy: "client-200",
			wantErr:   false,
		},
		{
			name: "cannot revoke by other client",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
				IsRevoked:      false,
			},
			clientID:  300,
			revokedBy: "client-300",
			wantErr:   true,
		},
		{
			name: "cannot revoke already revoked mapping",
			mapping: &PortMapping{
				ListenClientID: 100,
				TargetClientID: 200,
				IsRevoked:      true,
			},
			clientID:  100,
			revokedBy: "client-100",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Revoke(tt.revokedBy, tt.clientID)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.True(t, tt.mapping.IsRevoked)
			assert.NotNil(t, tt.mapping.RevokedAt)
			assert.Equal(t, tt.revokedBy, tt.mapping.RevokedBy)
			assert.Equal(t, MappingStatusInactive, tt.mapping.Status)
		})
	}
}

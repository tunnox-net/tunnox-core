package repos

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage/postgres"
)

var _ IClientConfigRepository = (*PgClientConfigRepository)(nil)

type PgClientConfigRepository struct {
	pg *postgres.Storage
}

func NewPgClientConfigRepository(pg *postgres.Storage) *PgClientConfigRepository {
	return &PgClientConfigRepository{pg: pg}
}

func (r *PgClientConfigRepository) GetConfig(clientID int64) (*models.ClientConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.pg.QueryRow(ctx, `
		SELECT id, user_id, name, auth_code, secret_key, type, config, status,
		       node_id, ip_address, first_connected_at, last_ip_address, created_at, updated_at
		FROM client_configs WHERE id = $1
	`, clientID)

	return r.scanClientConfig(row)
}

func (r *PgClientConfigRepository) SaveConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	config.UpdatedAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	configJSON, err := json.Marshal(config.Config)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to marshal config")
	}

	secretKey := config.SecretKey
	if config.SecretKeyEncrypted != "" {
		secretKey = config.SecretKeyEncrypted
	}

	_, err = r.pg.Pool().Exec(ctx, `
		INSERT INTO client_configs (id, user_id, name, auth_code, secret_key, type, config, status,
		                            node_id, ip_address, first_connected_at, last_ip_address, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			name = EXCLUDED.name,
			auth_code = EXCLUDED.auth_code,
			secret_key = EXCLUDED.secret_key,
			type = EXCLUDED.type,
			config = EXCLUDED.config,
			status = EXCLUDED.status,
			node_id = EXCLUDED.node_id,
			ip_address = EXCLUDED.ip_address,
			first_connected_at = EXCLUDED.first_connected_at,
			last_ip_address = EXCLUDED.last_ip_address,
			updated_at = EXCLUDED.updated_at
	`, config.ID, config.UserID, config.Name, config.AuthCode, secretKey, string(config.Type),
		configJSON, "active", nil, nil, config.FirstConnectedAt, config.LastIPAddress,
		config.CreatedAt, config.UpdatedAt)

	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to save config")
	}
	return nil
}

func (r *PgClientConfigRepository) CreateConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	exists, err := r.ExistsConfig(config.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	if exists {
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "config with ID %d already exists", config.ID)
	}

	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	return r.SaveConfig(config)
}

func (r *PgClientConfigRepository) UpdateConfig(config *models.ClientConfig) error {
	if config == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "config is nil")
	}
	if err := config.Validate(); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeValidationError, "invalid config")
	}

	exists, err := r.ExistsConfig(config.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "config with ID %d not found", config.ID)
	}

	config.UpdatedAt = time.Now()
	return r.SaveConfig(config)
}

func (r *PgClientConfigRepository) DeleteConfig(clientID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.pg.Pool().Exec(ctx, `DELETE FROM client_configs WHERE id = $1`, clientID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete config")
	}
	return nil
}

func (r *PgClientConfigRepository) ListConfigs() ([]*models.ClientConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, name, auth_code, secret_key, type, config, status,
		       node_id, ip_address, first_connected_at, last_ip_address, created_at, updated_at
		FROM client_configs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query configs")
	}
	defer rows.Close()

	var configs []*models.ClientConfig
	for rows.Next() {
		config, err := r.scanClientConfigRow(rows)
		if err != nil {
			dispose.Warnf("ListConfigs: failed to scan row: %v", err)
			continue
		}
		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to iterate rows")
	}

	return configs, nil
}

func (r *PgClientConfigRepository) ListUserConfigs(userID string) ([]*models.ClientConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, name, auth_code, secret_key, type, config, status,
		       node_id, ip_address, first_connected_at, last_ip_address, created_at, updated_at
		FROM client_configs WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query user configs")
	}
	defer rows.Close()

	var cfgs []*models.ClientConfig
	for rows.Next() {
		config, err := r.scanClientConfigRow(rows)
		if err != nil {
			dispose.Warnf("ListUserConfigs: failed to scan row: %v", err)
			continue
		}
		cfgs = append(cfgs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to iterate rows")
	}

	return cfgs, nil
}

func (r *PgClientConfigRepository) AddConfigToList(_ *models.ClientConfig) error {
	return nil
}

func (r *PgClientConfigRepository) ExistsConfig(clientID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := r.pg.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_configs WHERE id = $1)`, clientID).Scan(&exists)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	return exists, nil
}

func (r *PgClientConfigRepository) scanClientConfig(row pgx.Row) (*models.ClientConfig, error) {
	var (
		id               int64
		userID           sql.NullString
		name             string
		authCode         sql.NullString
		secretKey        sql.NullString
		clientType       string
		configJSON       []byte
		status           sql.NullString
		nodeID           sql.NullString
		ipAddress        sql.NullString
		firstConnectedAt sql.NullTime
		lastIPAddress    sql.NullString
		createdAt        time.Time
		updatedAt        time.Time
	)

	err := row.Scan(&id, &userID, &name, &authCode, &secretKey, &clientType,
		&configJSON, &status, &nodeID, &ipAddress, &firstConnectedAt, &lastIPAddress,
		&createdAt, &updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, coreerrors.New(coreerrors.CodeNotFound, "config not found")
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to scan config")
	}

	var clientConfig configs.ClientConfig
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &clientConfig); err != nil {
			dispose.Warnf("scanClientConfig: failed to unmarshal config JSON: %v", err)
		}
	}

	config := &models.ClientConfig{
		ID:            id,
		UserID:        userID.String,
		Name:          name,
		AuthCode:      authCode.String,
		Type:          models.ClientType(clientType),
		Config:        clientConfig,
		LastIPAddress: lastIPAddress.String,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}

	if secretKey.Valid && secretKey.String != "" {
		config.SecretKeyEncrypted = secretKey.String
	}

	if firstConnectedAt.Valid {
		config.FirstConnectedAt = &firstConnectedAt.Time
	}

	return config, nil
}

func (r *PgClientConfigRepository) scanClientConfigRow(rows pgx.Rows) (*models.ClientConfig, error) {
	var (
		id               int64
		userID           sql.NullString
		name             string
		authCode         sql.NullString
		secretKey        sql.NullString
		clientType       string
		configJSON       []byte
		status           sql.NullString
		nodeID           sql.NullString
		ipAddress        sql.NullString
		firstConnectedAt sql.NullTime
		lastIPAddress    sql.NullString
		createdAt        time.Time
		updatedAt        time.Time
	)

	err := rows.Scan(&id, &userID, &name, &authCode, &secretKey, &clientType,
		&configJSON, &status, &nodeID, &ipAddress, &firstConnectedAt, &lastIPAddress,
		&createdAt, &updatedAt)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to scan row")
	}

	var clientConfig configs.ClientConfig
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &clientConfig); err != nil {
			dispose.Warnf("scanClientConfigRow: failed to unmarshal config JSON: %v", err)
		}
	}

	config := &models.ClientConfig{
		ID:            id,
		UserID:        userID.String,
		Name:          name,
		AuthCode:      authCode.String,
		Type:          models.ClientType(clientType),
		Config:        clientConfig,
		LastIPAddress: lastIPAddress.String,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}

	if secretKey.Valid && secretKey.String != "" {
		config.SecretKeyEncrypted = secretKey.String
	}

	if firstConnectedAt.Valid {
		config.FirstConnectedAt = &firstConnectedAt.Time
	}

	return config, nil
}

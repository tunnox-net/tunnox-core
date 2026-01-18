package repos

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage/postgres"
)

var _ IPortMappingRepository = (*PgPortMappingRepository)(nil)

type PgPortMappingRepository struct {
	pg *postgres.Storage
}

func NewPgPortMappingRepository(pg *postgres.Storage) *PgPortMappingRepository {
	return &PgPortMappingRepository{pg: pg}
}

func (r *PgPortMappingRepository) SavePortMapping(mapping *models.PortMapping) error {
	if mapping == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "mapping is nil")
	}

	mapping.UpdatedAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	configJSON, err := json.Marshal(mapping.Config)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to marshal config")
	}

	_, err = r.pg.Pool().Exec(ctx, `
		INSERT INTO port_mappings (id, user_id, listen_client_id, target_client_id, protocol,
		                           source_port, target_host, target_port, config, status, type,
		                           bytes_sent, bytes_received, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			listen_client_id = EXCLUDED.listen_client_id,
			target_client_id = EXCLUDED.target_client_id,
			protocol = EXCLUDED.protocol,
			source_port = EXCLUDED.source_port,
			target_host = EXCLUDED.target_host,
			target_port = EXCLUDED.target_port,
			config = EXCLUDED.config,
			status = EXCLUDED.status,
			type = EXCLUDED.type,
			bytes_sent = EXCLUDED.bytes_sent,
			bytes_received = EXCLUDED.bytes_received,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
	`, mapping.ID, mapping.UserID, mapping.ListenClientID, mapping.TargetClientID,
		string(mapping.Protocol), mapping.SourcePort, mapping.TargetHost, mapping.TargetPort,
		configJSON, string(mapping.Status), string(mapping.Type),
		mapping.TrafficStats.BytesSent, mapping.TrafficStats.BytesReceived,
		mapping.ExpiresAt, mapping.CreatedAt, mapping.UpdatedAt)

	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to save mapping")
	}
	return nil
}

func (r *PgPortMappingRepository) CreatePortMapping(mapping *models.PortMapping) error {
	if mapping == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "mapping is nil")
	}

	exists, err := r.existsMapping(mapping.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	if exists {
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "mapping with ID %s already exists", mapping.ID)
	}

	now := time.Now()
	mapping.CreatedAt = now
	mapping.UpdatedAt = now

	return r.SavePortMapping(mapping)
}

func (r *PgPortMappingRepository) UpdatePortMapping(mapping *models.PortMapping) error {
	if mapping == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "mapping is nil")
	}

	exists, err := r.existsMapping(mapping.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "mapping with ID %s not found", mapping.ID)
	}

	mapping.UpdatedAt = time.Now()
	return r.SavePortMapping(mapping)
}

func (r *PgPortMappingRepository) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.pg.QueryRow(ctx, `
		SELECT id, user_id, listen_client_id, target_client_id, protocol,
		       source_port, target_host, target_port, config, status, type,
		       bytes_sent, bytes_received, expires_at, created_at, updated_at
		FROM port_mappings WHERE id = $1
	`, mappingID)

	return r.scanPortMapping(row)
}

func (r *PgPortMappingRepository) GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.pg.QueryRow(ctx, `
		SELECT pm.id, pm.user_id, pm.listen_client_id, pm.target_client_id, pm.protocol,
		       pm.source_port, pm.target_host, pm.target_port, pm.config, pm.status, pm.type,
		       pm.bytes_sent, pm.bytes_received, pm.expires_at, pm.created_at, pm.updated_at
		FROM port_mappings pm
		INNER JOIN http_domain_mappings hdm ON pm.id = hdm.mapping_id
		WHERE hdm.full_domain = $1
	`, fullDomain)

	mapping, err := r.scanPortMapping(row)
	if err != nil {
		if coreerrors.IsCode(err, coreerrors.CodeNotFound) {
			return nil, coreerrors.Newf(coreerrors.CodeNotFound, "mapping not found for domain: %s", fullDomain)
		}
		return nil, err
	}
	return mapping, nil
}

func (r *PgPortMappingRepository) DeletePortMapping(mappingID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.pg.Pool().Exec(ctx, `DELETE FROM port_mappings WHERE id = $1`, mappingID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete mapping")
	}
	return nil
}

func (r *PgPortMappingRepository) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.pg.Pool().Exec(ctx, `
		UPDATE port_mappings SET status = $1, updated_at = $2 WHERE id = $3
	`, string(status), time.Now(), mappingID)

	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update status")
	}
	return nil
}

func (r *PgPortMappingRepository) UpdatePortMappingStats(mappingID string, s *stats.TrafficStats) error {
	if s == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.pg.Pool().Exec(ctx, `
		UPDATE port_mappings SET bytes_sent = $1, bytes_received = $2, updated_at = $3 WHERE id = $4
	`, s.BytesSent, s.BytesReceived, time.Now(), mappingID)

	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to update stats")
	}
	return nil
}

func (r *PgPortMappingRepository) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, listen_client_id, target_client_id, protocol,
		       source_port, target_host, target_port, config, status, type,
		       bytes_sent, bytes_received, expires_at, created_at, updated_at
		FROM port_mappings WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query mappings")
	}
	defer rows.Close()

	return r.scanPortMappingRows(rows)
}

func (r *PgPortMappingRepository) GetClientPortMappings(clientID string) ([]*models.PortMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, listen_client_id, target_client_id, protocol,
		       source_port, target_host, target_port, config, status, type,
		       bytes_sent, bytes_received, expires_at, created_at, updated_at
		FROM port_mappings 
		WHERE listen_client_id::text = $1 OR target_client_id::text = $1
		ORDER BY created_at DESC
	`, clientID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query mappings")
	}
	defer rows.Close()

	return r.scanPortMappingRows(rows)
}

func (r *PgPortMappingRepository) AddMappingToUser(_ string, _ *models.PortMapping) error {
	return nil
}

func (r *PgPortMappingRepository) AddMappingToClient(_ string, _ *models.PortMapping) error {
	return nil
}

func (r *PgPortMappingRepository) ListAllMappings() ([]*models.PortMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, listen_client_id, target_client_id, protocol,
		       source_port, target_host, target_port, config, status, type,
		       bytes_sent, bytes_received, expires_at, created_at, updated_at
		FROM port_mappings ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query mappings")
	}
	defer rows.Close()

	mappings, err := r.scanPortMappingRows(rows)
	if err != nil {
		return nil, err
	}

	return mappings, nil
}

func (r *PgPortMappingRepository) AddMappingToList(_ *models.PortMapping) error {
	return nil
}

func (r *PgPortMappingRepository) CleanupMappingIndexesByData(_ *models.PortMapping) error {
	return nil
}

func (r *PgPortMappingRepository) existsMapping(mappingID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := r.pg.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM port_mappings WHERE id = $1)`, mappingID).Scan(&exists)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	return exists, nil
}

func (r *PgPortMappingRepository) scanPortMapping(row pgx.Row) (*models.PortMapping, error) {
	var (
		id             string
		userID         sql.NullString
		listenClientID int64
		targetClientID sql.NullInt64
		protocol       string
		sourcePort     int
		targetHost     string
		targetPort     int
		configJSON     []byte
		status         string
		mappingType    string
		bytesSent      int64
		bytesReceived  int64
		expiresAt      sql.NullTime
		createdAt      time.Time
		updatedAt      time.Time
	)

	err := row.Scan(&id, &userID, &listenClientID, &targetClientID, &protocol,
		&sourcePort, &targetHost, &targetPort, &configJSON, &status, &mappingType,
		&bytesSent, &bytesReceived, &expiresAt, &createdAt, &updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, coreerrors.New(coreerrors.CodeNotFound, "mapping not found")
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to scan mapping")
	}

	var mappingConfig configs.MappingConfig
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &mappingConfig); err != nil {
			dispose.Warnf("scanPortMapping: failed to unmarshal config JSON: %v", err)
		}
	}

	mapping := &models.PortMapping{
		ID:             id,
		UserID:         userID.String,
		ListenClientID: listenClientID,
		TargetClientID: targetClientID.Int64,
		Protocol:       models.Protocol(protocol),
		SourcePort:     sourcePort,
		TargetHost:     targetHost,
		TargetPort:     targetPort,
		Config:         mappingConfig,
		Status:         models.MappingStatus(status),
		Type:           models.MappingType(mappingType),
		TrafficStats: models.TrafficStats{
			BytesSent:     bytesSent,
			BytesReceived: bytesReceived,
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	if expiresAt.Valid {
		mapping.ExpiresAt = &expiresAt.Time
	}

	return mapping, nil
}

func (r *PgPortMappingRepository) scanPortMappingRows(rows pgx.Rows) ([]*models.PortMapping, error) {
	var mappings []*models.PortMapping
	for rows.Next() {
		var (
			id             string
			userID         sql.NullString
			listenClientID int64
			targetClientID sql.NullInt64
			protocol       string
			sourcePort     int
			targetHost     string
			targetPort     int
			configJSON     []byte
			status         string
			mappingType    string
			bytesSent      int64
			bytesReceived  int64
			expiresAt      sql.NullTime
			createdAt      time.Time
			updatedAt      time.Time
		)

		err := rows.Scan(&id, &userID, &listenClientID, &targetClientID, &protocol,
			&sourcePort, &targetHost, &targetPort, &configJSON, &status, &mappingType,
			&bytesSent, &bytesReceived, &expiresAt, &createdAt, &updatedAt)
		if err != nil {
			dispose.Warnf("scanPortMappingRows: failed to scan row: %v", err)
			continue
		}

		var mappingConfig configs.MappingConfig
		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &mappingConfig); err != nil {
				dispose.Warnf("scanPortMappingRows: failed to unmarshal config JSON: %v", err)
			}
		}

		mapping := &models.PortMapping{
			ID:             id,
			UserID:         userID.String,
			ListenClientID: listenClientID,
			TargetClientID: targetClientID.Int64,
			Protocol:       models.Protocol(protocol),
			SourcePort:     sourcePort,
			TargetHost:     targetHost,
			TargetPort:     targetPort,
			Config:         mappingConfig,
			Status:         models.MappingStatus(status),
			Type:           models.MappingType(mappingType),
			TrafficStats: models.TrafficStats{
				BytesSent:     bytesSent,
				BytesReceived: bytesReceived,
			},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}

		if expiresAt.Valid {
			mapping.ExpiresAt = &expiresAt.Time
		}

		mappings = append(mappings, mapping)
	}

	if err := rows.Err(); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to iterate rows")
	}

	return mappings, nil
}

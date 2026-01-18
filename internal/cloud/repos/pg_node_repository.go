package repos

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage/postgres"
)

var _ INodeRepository = (*PgNodeRepository)(nil)

type PgNodeRepository struct {
	pg *postgres.Storage
}

func NewPgNodeRepository(pg *postgres.Storage) *PgNodeRepository {
	return &PgNodeRepository{pg: pg}
}

func (r *PgNodeRepository) SaveNode(node *models.Node) error {
	if node == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "node is nil")
	}

	node.UpdatedAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metaJSON, err := json.Marshal(node.Meta)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to marshal meta")
	}

	_, err = r.pg.Pool().Exec(ctx, `
		INSERT INTO nodes (id, name, address, meta, last_heartbeat, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			address = EXCLUDED.address,
			meta = EXCLUDED.meta,
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at = EXCLUDED.updated_at
	`, node.ID, node.Name, node.Address, metaJSON, time.Now(), node.CreatedAt, node.UpdatedAt)

	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to save node")
	}
	return nil
}

func (r *PgNodeRepository) CreateNode(node *models.Node) error {
	if node == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "node is nil")
	}

	exists, err := r.existsNode(node.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	if exists {
		return coreerrors.Newf(coreerrors.CodeAlreadyExists, "node with ID %s already exists", node.ID)
	}

	now := time.Now()
	node.CreatedAt = now
	node.UpdatedAt = now

	return r.SaveNode(node)
}

func (r *PgNodeRepository) UpdateNode(node *models.Node) error {
	if node == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "node is nil")
	}

	exists, err := r.existsNode(node.ID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	if !exists {
		return coreerrors.Newf(coreerrors.CodeNotFound, "node with ID %s not found", node.ID)
	}

	node.UpdatedAt = time.Now()
	return r.SaveNode(node)
}

func (r *PgNodeRepository) GetNode(nodeID string) (*models.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := r.pg.QueryRow(ctx, `
		SELECT id, name, address, meta, last_heartbeat, created_at, updated_at
		FROM nodes WHERE id = $1
	`, nodeID)

	return r.scanNode(row)
}

func (r *PgNodeRepository) DeleteNode(nodeID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.pg.Pool().Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to delete node")
	}
	return nil
}

func (r *PgNodeRepository) ListNodes() ([]*models.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := r.pg.Query(ctx, `
		SELECT id, name, address, meta, last_heartbeat, created_at, updated_at
		FROM nodes ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to query nodes")
	}
	defer rows.Close()

	var nodes []*models.Node
	for rows.Next() {
		node, err := r.scanNodeRow(rows)
		if err != nil {
			dispose.Warnf("ListNodes: failed to scan row: %v", err)
			continue
		}
		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to iterate rows")
	}

	return nodes, nil
}

func (r *PgNodeRepository) AddNodeToList(_ *models.Node) error {
	return nil
}

func (r *PgNodeRepository) existsNode(nodeID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := r.pg.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id = $1)`, nodeID).Scan(&exists)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to check existence")
	}
	return exists, nil
}

func (r *PgNodeRepository) scanNode(row pgx.Row) (*models.Node, error) {
	var (
		id            string
		name          string
		address       string
		metaJSON      []byte
		lastHeartbeat sql.NullTime
		createdAt     time.Time
		updatedAt     time.Time
	)

	err := row.Scan(&id, &name, &address, &metaJSON, &lastHeartbeat, &createdAt, &updatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, coreerrors.New(coreerrors.CodeNotFound, "node not found")
		}
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to scan node")
	}

	var meta map[string]string
	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &meta); err != nil {
			dispose.Warnf("scanNode: failed to unmarshal meta JSON: %v", err)
		}
	}

	return &models.Node{
		ID:        id,
		Name:      name,
		Address:   address,
		Meta:      meta,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func (r *PgNodeRepository) scanNodeRow(rows pgx.Rows) (*models.Node, error) {
	var (
		id            string
		name          string
		address       string
		metaJSON      []byte
		lastHeartbeat sql.NullTime
		createdAt     time.Time
		updatedAt     time.Time
	)

	err := rows.Scan(&id, &name, &address, &metaJSON, &lastHeartbeat, &createdAt, &updatedAt)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to scan row")
	}

	var meta map[string]string
	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &meta); err != nil {
			dispose.Warnf("scanNodeRow: failed to unmarshal meta JSON: %v", err)
		}
	}

	return &models.Node{
		ID:        id,
		Name:      name,
		Address:   address,
		Meta:      meta,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

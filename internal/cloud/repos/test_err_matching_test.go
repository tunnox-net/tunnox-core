package repos

import (
	"context"
	"errors"
	"testing"
	"tunnox-core/internal/core/storage"
)

func TestErrorMatching(t *testing.T) {
	factory := storage.NewStorageFactory(context.TODO())
	memStorage, _ := factory.CreateStorage(storage.StorageTypeMemory, nil)
	repo := NewRepository(memStorage)
	connCodeRepo := NewConnectionCodeRepository(repo)

	// 查询不存在的连接码
	_, err := connCodeRepo.GetByCode("nonexistent")
	t.Logf("Error: %v", err)
	t.Logf("Is ErrNotFound: %v", errors.Is(err, ErrNotFound))

	if !errors.Is(err, ErrNotFound) {
		t.Fatal("Expected errors.Is(err, ErrNotFound) to be true")
	}
}

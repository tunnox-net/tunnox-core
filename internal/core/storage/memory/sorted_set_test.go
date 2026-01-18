package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortedSet_ZAddAndZScore(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	err := storage.ZAdd(key, "member1", 1.0)
	assert.NoError(t, err)

	score, exists, err := storage.ZScore(key, "member1")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, 1.0, score)
}

func TestSortedSet_ZAddUpdateScore(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	storage.ZAdd(key, "member1", 1.0)
	storage.ZAdd(key, "member1", 5.0)

	score, exists, _ := storage.ZScore(key, "member1")
	assert.True(t, exists)
	assert.Equal(t, 5.0, score)
}

func TestSortedSet_ZRem(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	storage.ZAdd(key, "member1", 1.0)
	storage.ZAdd(key, "member2", 2.0)

	err := storage.ZRem(key, "member1")
	assert.NoError(t, err)

	_, exists, _ := storage.ZScore(key, "member1")
	assert.False(t, exists)

	_, exists, _ = storage.ZScore(key, "member2")
	assert.True(t, exists)
}

func TestSortedSet_ZRangeByScore(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	storage.ZAdd(key, "a", 1.0)
	storage.ZAdd(key, "b", 2.0)
	storage.ZAdd(key, "c", 3.0)
	storage.ZAdd(key, "d", 4.0)

	members, err := storage.ZRangeByScore(key, 2.0, 3.0)
	assert.NoError(t, err)
	assert.Len(t, members, 2)
	assert.Contains(t, members, "b")
	assert.Contains(t, members, "c")
}

func TestSortedSet_ZRemRangeByScore(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	storage.ZAdd(key, "a", 1.0)
	storage.ZAdd(key, "b", 2.0)
	storage.ZAdd(key, "c", 3.0)
	storage.ZAdd(key, "d", 4.0)

	removed, err := storage.ZRemRangeByScore(key, 0, 2.5)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), removed)

	count, _ := storage.ZCard(key)
	assert.Equal(t, int64(2), count)
}

func TestSortedSet_ZCard(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	count, err := storage.ZCard(key)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	storage.ZAdd(key, int64(12345678), 1.0)
	storage.ZAdd(key, int64(87654321), 2.0)

	count, _ = storage.ZCard(key)
	assert.Equal(t, int64(2), count)
}

func TestSortedSet_Int64Member(t *testing.T) {
	storage := New(context.Background())
	defer storage.Close()

	key := "test:zset"
	clientID := int64(12345678)

	err := storage.ZAdd(key, clientID, 100.0)
	assert.NoError(t, err)

	score, exists, err := storage.ZScore(key, clientID)
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, 100.0, score)

	err = storage.ZRem(key, clientID)
	assert.NoError(t, err)

	_, exists, _ = storage.ZScore(key, clientID)
	assert.False(t, exists)
}

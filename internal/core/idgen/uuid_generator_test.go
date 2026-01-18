package idgen

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUUIDGenerator_Generate(t *testing.T) {
	gen := NewUUIDGenerator(PrefixConnectionID)

	id, err := gen.Generate()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(id, PrefixConnectionID))
	assert.Len(t, id, len(PrefixConnectionID)+36) // prefix + UUID format
}

func TestUUIDGenerator_GenerateUnique(t *testing.T) {
	gen := NewUUIDGenerator(PrefixTunnelID)

	seen := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id, err := gen.Generate()
		assert.NoError(t, err)
		assert.False(t, seen[id], "duplicate ID generated: %s", id)
		seen[id] = true
	}
}

func TestUUIDGenerator_Concurrent(t *testing.T) {
	gen := NewUUIDGenerator(PrefixPortMappingInstanceID)

	var wg sync.WaitGroup
	ids := make(chan string, 1000)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				id, err := gen.Generate()
				assert.NoError(t, err)
				ids <- id
			}
		}()
	}

	wg.Wait()
	close(ids)

	seen := make(map[string]bool)
	for id := range ids {
		assert.False(t, seen[id], "duplicate ID in concurrent generation: %s", id)
		seen[id] = true
	}
}

func TestUUIDGenerator_NoRedisTracking(t *testing.T) {
	gen := NewUUIDGenerator("test_")

	id, _ := gen.Generate()

	used, err := gen.IsUsed(id)
	assert.NoError(t, err)
	assert.False(t, used) // UUID generator never reports used

	assert.Equal(t, 0, gen.GetUsedCount())

	assert.NoError(t, gen.Release(id))
	assert.NoError(t, gen.Close())
}

func BenchmarkUUIDGenerator_Generate(b *testing.B) {
	gen := NewUUIDGenerator(PrefixConnectionID)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = gen.Generate()
	}
}

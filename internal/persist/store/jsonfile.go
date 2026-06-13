package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/0xc0de1ab/muhan/internal/persist/jsonstore"
	worldload "github.com/0xc0de1ab/muhan/internal/world/load"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// JSONFileStore persists runtime world state as one atomic JSON snapshot.
type JSONFileStore struct {
	mu    sync.RWMutex
	path  string
	world *worldload.World
}

var _ Store = (*JSONFileStore)(nil)

func NewJSONFileStore(path string) (*JSONFileStore, error) {
	if path == "" {
		return nil, invalidf("empty JSON file store path")
	}

	world, err := loadJSONFileWorld(path)
	if err != nil {
		return nil, err
	}
	return &JSONFileStore{
		path:  path,
		world: cloneJSONFileWorld(world),
	}, nil
}

func (s *JSONFileStore) LoadBootstrap(ctx context.Context) (*worldload.World, error) {
	if s == nil {
		return nil, invalidf("nil JSON file store")
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	return cloneJSONFileWorld(s.world), nil
}

func (s *JSONFileStore) Save(ctx context.Context, changes ChangeSet) error {
	return s.update(ctx, func(next *worldload.World) error {
		return applyChangeSet(next, changes)
	})
}

func (s *JSONFileStore) MovePlayer(ctx context.Context, id model.PlayerID, roomID model.RoomID) error {
	return s.update(ctx, func(next *worldload.World) error {
		return movePlayer(next, id, roomID)
	})
}

func (s *JSONFileStore) MoveCreature(ctx context.Context, id model.CreatureID, roomID model.RoomID) error {
	return s.update(ctx, func(next *worldload.World) error {
		return moveCreature(next, id, roomID)
	})
}

func (s *JSONFileStore) MoveObject(ctx context.Context, id model.ObjectInstanceID, location model.ObjectLocation) error {
	return s.update(ctx, func(next *worldload.World) error {
		return moveObject(next, id, location)
	})
}

func (s *JSONFileStore) update(ctx context.Context, apply func(*worldload.World) error) error {
	if s == nil {
		return invalidf("nil JSON file store")
	}
	if err := checkContext(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkContext(ctx); err != nil {
		return err
	}

	next := cloneJSONFileWorld(s.world)
	if err := apply(next); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := saveJSONFileWorld(s.path, next); err != nil {
		return err
	}
	s.world = next
	return nil
}

func loadJSONFileWorld(path string) (*worldload.World, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return worldload.NewWorld(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read JSON world %q: %w", path, err)
	}

	var decoded worldload.World
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, invalidf("decode JSON world %q: %v", path, err)
	}

	world := cloneJSONFileWorld(&decoded)
	if err := validateWorld(world); err != nil {
		return nil, err
	}
	return world, nil
}

func saveJSONFileWorld(path string, world *worldload.World) error {
	if err := jsonstore.WriteJSON(path, cloneJSONFileWorld(world)); err != nil {
		return fmt.Errorf("write JSON world %q: %w", path, err)
	}
	return nil
}

func cloneJSONFileWorld(in *worldload.World) *worldload.World {
	out := cloneWorld(in)
	if in == nil {
		return out
	}
	out.MarriageInvites = cloneMarriageInvites(in.MarriageInvites)
	return out
}

func cloneMarriageInvites(in map[model.SpecialID][]string) map[model.SpecialID][]string {
	if in == nil {
		return nil
	}
	out := make(map[model.SpecialID][]string, len(in))
	for id, names := range in {
		out[id] = append([]string(nil), names...)
	}
	return out
}

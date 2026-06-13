package load

import (
	"fmt"
	"sort"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

type World struct {
	Rooms            map[model.RoomID]model.Room                     `json:"rooms"`
	Players          map[model.PlayerID]model.Player                 `json:"players"`
	Creatures        map[model.CreatureID]model.Creature             `json:"creatures"`
	Families         map[int]model.Family                            `json:"families,omitempty"`
	Banks            map[model.BankID]model.BankAccount              `json:"banks"`
	Objects          map[model.ObjectInstanceID]model.ObjectInstance `json:"objects"`
	ObjectPrototypes map[model.PrototypeID]model.ObjectPrototype     `json:"objectPrototypes"`
	MarriageInvites  map[model.SpecialID][]string                    `json:"marriageInvites,omitempty"`
}

type Finding struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	ID      string `json:"id,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Message string `json:"message"`
}

type RefReport struct {
	Warnings []Finding `json:"warnings,omitempty"`
	Errors   []Finding `json:"errors,omitempty"`
}

func NewWorld() *World {
	w := &World{}
	w.ensureMaps()
	return w
}

func (w *World) AddRoom(room model.Room) error {
	w.ensureMaps()
	if err := room.Validate(); err != nil {
		return fmt.Errorf("validate room %q: %w", room.ID, err)
	}
	if _, exists := w.Rooms[room.ID]; exists {
		return fmt.Errorf("duplicate room id %q", room.ID)
	}
	w.Rooms[room.ID] = room
	return nil
}

func (w *World) AddPlayer(player model.Player) error {
	w.ensureMaps()
	if err := player.Validate(); err != nil {
		return fmt.Errorf("validate player %q: %w", player.ID, err)
	}
	if _, exists := w.Players[player.ID]; exists {
		return fmt.Errorf("duplicate player id %q", player.ID)
	}
	w.Players[player.ID] = player
	return nil
}

func (w *World) AddCreature(creature model.Creature) error {
	w.ensureMaps()
	if err := creature.Validate(); err != nil {
		return fmt.Errorf("validate creature %q: %w", creature.ID, err)
	}
	if _, exists := w.Creatures[creature.ID]; exists {
		return fmt.Errorf("duplicate creature id %q", creature.ID)
	}
	w.Creatures[creature.ID] = creature
	return nil
}

func (w *World) AddFamily(family model.Family) error {
	w.ensureMaps()
	if err := family.Validate(); err != nil {
		return fmt.Errorf("validate family %d: %w", family.ID, err)
	}
	if _, exists := w.Families[family.ID]; exists {
		return fmt.Errorf("duplicate family id %d", family.ID)
	}
	w.Families[family.ID] = family
	return nil
}

func (w *World) AddBank(account model.BankAccount) error {
	w.ensureMaps()
	if err := account.Validate(); err != nil {
		return fmt.Errorf("validate bank %q: %w", account.ID, err)
	}
	if _, exists := w.Banks[account.ID]; exists {
		return fmt.Errorf("duplicate bank id %q", account.ID)
	}
	w.Banks[account.ID] = account
	return nil
}

func (w *World) AddObjectInstance(object model.ObjectInstance) error {
	w.ensureMaps()
	if err := object.Validate(); err != nil {
		return fmt.Errorf("validate object instance %q: %w", object.ID, err)
	}
	if _, exists := w.Objects[object.ID]; exists {
		return fmt.Errorf("duplicate object instance id %q", object.ID)
	}
	w.Objects[object.ID] = object
	return nil
}

func (w *World) AddObjectPrototype(proto model.ObjectPrototype) error {
	w.ensureMaps()
	if err := proto.Validate(); err != nil {
		return fmt.Errorf("validate object prototype %q: %w", proto.ID, err)
	}
	if _, exists := w.ObjectPrototypes[proto.ID]; exists {
		return fmt.Errorf("duplicate object prototype id %q", proto.ID)
	}
	w.ObjectPrototypes[proto.ID] = proto
	return nil
}

func (w *World) ValidateRefs() RefReport {
	w.ensureMaps()

	var report RefReport
	for _, id := range sortedRoomIDs(w.Rooms) {
		room := w.Rooms[id]
		for _, exit := range room.Exits {
			if exit.ToRoomID.IsZero() {
				continue
			}
			if _, ok := w.Rooms[exit.ToRoomID]; !ok {
				report.warn("missing_room_ref", string(room.ID), string(exit.ToRoomID),
					fmt.Sprintf("room %q exit %q references missing room %q", room.ID, exit.Name, exit.ToRoomID))
			}
		}
		for _, creatureID := range room.CreatureIDs {
			if creatureID.IsZero() {
				continue
			}
			if _, ok := w.Creatures[creatureID]; !ok {
				report.warn("missing_creature_ref", string(room.ID), string(creatureID),
					fmt.Sprintf("room %q references missing creature %q", room.ID, creatureID))
			}
		}
		for _, playerID := range room.PlayerIDs {
			if playerID.IsZero() {
				continue
			}
			if _, ok := w.Players[playerID]; !ok {
				report.warn("missing_player_ref", string(room.ID), string(playerID),
					fmt.Sprintf("room %q references missing player %q", room.ID, playerID))
			}
		}
		for _, objectID := range room.Objects.ObjectIDs {
			object, ok := w.Objects[objectID]
			if !ok {
				report.warn("missing_object_ref", string(room.ID), string(objectID),
					fmt.Sprintf("room %q references missing object %q", room.ID, objectID))
				continue
			}
			if object.Location.RoomID != room.ID {
				report.warn("object_location_mismatch", string(room.ID), string(objectID),
					fmt.Sprintf("room %q references object %q located at %q", room.ID, objectID, object.Location.RoomID))
			}
		}
	}

	for _, id := range sortedPlayerIDs(w.Players) {
		player := w.Players[id]
		if !player.RoomID.IsZero() {
			if _, ok := w.Rooms[player.RoomID]; !ok {
				report.warn("missing_room_ref", string(player.ID), string(player.RoomID),
					fmt.Sprintf("player %q references missing room %q", player.ID, player.RoomID))
			}
		}
		if !player.CreatureID.IsZero() {
			creature, ok := w.Creatures[player.CreatureID]
			if !ok {
				report.warn("missing_creature_ref", string(player.ID), string(player.CreatureID),
					fmt.Sprintf("player %q references missing creature %q", player.ID, player.CreatureID))
				continue
			}
			if !creature.PlayerID.IsZero() && creature.PlayerID != player.ID {
				report.warn("player_creature_mismatch", string(player.ID), string(player.CreatureID),
					fmt.Sprintf("player %q references creature %q owned by player %q", player.ID, player.CreatureID, creature.PlayerID))
			}
		}
	}

	for _, id := range sortedCreatureIDs(w.Creatures) {
		creature := w.Creatures[id]
		if !creature.RoomID.IsZero() {
			if _, ok := w.Rooms[creature.RoomID]; !ok {
				report.warn("missing_room_ref", string(creature.ID), string(creature.RoomID),
					fmt.Sprintf("creature %q references missing room %q", creature.ID, creature.RoomID))
			}
		}
		if !creature.PlayerID.IsZero() {
			player, ok := w.Players[creature.PlayerID]
			if !ok {
				report.warn("missing_player_ref", string(creature.ID), string(creature.PlayerID),
					fmt.Sprintf("creature %q references missing player %q", creature.ID, creature.PlayerID))
				continue
			}
			if !player.CreatureID.IsZero() && player.CreatureID != creature.ID {
				report.warn("player_creature_mismatch", string(creature.ID), string(creature.PlayerID),
					fmt.Sprintf("creature %q references player %q whose creature is %q", creature.ID, creature.PlayerID, player.CreatureID))
			}
		}
		for _, objectID := range creature.Inventory.ObjectIDs {
			object, ok := w.Objects[objectID]
			if !ok {
				report.warn("missing_object_ref", string(creature.ID), string(objectID),
					fmt.Sprintf("creature %q references missing object %q", creature.ID, objectID))
				continue
			}
			if object.Location.CreatureID != creature.ID {
				report.warn("object_location_mismatch", string(creature.ID), string(objectID),
					fmt.Sprintf("creature %q references object %q located at creature %q", creature.ID, objectID, object.Location.CreatureID))
			}
		}
	}

	for _, id := range sortedBankIDs(w.Banks) {
		bank := w.Banks[id]
		if !bank.OwnerPlayerID.IsZero() {
			if _, ok := w.Players[bank.OwnerPlayerID]; !ok {
				report.warn("missing_player_ref", string(bank.ID), string(bank.OwnerPlayerID),
					fmt.Sprintf("bank %q references missing player %q", bank.ID, bank.OwnerPlayerID))
			}
		}
		for _, objectID := range bank.Objects.ObjectIDs {
			object, ok := w.Objects[objectID]
			if !ok {
				report.warn("missing_object_ref", string(bank.ID), string(objectID),
					fmt.Sprintf("bank %q references missing object %q", bank.ID, objectID))
				continue
			}
			if object.Location.BankID != bank.ID {
				report.warn("object_location_mismatch", string(bank.ID), string(objectID),
					fmt.Sprintf("bank %q references object %q located at bank %q", bank.ID, objectID, object.Location.BankID))
			}
		}
	}

	for _, id := range sortedObjectIDs(w.Objects) {
		object := w.Objects[id]
		if !object.PrototypeID.IsZero() {
			if _, ok := w.ObjectPrototypes[object.PrototypeID]; !ok {
				report.warn("missing_object_prototype_ref", string(object.ID), string(object.PrototypeID),
					fmt.Sprintf("object %q references missing prototype %q", object.ID, object.PrototypeID))
			}
		}
		switch {
		case !object.Location.RoomID.IsZero():
			if _, ok := w.Rooms[object.Location.RoomID]; !ok {
				report.warn("missing_room_ref", string(object.ID), string(object.Location.RoomID),
					fmt.Sprintf("object %q references missing room %q", object.ID, object.Location.RoomID))
			}
		case !object.Location.CreatureID.IsZero():
			if _, ok := w.Creatures[object.Location.CreatureID]; !ok {
				report.warn("missing_creature_ref", string(object.ID), string(object.Location.CreatureID),
					fmt.Sprintf("object %q references missing creature %q", object.ID, object.Location.CreatureID))
			}
		case !object.Location.BankID.IsZero():
			if _, ok := w.Banks[object.Location.BankID]; !ok {
				report.warn("missing_bank_ref", string(object.ID), string(object.Location.BankID),
					fmt.Sprintf("object %q references missing bank %q", object.ID, object.Location.BankID))
			}
		case !object.Location.ContainerID.IsZero():
			if _, ok := w.Objects[object.Location.ContainerID]; !ok {
				report.warn("missing_object_ref", string(object.ID), string(object.Location.ContainerID),
					fmt.Sprintf("object %q references missing container %q", object.ID, object.Location.ContainerID))
			}
		}
		for _, childID := range object.Contents.ObjectIDs {
			child, ok := w.Objects[childID]
			if !ok {
				report.warn("missing_object_ref", string(object.ID), string(childID),
					fmt.Sprintf("object %q references missing contained object %q", object.ID, childID))
				continue
			}
			if child.Location.ContainerID != object.ID {
				report.warn("object_location_mismatch", string(object.ID), string(childID),
					fmt.Sprintf("object %q references child %q located in container %q", object.ID, childID, child.Location.ContainerID))
			}
		}
	}

	return report
}

func (w *World) ensureMaps() {
	if w.Rooms == nil {
		w.Rooms = map[model.RoomID]model.Room{}
	}
	if w.Players == nil {
		w.Players = map[model.PlayerID]model.Player{}
	}
	if w.Creatures == nil {
		w.Creatures = map[model.CreatureID]model.Creature{}
	}
	if w.Families == nil {
		w.Families = map[int]model.Family{}
	}
	if w.Banks == nil {
		w.Banks = map[model.BankID]model.BankAccount{}
	}
	if w.Objects == nil {
		w.Objects = map[model.ObjectInstanceID]model.ObjectInstance{}
	}
	if w.ObjectPrototypes == nil {
		w.ObjectPrototypes = map[model.PrototypeID]model.ObjectPrototype{}
	}
	if w.MarriageInvites == nil {
		w.MarriageInvites = map[model.SpecialID][]string{}
	}
}

func (r *RefReport) warn(kind, id, ref, message string) {
	r.Warnings = append(r.Warnings, Finding{
		Kind:    kind,
		ID:      id,
		Ref:     ref,
		Message: message,
	})
}

func sortedRoomIDs(m map[model.RoomID]model.Room) []model.RoomID {
	ids := make([]model.RoomID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedPlayerIDs(m map[model.PlayerID]model.Player) []model.PlayerID {
	ids := make([]model.PlayerID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedCreatureIDs(m map[model.CreatureID]model.Creature) []model.CreatureID {
	ids := make([]model.CreatureID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedBankIDs(m map[model.BankID]model.BankAccount) []model.BankID {
	ids := make([]model.BankID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortedObjectIDs(m map[model.ObjectInstanceID]model.ObjectInstance) []model.ObjectInstanceID {
	ids := make([]model.ObjectInstanceID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

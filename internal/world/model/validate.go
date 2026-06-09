package model

import "fmt"

func (r Room) Validate() error {
	if r.ID.IsZero() {
		return fmt.Errorf("room id is required")
	}
	if r.DisplayName == "" {
		return fmt.Errorf("room display name is required")
	}
	for i, exit := range r.Exits {
		if exit.Name == "" {
			return fmt.Errorf("room exit %d name is required", i)
		}
		if exit.ToRoomID.IsZero() {
			return fmt.Errorf("room exit %d target room id is required", i)
		}
	}
	if err := r.Objects.Validate(); err != nil {
		return fmt.Errorf("room objects: %w", err)
	}
	return nil
}

func (c Creature) Validate() error {
	if c.ID.IsZero() {
		return fmt.Errorf("creature id is required")
	}
	if c.DisplayName == "" {
		return fmt.Errorf("creature display name is required")
	}
	if c.Kind == "" {
		return fmt.Errorf("creature kind is required")
	}
	if c.Kind == CreatureKindPlayer && c.PlayerID.IsZero() {
		return fmt.Errorf("player creature must reference a player id")
	}
	if err := c.Inventory.Validate(); err != nil {
		return fmt.Errorf("creature inventory: %w", err)
	}
	return nil
}

func (p Player) Validate() error {
	if p.ID.IsZero() {
		return fmt.Errorf("player id is required")
	}
	if p.DisplayName == "" {
		return fmt.Errorf("player display name is required")
	}
	return nil
}

func (f Family) Validate() error {
	if f.ID < 0 {
		return fmt.Errorf("family id cannot be negative")
	}
	if f.Slot < 0 {
		return fmt.Errorf("family slot cannot be negative")
	}
	if f.DisplayName == "" {
		return fmt.Errorf("family display name is required")
	}
	if f.JoinSubsidy < 0 {
		return fmt.Errorf("family join subsidy cannot be negative")
	}
	for i, member := range f.Members {
		if member.DisplayName == "" {
			return fmt.Errorf("family member %d display name is required", i)
		}
		if member.Class < 0 {
			return fmt.Errorf("family member %d class cannot be negative", i)
		}
	}
	return nil
}

func (b BankAccount) Validate() error {
	if b.ID.IsZero() {
		return fmt.Errorf("bank id is required")
	}
	if b.Kind == "" {
		return fmt.Errorf("bank kind is required")
	}
	if b.OwnerName == "" {
		return fmt.Errorf("bank owner name is required")
	}
	if err := b.Objects.Validate(); err != nil {
		return fmt.Errorf("bank objects: %w", err)
	}
	return nil
}

func (p ObjectPrototype) Validate() error {
	if p.ID.IsZero() {
		return fmt.Errorf("object prototype id is required")
	}
	if p.DisplayName == "" {
		return fmt.Errorf("object prototype display name is required")
	}
	return nil
}

func (o ObjectInstance) Validate() error {
	if o.ID.IsZero() {
		return fmt.Errorf("object instance id is required")
	}
	if o.PrototypeID.IsZero() {
		return fmt.Errorf("object instance prototype id is required")
	}
	if o.Quantity < 0 {
		return fmt.Errorf("object instance quantity cannot be negative")
	}
	if err := o.Location.Validate(); err != nil {
		return fmt.Errorf("object instance location: %w", err)
	}
	if o.Location.ContainerID == o.ID {
		return fmt.Errorf("object instance cannot be located inside itself")
	}
	if err := o.Contents.Validate(); err != nil {
		return fmt.Errorf("object instance contents: %w", err)
	}
	for _, childID := range o.Contents.ObjectIDs {
		if childID == o.ID {
			return fmt.Errorf("object instance cannot contain itself")
		}
	}
	return nil
}

func (l ObjectLocation) Validate() error {
	holders := 0
	if !l.RoomID.IsZero() {
		holders++
	}
	if !l.CreatureID.IsZero() {
		holders++
	}
	if !l.BankID.IsZero() {
		holders++
	}
	if !l.ContainerID.IsZero() {
		holders++
	}
	if holders != 1 {
		return fmt.Errorf("exactly one object holder is required, got %d", holders)
	}
	return nil
}

func (p BoardPost) Validate() error {
	if p.ID.IsZero() {
		return fmt.Errorf("board post id is required")
	}
	if p.BoardID.IsZero() {
		return fmt.Errorf("board id is required")
	}
	return nil
}

func (l ObjectRefList) Validate() error {
	seen := make(map[ObjectInstanceID]struct{}, len(l.ObjectIDs))
	for i, id := range l.ObjectIDs {
		if id.IsZero() {
			return fmt.Errorf("object id %d is empty", i)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("object id %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

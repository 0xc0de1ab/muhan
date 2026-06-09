package cbin

import "fmt"

// TreeWarning records non-fatal decode issues found while building a tree.
type TreeWarning struct {
	Offset  int
	Field   string
	Message string
}

// ObjectNode is an offset-aware decoded object tree node.
type ObjectNode struct {
	Offset   int
	Record   ObjectRecord
	Children []ObjectNode
	Stats    Stats
	Warnings []TreeWarning
}

// CreatureNode is an offset-aware decoded creature tree node.
type CreatureNode struct {
	Offset    int
	Record    CreatureRecord
	Inventory []ObjectNode
	Stats     Stats
	Warnings  []TreeWarning
}

// ExitNode is an offset-aware decoded room exit node.
type ExitNode struct {
	Offset   int
	Record   ExitRecord
	Warnings []TreeWarning
}

// RoomDescriptionNode is an offset-aware decoded room description payload.
type RoomDescriptionNode struct {
	Index        int
	LengthOffset int
	Offset       int
	Size         int
	Text         TextField
}

// RoomNode is an offset-aware decoded room tree node.
type RoomNode struct {
	Offset       int
	Record       RoomRecord
	Exits        []ExitNode
	Creatures    []CreatureNode
	Objects      []ObjectNode
	Descriptions [3]RoomDescriptionNode
	Stats        Stats
	Warnings     []TreeWarning
}

// DecodeObjectTree validates and decodes a legacy object tree.
func DecodeObjectTree(data []byte, allowTrailing bool) (ObjectNode, error) {
	var (
		stats Stats
		err   error
	)
	if allowTrailing {
		stats, err = DecodeObjectFileAllowTrailing(data)
	} else {
		stats, err = DecodeObjectFile(data)
	}
	if err != nil {
		return ObjectNode{}, err
	}

	cur := NewCursor(data)
	node, err := readObjectNode(cur, 1)
	if err != nil {
		return ObjectNode{}, err
	}
	node.Stats = stats
	return node, nil
}

// DecodeCreatureTree validates and decodes a legacy creature tree.
func DecodeCreatureTree(data []byte) (CreatureNode, error) {
	stats, err := DecodeCreatureFile(data)
	if err != nil {
		return CreatureNode{}, err
	}

	cur := NewCursor(data)
	node, err := readCreatureNode(cur, 1)
	if err != nil {
		return CreatureNode{}, err
	}
	node.Stats = stats
	return node, nil
}

// DecodeRoomTree validates and decodes a legacy room tree.
func DecodeRoomTree(data []byte) (RoomNode, error) {
	stats, err := DecodeRoomFile(data)
	if err != nil {
		return RoomNode{}, err
	}

	cur := NewCursor(data)
	node, err := readRoomNode(cur, 1)
	if err != nil {
		return RoomNode{}, err
	}
	node.Stats = stats
	return node, nil
}

func readObjectNode(cur *Cursor, depth int) (ObjectNode, error) {
	if depth > MaxRecursionDepth {
		return ObjectNode{}, fmt.Errorf("object recursion depth exceeds %d", MaxRecursionDepth)
	}

	offset := cur.Offset()
	rec, err := DecodeObjectRecord(cur.data[offset:])
	if err != nil {
		return ObjectNode{}, fmt.Errorf("object record at offset %d: %w", offset, err)
	}
	if err := cur.skip(ObjectSize); err != nil {
		return ObjectNode{}, fmt.Errorf("object record at offset %d: %w", offset, err)
	}

	countOffset := cur.Offset()
	count, err := cur.int32()
	if err != nil {
		return ObjectNode{}, fmt.Errorf("object child count: %w", err)
	}
	if count < 0 || count > MaxObjectChildren {
		return ObjectNode{}, fmt.Errorf("invalid object child count %d at offset %d", count, countOffset)
	}

	node := ObjectNode{
		Offset:   offset,
		Record:   rec,
		Children: make([]ObjectNode, 0, count),
		Stats:    Stats{Objects: 1, MaxDepth: 1},
		Warnings: objectWarnings(offset, rec),
	}
	for i := 0; i < count; i++ {
		childOffset := cur.Offset()
		child, err := readObjectNode(cur, depth+1)
		if err != nil {
			return ObjectNode{}, fmt.Errorf("object child %d/%d at offset %d: %w", i+1, count, childOffset, err)
		}
		node.Children = append(node.Children, child)
		mergeChildStats(&node.Stats, child.Stats, 1)
		node.Warnings = append(node.Warnings, child.Warnings...)
	}
	return node, nil
}

func readCreatureNode(cur *Cursor, depth int) (CreatureNode, error) {
	if depth > MaxRecursionDepth {
		return CreatureNode{}, fmt.Errorf("creature recursion depth exceeds %d", MaxRecursionDepth)
	}

	offset := cur.Offset()
	rec, err := DecodeCreatureRecord(cur.data[offset:])
	if err != nil {
		return CreatureNode{}, fmt.Errorf("creature record at offset %d: %w", offset, err)
	}
	if err := cur.skip(CreatureSize); err != nil {
		return CreatureNode{}, fmt.Errorf("creature record at offset %d: %w", offset, err)
	}

	countOffset := cur.Offset()
	count, err := cur.int32()
	if err != nil {
		return CreatureNode{}, fmt.Errorf("creature inventory count: %w", err)
	}
	if count < 0 || count > MaxCreatureItems {
		return CreatureNode{}, fmt.Errorf("invalid creature inventory count %d at offset %d", count, countOffset)
	}

	node := CreatureNode{
		Offset:    offset,
		Record:    rec,
		Inventory: make([]ObjectNode, 0, count),
		Stats:     Stats{Creatures: 1, MaxDepth: 1},
		Warnings:  creatureWarnings(offset, rec),
	}
	for i := 0; i < count; i++ {
		itemOffset := cur.Offset()
		item, err := readObjectNode(cur, depth+1)
		if err != nil {
			return CreatureNode{}, fmt.Errorf("creature inventory object %d/%d at offset %d: %w", i+1, count, itemOffset, err)
		}
		node.Inventory = append(node.Inventory, item)
		mergeChildStats(&node.Stats, item.Stats, 1)
		node.Warnings = append(node.Warnings, item.Warnings...)
	}
	return node, nil
}

func readRoomNode(cur *Cursor, depth int) (RoomNode, error) {
	if depth > MaxRecursionDepth {
		return RoomNode{}, fmt.Errorf("room recursion depth exceeds %d", MaxRecursionDepth)
	}

	offset := cur.Offset()
	rec, err := DecodeRoomRecord(cur.data[offset:])
	if err != nil {
		return RoomNode{}, fmt.Errorf("room record at offset %d: %w", offset, err)
	}
	if err := cur.skip(RoomSize); err != nil {
		return RoomNode{}, fmt.Errorf("room record at offset %d: %w", offset, err)
	}

	node := RoomNode{
		Offset:   offset,
		Record:   rec,
		Stats:    Stats{Rooms: 1, MaxDepth: 1},
		Warnings: roomWarnings(offset, rec),
	}

	exitCountOffset := cur.Offset()
	exitCount, err := cur.int32()
	if err != nil {
		return RoomNode{}, fmt.Errorf("room exit count: %w", err)
	}
	if exitCount < 0 || exitCount > MaxRoomExits {
		return RoomNode{}, fmt.Errorf("invalid room exit count %d at offset %d", exitCount, exitCountOffset)
	}
	node.Exits = make([]ExitNode, 0, exitCount)
	for i := 0; i < exitCount; i++ {
		exitOffset := cur.Offset()
		exit, err := DecodeExitRecord(cur.data[exitOffset:])
		if err != nil {
			return RoomNode{}, fmt.Errorf("room exit %d/%d at offset %d: %w", i+1, exitCount, exitOffset, err)
		}
		if err := cur.skip(ExitSize); err != nil {
			return RoomNode{}, fmt.Errorf("room exit %d/%d at offset %d: %w", i+1, exitCount, exitOffset, err)
		}
		exitNode := ExitNode{
			Offset:   exitOffset,
			Record:   exit,
			Warnings: exitWarnings(exitOffset, exit),
		}
		node.Exits = append(node.Exits, exitNode)
		node.Warnings = append(node.Warnings, exitNode.Warnings...)
	}
	node.Stats.Exits = exitCount

	creatureCountOffset := cur.Offset()
	creatureCount, err := cur.int32()
	if err != nil {
		return RoomNode{}, fmt.Errorf("room creature count: %w", err)
	}
	if creatureCount < 0 || creatureCount > MaxRoomCreatures {
		return RoomNode{}, fmt.Errorf("invalid room creature count %d at offset %d", creatureCount, creatureCountOffset)
	}
	node.Creatures = make([]CreatureNode, 0, creatureCount)
	for i := 0; i < creatureCount; i++ {
		creatureOffset := cur.Offset()
		creature, err := readCreatureNode(cur, depth+1)
		if err != nil {
			return RoomNode{}, fmt.Errorf("room creature %d/%d at offset %d: %w", i+1, creatureCount, creatureOffset, err)
		}
		node.Creatures = append(node.Creatures, creature)
		mergeChildStats(&node.Stats, creature.Stats, 1)
		node.Warnings = append(node.Warnings, creature.Warnings...)
	}

	objectCountOffset := cur.Offset()
	objectCount, err := cur.int32()
	if err != nil {
		return RoomNode{}, fmt.Errorf("room object count: %w", err)
	}
	if objectCount < 0 || objectCount > MaxRoomObjects {
		return RoomNode{}, fmt.Errorf("invalid room object count %d at offset %d", objectCount, objectCountOffset)
	}
	node.Objects = make([]ObjectNode, 0, objectCount)
	for i := 0; i < objectCount; i++ {
		objectOffset := cur.Offset()
		object, err := readObjectNode(cur, depth+1)
		if err != nil {
			return RoomNode{}, fmt.Errorf("room object %d/%d at offset %d: %w", i+1, objectCount, objectOffset, err)
		}
		node.Objects = append(node.Objects, object)
		mergeChildStats(&node.Stats, object.Stats, 1)
		node.Warnings = append(node.Warnings, object.Warnings...)
	}

	for i := range node.Descriptions {
		description, err := readRoomDescriptionNode(cur, i)
		if err != nil {
			return RoomNode{}, err
		}
		node.Descriptions[i] = description
		switch i {
		case 0:
			node.Record.ShortDescription = description.Text
		case 1:
			node.Record.LongDescription = description.Text
		case 2:
			node.Record.ObjectDescription = description.Text
		}
		if description.Size > 0 {
			node.Stats.Descriptions++
			node.Stats.DescriptionBytes += description.Size
		}
		node.Warnings = append(node.Warnings, textWarning(description.Offset, fmt.Sprintf("room.description[%d]", i), description.Text)...)
	}

	return node, nil
}

func readRoomDescriptionNode(cur *Cursor, index int) (RoomDescriptionNode, error) {
	lengthOffset := cur.Offset()
	size, err := cur.int32()
	if err != nil {
		return RoomDescriptionNode{}, fmt.Errorf("room description %d length: %w", index, err)
	}
	if size < 0 || size > MaxDescriptionBytes {
		return RoomDescriptionNode{}, fmt.Errorf("invalid room description %d length %d at offset %d", index, size, lengthOffset)
	}

	offset := cur.Offset()
	if cur.off+size > len(cur.data) {
		return RoomDescriptionNode{}, fmt.Errorf("room description %d bytes: need %d bytes at offset %d, remaining %d", index, size, cur.Offset(), cur.Remaining())
	}
	description := RoomDescriptionNode{
		Index:        index,
		LengthOffset: lengthOffset,
		Offset:       offset,
		Size:         size,
		Text:         decodeCString(cur.data, offset, size, fmt.Sprintf("room.description[%d]", index)),
	}
	if err := cur.skip(size); err != nil {
		return RoomDescriptionNode{}, fmt.Errorf("room description %d bytes: %w", index, err)
	}
	return description, nil
}

func mergeChildStats(stats *Stats, child Stats, depthOffset int) {
	stats.Objects += child.Objects
	stats.Creatures += child.Creatures
	stats.Rooms += child.Rooms
	stats.Exits += child.Exits
	stats.Descriptions += child.Descriptions
	stats.DescriptionBytes += child.DescriptionBytes
	if depth := child.MaxDepth + depthOffset; depth > stats.MaxDepth {
		stats.MaxDepth = depth
	}
}

func objectWarnings(offset int, rec ObjectRecord) []TreeWarning {
	var warnings []TreeWarning
	warnings = append(warnings, textWarning(offset+objectNameOff, "object.name", rec.Name)...)
	warnings = append(warnings, textWarning(offset+objectDescriptionOff, "object.description", rec.Description)...)
	for i, key := range rec.Keys {
		warnings = append(warnings, textWarning(offset+objectKeyOff+i*cString20, fmt.Sprintf("object.key[%d]", i), key)...)
	}
	warnings = append(warnings, textWarning(offset+objectUseOutputOff, "object.use_output", rec.UseOutput)...)
	return warnings
}

func creatureWarnings(offset int, rec CreatureRecord) []TreeWarning {
	var warnings []TreeWarning
	warnings = append(warnings, textWarning(offset+creatureNameOff, "creature.name", rec.Name)...)
	warnings = append(warnings, textWarning(offset+creatureDescriptionOff, "creature.description", rec.Description)...)
	warnings = append(warnings, textWarning(offset+creatureTalkOff, "creature.talk", rec.Talk)...)
	warnings = append(warnings, textWarning(offset+creaturePasswordOff, "creature.password", rec.Password)...)
	for i, key := range rec.Keys {
		warnings = append(warnings, textWarning(offset+creatureKeyOff+i*cString20, fmt.Sprintf("creature.key[%d]", i), key)...)
	}
	return warnings
}

func roomWarnings(offset int, rec RoomRecord) []TreeWarning {
	return textWarning(offset+roomNameOff, "room.name", rec.Name)
}

func exitWarnings(offset int, rec ExitRecord) []TreeWarning {
	return textWarning(offset+exitNameOff, "exit.name", rec.Name)
}

func textWarning(offset int, field string, text TextField) []TreeWarning {
	if text.Err == nil {
		return nil
	}
	return []TreeWarning{{
		Offset:  offset,
		Field:   field,
		Message: text.Err.Error(),
	}}
}

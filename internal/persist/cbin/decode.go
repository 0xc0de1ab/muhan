package cbin

import (
	"encoding/binary"
	"fmt"
)

type Cursor struct {
	data []byte
	off  int
}

func NewCursor(data []byte) *Cursor {
	return &Cursor{data: data}
}

func (c *Cursor) Offset() int {
	return c.off
}

func (c *Cursor) Remaining() int {
	return len(c.data) - c.off
}

func (c *Cursor) EOF() bool {
	return c.off == len(c.data)
}

func (c *Cursor) skip(n int) error {
	if n < 0 || c.off+n > len(c.data) {
		return fmt.Errorf("need %d bytes at offset %d, remaining %d", n, c.off, c.Remaining())
	}
	c.off += n
	return nil
}

func (c *Cursor) int32() (int, error) {
	if c.off+4 > len(c.data) {
		return 0, fmt.Errorf("need int32 at offset %d, remaining %d", c.off, c.Remaining())
	}
	v := int(int32(binary.LittleEndian.Uint32(c.data[c.off : c.off+4])))
	c.off += 4
	return v, nil
}

func DecodeObjectFile(data []byte) (Stats, error) {
	return decodeObjectFile(data, false)
}

func DecodeObjectFileAllowTrailing(data []byte) (Stats, error) {
	return decodeObjectFile(data, true)
}

func decodeObjectFile(data []byte, allowTrailing bool) (Stats, error) {
	cur := NewCursor(data)
	var st Stats
	if err := decodeObject(cur, &st, 1); err != nil {
		return st, err
	}
	if !cur.EOF() {
		if allowTrailing {
			st.TrailingBytes = cur.Remaining()
			return st, nil
		}
		return st, fmt.Errorf("trailing %d bytes after object tree", cur.Remaining())
	}
	return st, nil
}

func DecodeCreatureFile(data []byte) (Stats, error) {
	cur := NewCursor(data)
	var st Stats
	if err := decodeCreature(cur, &st, 1); err != nil {
		return st, err
	}
	if !cur.EOF() {
		return st, fmt.Errorf("trailing %d bytes after creature tree", cur.Remaining())
	}
	return st, nil
}

func DecodeRoomFile(data []byte) (Stats, error) {
	cur := NewCursor(data)
	var st Stats
	if err := decodeRoom(cur, &st, 1); err != nil {
		return st, err
	}
	if !cur.EOF() {
		return st, fmt.Errorf("trailing %d bytes after room", cur.Remaining())
	}
	return st, nil
}

func ValidateObjectPrototypeFile(data []byte) (int, error) {
	if len(data)%ObjectSize != 0 {
		return 0, fmt.Errorf("size %d is not a multiple of object record size %d", len(data), ObjectSize)
	}
	return len(data) / ObjectSize, nil
}

func ValidateCreaturePrototypeFile(data []byte) (int, error) {
	if len(data)%CreatureSize != 0 {
		return 0, fmt.Errorf("size %d is not a multiple of creature record size %d", len(data), CreatureSize)
	}
	return len(data) / CreatureSize, nil
}

func ValidateBoardIndexFile(data []byte) (int, error) {
	if len(data)%BoardIndexSize != 0 {
		return 0, fmt.Errorf("size %d is not a multiple of board index record size %d", len(data), BoardIndexSize)
	}
	return len(data) / BoardIndexSize, nil
}

func decodeObject(cur *Cursor, st *Stats, depth int) error {
	if depth > MaxRecursionDepth {
		return fmt.Errorf("object recursion depth exceeds %d", MaxRecursionDepth)
	}
	st.addDepth(depth)
	if err := cur.skip(ObjectSize); err != nil {
		return fmt.Errorf("object record: %w", err)
	}
	st.Objects++
	count, err := cur.int32()
	if err != nil {
		return fmt.Errorf("object child count: %w", err)
	}
	if count < 0 || count > MaxObjectChildren {
		return fmt.Errorf("invalid object child count %d at offset %d", count, cur.Offset()-4)
	}
	for i := 0; i < count; i++ {
		if err := decodeObject(cur, st, depth+1); err != nil {
			return fmt.Errorf("object child %d/%d: %w", i+1, count, err)
		}
	}
	return nil
}

func decodeCreature(cur *Cursor, st *Stats, depth int) error {
	if depth > MaxRecursionDepth {
		return fmt.Errorf("creature recursion depth exceeds %d", MaxRecursionDepth)
	}
	st.addDepth(depth)
	if err := cur.skip(CreatureSize); err != nil {
		return fmt.Errorf("creature record: %w", err)
	}
	st.Creatures++
	count, err := cur.int32()
	if err != nil {
		return fmt.Errorf("creature inventory count: %w", err)
	}
	if count < 0 || count > MaxCreatureItems {
		return fmt.Errorf("invalid creature inventory count %d at offset %d", count, cur.Offset()-4)
	}
	for i := 0; i < count; i++ {
		if err := decodeObject(cur, st, depth+1); err != nil {
			return fmt.Errorf("creature inventory object %d/%d: %w", i+1, count, err)
		}
	}
	return nil
}

func decodeRoom(cur *Cursor, st *Stats, depth int) error {
	if depth > MaxRecursionDepth {
		return fmt.Errorf("room recursion depth exceeds %d", MaxRecursionDepth)
	}
	st.addDepth(depth)
	if err := cur.skip(RoomSize); err != nil {
		return fmt.Errorf("room record: %w", err)
	}
	st.Rooms++

	exitCount, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room exit count: %w", err)
	}
	if exitCount < 0 || exitCount > MaxRoomExits {
		return fmt.Errorf("invalid room exit count %d at offset %d", exitCount, cur.Offset()-4)
	}
	if err := cur.skip(exitCount * ExitSize); err != nil {
		return fmt.Errorf("room exits: %w", err)
	}
	st.Exits += exitCount

	creatureCount, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room creature count: %w", err)
	}
	if creatureCount < 0 || creatureCount > MaxRoomCreatures {
		return fmt.Errorf("invalid room creature count %d at offset %d", creatureCount, cur.Offset()-4)
	}
	for i := 0; i < creatureCount; i++ {
		if err := decodeCreature(cur, st, depth+1); err != nil {
			return fmt.Errorf("room creature %d/%d: %w", i+1, creatureCount, err)
		}
	}

	objectCount, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room object count: %w", err)
	}
	if objectCount < 0 || objectCount > MaxRoomObjects {
		return fmt.Errorf("invalid room object count %d at offset %d", objectCount, cur.Offset()-4)
	}
	for i := 0; i < objectCount; i++ {
		if err := decodeObject(cur, st, depth+1); err != nil {
			return fmt.Errorf("room object %d/%d: %w", i+1, objectCount, err)
		}
	}

	for i := 0; i < 3; i++ {
		if err := decodeDescription(cur, st, i); err != nil {
			return err
		}
	}
	return nil
}

func decodeDescription(cur *Cursor, st *Stats, index int) error {
	size, err := cur.int32()
	if err != nil {
		return fmt.Errorf("room description %d length: %w", index, err)
	}
	if size < 0 || size > MaxDescriptionBytes {
		return fmt.Errorf("invalid room description %d length %d at offset %d", index, size, cur.Offset()-4)
	}
	if err := cur.skip(size); err != nil {
		return fmt.Errorf("room description %d bytes: %w", index, err)
	}
	if size > 0 {
		st.Descriptions++
		st.DescriptionBytes += size
	}
	return nil
}

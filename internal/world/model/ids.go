package model

type PlayerID string
type CreatureID string
type RoomID string
type ObjectInstanceID string
type PrototypeID string
type BankID string
type BoardID string
type BoardPostID string
type SpecialID int

func (id PlayerID) IsZero() bool {
	return id == ""
}

func (id CreatureID) IsZero() bool {
	return id == ""
}

func (id RoomID) IsZero() bool {
	return id == ""
}

func (id ObjectInstanceID) IsZero() bool {
	return id == ""
}

func (id PrototypeID) IsZero() bool {
	return id == ""
}

func (id BankID) IsZero() bool {
	return id == ""
}

func (id BoardID) IsZero() bool {
	return id == ""
}

func (id BoardPostID) IsZero() bool {
	return id == ""
}

func (id SpecialID) IsZero() bool {
	return id == 0
}

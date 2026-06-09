package model

import "time"

type CreatureKind string

const (
	CreatureKindPlayer  CreatureKind = "player"
	CreatureKindMonster CreatureKind = "monster"
	CreatureKindNPC     CreatureKind = "npc"
)

type ObjectKind string

const (
	ObjectKindWeapon      ObjectKind = "weapon"
	ObjectKindArmor       ObjectKind = "armor"
	ObjectKindPotion      ObjectKind = "potion"
	ObjectKindScroll      ObjectKind = "scroll"
	ObjectKindWand        ObjectKind = "wand"
	ObjectKindContainer   ObjectKind = "container"
	ObjectKindMoney       ObjectKind = "money"
	ObjectKindKey         ObjectKind = "key"
	ObjectKindLightSource ObjectKind = "lightSource"
	ObjectKindMisc        ObjectKind = "misc"
)

type ObjectRefList struct {
	ObjectIDs []ObjectInstanceID `json:"objectIds,omitempty"`
}

type Room struct {
	ID                RoomID            `json:"id"`
	DisplayName       string            `json:"displayName"`
	ShortDescription  string            `json:"shortDescription,omitempty"`
	LongDescription   string            `json:"longDescription,omitempty"`
	ObjectDescription string            `json:"objectDescription,omitempty"`
	Exits             []Exit            `json:"exits,omitempty"`
	CreatureIDs       []CreatureID      `json:"creatureIds,omitempty"`
	PlayerIDs         []PlayerID        `json:"playerIds,omitempty"`
	Objects           ObjectRefList     `json:"objects,omitempty"`
	Properties        map[string]string `json:"properties,omitempty"`
	Metadata          Metadata          `json:"metadata,omitempty"`
}

type Exit struct {
	Name     string   `json:"name"`
	ToRoomID RoomID   `json:"toRoomId"`
	Flags    []string `json:"flags,omitempty"`
	Metadata Metadata `json:"metadata,omitempty"`
}

type Creature struct {
	ID          CreatureID                  `json:"id"`
	Kind        CreatureKind                `json:"kind"`
	DisplayName string                      `json:"displayName"`
	Description string                      `json:"description,omitempty"`
	Level       int                         `json:"level,omitempty"`
	RoomID      RoomID                      `json:"roomId,omitempty"`
	PlayerID    PlayerID                    `json:"playerId,omitempty"`
	Inventory   ObjectRefList               `json:"inventory,omitempty"`
	Equipment   map[string]ObjectInstanceID `json:"equipment,omitempty"`
	Stats       map[string]int              `json:"stats,omitempty"`
	Properties  map[string]string           `json:"properties,omitempty"`
	Metadata    Metadata                    `json:"metadata,omitempty"`
}

type Player struct {
	ID          PlayerID   `json:"id"`
	DisplayName string     `json:"displayName"`
	CreatureID  CreatureID `json:"creatureId,omitempty"`
	RoomID      RoomID     `json:"roomId,omitempty"`
	AccountName string     `json:"accountName,omitempty"`
	Metadata    Metadata   `json:"metadata,omitempty"`
}

type BankAccount struct {
	ID            BankID        `json:"id"`
	Kind          string        `json:"kind"`
	OwnerName     string        `json:"ownerName"`
	OwnerPlayerID PlayerID      `json:"ownerPlayerId,omitempty"`
	Objects       ObjectRefList `json:"objects,omitempty"`
	Metadata      Metadata      `json:"metadata,omitempty"`
}

type BankSaveBundle struct {
	SchemaVersion int              `json:"schemaVersion,omitempty"`
	BankAccount   BankAccount      `json:"bankAccount"`
	Objects       []ObjectInstance `json:"objects,omitempty"`
}


type ObjectPrototype struct {
	ID          PrototypeID       `json:"id"`
	Kind        ObjectKind        `json:"kind,omitempty"`
	DisplayName string            `json:"displayName"`
	Description string            `json:"description,omitempty"`
	Keywords    []string          `json:"keywords,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	Metadata    Metadata          `json:"metadata,omitempty"`
}

type ObjectInstance struct {
	ID                  ObjectInstanceID  `json:"id"`
	PrototypeID         PrototypeID       `json:"prototypeId"`
	DisplayNameOverride string            `json:"displayNameOverride,omitempty"`
	Quantity            int               `json:"quantity,omitempty"`
	Location            ObjectLocation    `json:"location"`
	Contents            ObjectRefList     `json:"contents,omitempty"`
	Properties          map[string]string `json:"properties,omitempty"`
	Metadata            Metadata          `json:"metadata,omitempty"`
}

type ObjectLocation struct {
	RoomID      RoomID           `json:"roomId,omitempty"`
	CreatureID  CreatureID       `json:"creatureId,omitempty"`
	BankID      BankID           `json:"bankId,omitempty"`
	ContainerID ObjectInstanceID `json:"containerId,omitempty"`
	Slot        string           `json:"slot,omitempty"`
}

type BoardPost struct {
	ID         BoardPostID `json:"id"`
	BoardID    BoardID     `json:"boardId"`
	Title      string      `json:"title"`
	AuthorID   PlayerID    `json:"authorId,omitempty"`
	AuthorName string      `json:"authorName,omitempty"`
	Body       string      `json:"body"`
	CreatedAt  time.Time   `json:"createdAt,omitempty"`
	ReadCount  int         `json:"readCount,omitempty"`
	Metadata   Metadata    `json:"metadata,omitempty"`
}

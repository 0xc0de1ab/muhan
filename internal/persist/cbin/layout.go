package cbin

const (
	ObjectSize     = 352
	CreatureSize   = 1184
	RoomSize       = 480
	ExitSize       = 44
	DailySize      = 8
	LasttimeSize   = 12
	BoardIndexSize = 256
)

const (
	MaxObjectChildren   = 10000
	MaxCreatureItems    = 10000
	MaxRoomExits        = 1000
	MaxRoomCreatures    = 10000
	MaxRoomObjects      = 10000
	MaxDescriptionBytes = 1 << 20
	MaxRecursionDepth   = 128
)

type Stats struct {
	Objects          int `json:"objects"`
	Creatures        int `json:"creatures"`
	Rooms            int `json:"rooms"`
	Exits            int `json:"exits"`
	Descriptions     int `json:"descriptions"`
	DescriptionBytes int `json:"descriptionBytes"`
	TrailingBytes    int `json:"trailingBytes,omitempty"`
	MaxDepth         int `json:"maxDepth"`
}

func (s *Stats) addDepth(depth int) {
	if depth > s.MaxDepth {
		s.MaxDepth = depth
	}
}

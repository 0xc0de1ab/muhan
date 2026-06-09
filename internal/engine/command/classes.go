package command

// Standard legacy class ID mappings (matching the original C source structure)
const (
	legacyClassAssassin   = 1
	legacyClassBarbarian  = 2
	legacyClassCleric     = 3
	legacyClassFighter    = 4
	legacyClassMage       = 5
	legacyClassPaladin    = 6
	legacyClassRanger     = 7
	legacyClassThief      = 8
	legacyClassInvincible = 9
	legacyClassCaretaker  = 10
	legacyClassBulsa      = 11
	legacyClassSubDM      = 12
	legacyClassDM         = 13
)

// Legacy alias mappings used in magic_heal.go
const (
	healClassAssassin   = legacyClassAssassin
	healClassBarbarian  = legacyClassBarbarian
	healClassCleric     = legacyClassCleric
	healClassFighter    = legacyClassFighter
	healClassMage       = legacyClassMage
	healClassPaladin    = legacyClassPaladin
	healClassRanger     = legacyClassRanger
	healClassThief      = legacyClassThief
	healClassInvincible = legacyClassInvincible
	healClassCaretaker  = legacyClassCaretaker
	healClassBulsa      = legacyClassBulsa
	healClassSubDM      = legacyClassSubDM
	healClassDM         = legacyClassDM
)

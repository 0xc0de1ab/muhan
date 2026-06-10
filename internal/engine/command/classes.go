package command

import "muhan/internal/world/model"

// Standard legacy class ID mappings (matching the original C source structure)
// Legacy alias mappings used in magic_heal.go
const (
	healClassAssassin   = model.ClassAssassin
	healClassBarbarian  = model.ClassBarbarian
	healClassCleric     = model.ClassCleric
	healClassFighter    = model.ClassFighter
	healClassMage       = model.ClassMage
	healClassPaladin    = model.ClassPaladin
	healClassRanger     = model.ClassRanger
	healClassThief      = model.ClassThief
	healClassInvincible = model.ClassInvincible
	healClassCaretaker  = model.ClassCaretaker
	healClassBulsa      = model.ClassBulsa
	healClassSubDM      = model.ClassSubDM
	healClassDM         = model.ClassDM
)

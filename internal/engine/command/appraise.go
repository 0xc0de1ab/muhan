package command

import (
	"fmt"
	"strings"

	"muhan/internal/krtext"
	"muhan/internal/world/model"
)

const (
	appraiseClassAssassin   = model.ClassAssassin
	appraiseClassFighter    = model.ClassFighter
	appraiseClassPaladin    = model.ClassPaladin
	appraiseClassRanger     = model.ClassRanger
	appraiseClassThief      = model.ClassThief
	appraiseClassInvincible = model.ClassInvincible

	legacyObjectSharp       = 0
	legacyObjectThrust      = 1
	legacyObjectBlunt       = 2
	legacyObjectPole        = 3
	legacyObjectMissile     = 4
	legacyObjectArmor       = 5
	legacyObjectPotion      = 6
	legacyObjectScroll      = 7
	legacyObjectWand        = 8
	legacyObjectContainer   = 9
	legacyObjectKey         = 11
	legacyObjectLightSource = 12
	legacyObjectMisc        = 13
)

func NewAppraiseHandler(world InventoryWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		player, creature, err := CurrentInventoryCreature(world, InventoryPlayerIDFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}
		class := creatureClass(creature)
		if class != appraiseClassThief && class < appraiseClassInvincible {
			ctx.WriteString("도둑만 물건을 감정할수 있습니다.")
			return StatusDefault, nil
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 감정하실려구요?")
			return StatusDefault, nil
		}
		object, _, ok := findEquipInventoryObjectWithVisibility(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런것을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}

		ctx.WriteString(renderObjectAppraisalWithMagic(world, object, appraiserDetectsMagic(player, creature)))
		return StatusDefault, nil
	}
}

func NewObjectCompareHandler(world InventoryWorld) Handler {
	return func(ctx *Context, resolved ResolvedCommand) (Status, error) {
		player, creature, err := CurrentInventoryCreature(world, InventoryPlayerIDFromContext(ctx))
		if err != nil {
			return StatusDefault, err
		}

		target := getArg(resolved, 0)
		if target == "" {
			ctx.WriteString("무엇을 비교하시려고요?")
			return StatusDefault, nil
		}
		object, _, ok := findEquipInventoryObjectWithVisibility(world, creature, target, firstGetOrdinal(resolved), inventoryViewerDetectsInvisible(player, creature))
		if !ok {
			ctx.WriteString("당신은 그런 것을 갖고 있지 않습니다.")
			return StatusDefault, nil
		}

		ctx.WriteString(renderObjectComparison(world, creature, object))
		return StatusDefault, nil
	}
}

func renderObjectAppraisal(world InventoryWorld, object model.ObjectInstance) string {
	return renderObjectAppraisalWithMagic(world, object, false)
}

func renderObjectAppraisalWithMagic(world InventoryWorld, object model.ObjectInstance, detectMagic bool) string {
	var b strings.Builder
	name := objectDisplayName(world, object)
	legacyType := objectLegacyTypeOrKind(world, object)
	shotsCurrent := objectIntPropertyOrZero(world, object, "shotsCurrent")

	fmt.Fprintf(&b, "이름: %s\n", name)
	fmt.Fprintf(&b, "사용회수 %d\n", shotsCurrent)
	b.WriteString("종류: ")
	if legacyType >= legacyObjectSharp && legacyType <= legacyObjectMissile {
		b.WriteString(legacyWeaponTypeName(legacyType))
		b.WriteString(" 무기.\n")
		sDice := objectIntPropertyOrZero(world, object, "sDice")
		nDice := objectIntPropertyOrZero(world, object, "nDice")
		pDice := objectIntPropertyOrZero(world, object, "pDice")
		adjustment := objectIntPropertyOrZero(world, object, "adjustment")
		fmt.Fprintf(&b, "타격치: %d면%d굴림 더하기 %d", sDice, nDice, pDice)
		if adjustment != 0 {
			fmt.Fprintf(&b, " (+%d)\n", adjustment)
		} else {
			b.WriteByte('\n')
		}
	} else {
		switch legacyType {
		case legacyObjectArmor:
			b.WriteString("방어구")
			fmt.Fprintf(&b, "\n방어력: %02d", objectIntPropertyOrZero(world, object, "armor"))
		case legacyObjectPotion:
			b.WriteString("약")
		case legacyObjectScroll:
			b.WriteString("주문서")
		case legacyObjectWand:
			b.WriteString("주문걸린 물건")
		case legacyObjectContainer:
			b.WriteString("담는 종류")
		case legacyObjectKey:
			b.WriteString("열쇠")
		case legacyObjectLightSource:
			b.WriteString("광원")
		case legacyObjectMisc:
			b.WriteString("모르겠음")
		default:
			b.WriteString("모르겠음")
		}
		b.WriteByte('\n')
	}

	b.WriteString("특성 : ")
	traits := objectAppraisalTraits(world, object)
	if len(traits) == 0 {
		b.WriteString("특성 없음.")
	} else {
		b.WriteString(strings.Join(traits, ", "))
		b.WriteByte('.')
	}
	b.WriteByte('\n')
	writeAppraisalMagicDetails(&b, world, object, detectMagic)
	return b.String()
}

func writeAppraisalMagicDetails(
	b *strings.Builder,
	world InventoryWorld,
	object model.ObjectInstance,
	detectMagic bool,
) {
	if !detectMagic || !objectHasMagicInformation(world, object) {
		return
	}
	b.WriteString("마법적 기운: 있음\n")
	if magicPower := objectIntPropertyOrDefault(world, object, "magicPower", "magicpower"); magicPower != 0 {
		fmt.Fprintf(b, "마법 힘: %d\n", magicPower)
	}
	if charges, ok := objectFirstIntProperty(world, object, "charges", "shotsCurrent", "shotscur"); ok {
		fmt.Fprintf(b, "남은 충전: %d\n", charges)
	}
}

func appraiserDetectsMagic(player model.Player, creature model.Creature) bool {
	return viewerHasDetectMagicCreatureTag(creature) || viewerHasDetectMagicPlayerTag(player)
}

func renderObjectComparison(world InventoryWorld, creature model.Creature, object model.ObjectInstance) string {
	name := objectDisplayName(world, object)
	subject := name + krtext.Particle(name, '0')
	legacyType := objectLegacyTypeOrKind(world, object)
	if legacyType == legacyObjectArmor {
		armor := objectIntPropertyOrZero(world, object, "armor")
		checkAC := armor * 5
		if objectWearFlag(world, object) == legacyWearBody {
			checkAC = armor * 2
		}
		if checkAC > 29 {
			return fmt.Sprintf("%s %d 레벨부터 입을 수 있습니다.", subject, checkAC)
		}
		return fmt.Sprintf("%s 누구나 입을 수 있습니다.", subject)
	}
	if legacyType >= legacyObjectSharp && legacyType < legacyObjectArmor {
		checkDamage := objectIntPropertyOrZero(world, object, "nDice")*
			objectIntPropertyOrZero(world, object, "sDice") +
			objectIntPropertyOrZero(world, object, "pDice")
		class := creatureClass(creature)
		switch class {
		case appraiseClassFighter:
			checkDamage -= 7
		case appraiseClassAssassin, appraiseClassThief:
			checkDamage -= 3
		case appraiseClassPaladin, appraiseClassRanger:
			checkDamage -= 2
		}
		if class >= appraiseClassInvincible {
			if checkDamage > 15 {
				return fmt.Sprintf("%s 검사는 %d레벨, 자객 도둑은 %d레벨, 무사 포졸은 %d레벨,\n권법가 불제자 도술사는 %d레벨부터 사용할 수 있습니다.",
					subject, (checkDamage-7)*3, (checkDamage-3)*3, (checkDamage-2)*3, checkDamage*3)
			}
			return fmt.Sprintf("%s 누구나 무장할 수 있습니다.", subject)
		}
		if checkDamage > 15 {
			return fmt.Sprintf("%s %d 레벨부터 무장할 수 있습니다.", subject, checkDamage*3)
		}
		return fmt.Sprintf("%s 누구나 무장할 수 있습니다.", subject)
	}
	return "무기나 방어구만 가능합니다."
}

func objectAppraisalTraits(world InventoryWorld, object model.ObjectInstance) []string {
	traits := []string{}
	if objectHasAnyFlagOrProperty(world, object, "noMage", "onomag") {
		traits = append(traits, "도술사 불제자 거부")
	}
	if objectHasAnyFlagOrProperty(world, object, "goodOnly", "ogoodo") {
		traits = append(traits, "선한 사람용")
	}
	if objectHasAnyFlagOrProperty(world, object, "evilOnly", "oevilo") {
		traits = append(traits, "악한 사람용")
	}
	if objectHasAnyFlagOrProperty(world, object, "enchanted", "oencha") {
		traits = append(traits, "빙의 되있음")
	}
	if objectHasAnyFlagOrProperty(world, object, "femaleOnly", "noMale", "onomal") {
		traits = append(traits, "남성 금지")
	}
	if objectHasAnyFlagOrProperty(world, object, "maleOnly", "noFemale", "onofem") {
		traits = append(traits, "여성 금지")
	}
	return traits
}

func objectHasAnyFlagOrProperty(world InventoryWorld, object model.ObjectInstance, names ...string) bool {
	return objectHasAnyTag(world, object, names...) || objectHasAnyPropertyFlag(world, object, names...)
}

func legacyWeaponTypeName(legacyType int) string {
	switch legacyType {
	case legacyObjectSharp:
		return "도"
	case legacyObjectThrust:
		return "검"
	case legacyObjectBlunt:
		return "봉"
	case legacyObjectPole:
		return "창"
	case legacyObjectMissile:
		return "궁"
	default:
		return "무기"
	}
}

func objectLegacyTypeOrKind(world InventoryWorld, object model.ObjectInstance) int {
	if legacyType := objectLegacyType(world, object); legacyType >= 0 {
		return legacyType
	}
	switch {
	case objectKindIs(world, object, model.ObjectKindWeapon):
		return legacyObjectThrust
	case objectKindIs(world, object, model.ObjectKindArmor):
		return legacyObjectArmor
	case objectKindIs(world, object, model.ObjectKindPotion):
		return legacyObjectPotion
	case objectKindIs(world, object, model.ObjectKindScroll):
		return legacyObjectScroll
	case objectKindIs(world, object, model.ObjectKindWand):
		return legacyObjectWand
	case objectKindIs(world, object, model.ObjectKindContainer):
		return legacyObjectContainer
	case objectKindIs(world, object, model.ObjectKindKey):
		return legacyObjectKey
	case objectKindIs(world, object, model.ObjectKindLightSource):
		return legacyObjectLightSource
	default:
		return legacyObjectMisc
	}
}

func objectIntPropertyOrZero(world InventoryWorld, object model.ObjectInstance, key string) int {
	value, _ := objectIntProperty(world, object, key)
	return value
}

func creatureClass(creature model.Creature) int {
	return creatureStat(creature, "class")
}

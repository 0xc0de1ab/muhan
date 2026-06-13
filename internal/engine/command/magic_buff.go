package command

import (
	"fmt"

	"github.com/0xc0de1ab/muhan/internal/world/model"
)

func magicEffectBless(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	how := determineHow(world, object)
	if how == howCast && !creatureHasAnyFlag(actor, "SBLESS", "sbless") {
		ctx.WriteString("\n당신은 아직 그런 주술을 터득하지 못했습니다.\n")
		return false, nil
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 10); failed || err != nil {
		return false, err
	}

	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	if targetArg == "" || targetArg == "나" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
		if ok && target.creature.ID == actor.ID {
			isSelf = true
		}
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("\n그런 사람이 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	var interval int64
	if how == howCast {
		intel := creatureStat(actor, "intelligence")
		intelBonus := legacyStatBonus(intel)
		interval = int64(1200 + intelBonus*600)
		if interval < 300 {
			interval = 300
		}
		class := creatureClass(actor)
		level := creatureStat(actor, "level")
		if class == model.ClassCleric || class == model.ClassPaladin {
			interval += int64(60 * ((level + 3) / 4))
		}
		if room, ok := world.Room(actor.RoomID); ok {
			if roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				interval += 800
			}
		}
	} else {
		interval = 1200
	}

	if err := magicEffectUpdateTags(world, target, []string{"blessed", "PBLESS"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PBLESS", timeNow().Unix()+interval)
	}

	if recalc, ok := world.(interface {
		RecalculateTHACO(model.CreatureID) error
	}); ok {
		_ = recalc.RecalculateTHACO(target.creature.ID)
	}

	actorName := attackCreatureName(actor)
	targetName := attackCreatureName(target.creature)

	if isSelf {
		if how == howPotion {
			ctx.WriteString("\n하늘에서 천수광이 내려와 성스런 기운들로 몸을 휘감습니다.\n")
		} else {
			if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
				ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
			ctx.WriteString("\n당신은 조용히 눈을 감으며 성현주를 외웁니다.\n성현주를 외우자 머리에서 삼매광이 뿜어져 나와 성스런 기운이\n몸을 휘감습니다.\n")
			roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s이 조용히 눈을 감으며 성현주를 외웁니다.\n그의 머리에서 삼매광이 뿜어져 나와 성스런 기운이 몸을\n휘감습니다.\n", actorName))
		}
	} else {
		if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
			ctx.WriteString("\n이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
		ctx.WriteString(fmt.Sprintf("\n당신은 한쪽손을 %s의 머리에 얹으며 성현주를 외웁니다.\n그의 머리에서 삼매광이 뿜어져 나와 성스러운 기운이 몸을 휘감습니다.\n", targetName))
		if target.hasPlayer {
			_ = sendToPlayerAgent3(ctx, target.player.ID, fmt.Sprintf("\n%s이 당신의 머리에 한쪽손을 얹으며 성현주를 외웁니다.\n당신의 머리에서 삼매광이 뿜어져 나와 성스러운 기운이 몸을\n휘감습니다.\n", actorName))
		}
		var targetPlayerID model.PlayerID
		if target.hasPlayer {
			targetPlayerID = target.player.ID
		}
		_ = roomBroadcast2(ctx, world, actor.RoomID, ctx.SessionID, targetPlayerID, fmt.Sprintf("\n%s이 %s의 머리에 한쪽손을 얹으며 성현주를 \n외웁니다.\n그의 머리에서 삼매광이 뿜어져 나와 성스러운 기운이 몸을\n휘감습니다.\n", actorName, targetName))
	}

	return true, nil
}

func magicEffectProtection(ctx *Context, world StatusWorld, actor model.Creature, object model.ObjectInstance, resolved ResolvedCommand) (bool, error) {
	how := determineHow(world, object)
	if how == howCast && !creatureHasAnyFlag(actor, "SPROTE", "sprote") {
		ctx.WriteString("\n당신은 아직 그 주술을 터득하지 못했습니다.\n")
		return false, nil
	}
	if failed, err := magicEffectSpellFail(world, actor, how, 10); failed || err != nil {
		return false, err
	}

	isSelf := false
	targetArg := getArg(resolved, 1)
	var target magicEffectTarget
	var ok bool
	if targetArg == "" || targetArg == "나" {
		isSelf = true
		target, ok = magicEffectSelfTarget(ctx, world, actor)
	} else {
		if how == howPotion {
			ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
			return false, nil
		}
		target, ok = magicEffectResolveTarget(ctx, world, actor, targetArg, getOrdinal(resolved, 1))
		if ok && target.creature.ID == actor.ID {
			isSelf = true
		}
	}
	if !ok || target.creature.ID.IsZero() || (!isSelf && !target.hasPlayer) {
		ctx.WriteString("그런 사람은 존재하지 않습니다.\n")
		return false, nil
	}
	if how == howPotion && !isSelf {
		ctx.WriteString("그 물건은 자신에게만 사용할 수 있습니다.\n")
		return false, nil
	}

	var interval int64
	if how == howCast {
		intel := creatureStat(actor, "intelligence")
		intelBonus := legacyStatBonus(intel)
		interval = int64(1200 + intelBonus*600)
		if interval < 300 {
			interval = 300
		}
		class := creatureClass(actor)
		level := creatureStat(actor, "level")
		if class == model.ClassCleric || class == model.ClassPaladin {
			interval += int64(60 * ((level + 3) / 4))
		}
		if room, ok := world.Room(actor.RoomID); ok {
			if roomHasAnyFlag(room, "RPMEXT", "rpmext") {
				interval += 800
			}
		}
	} else {
		interval = 1200
	}

	if err := magicEffectUpdateTags(world, target, []string{"protection", "PPROTE"}, nil); err != nil {
		return false, err
	}
	if expUpdater, ok := world.(magicEffectExpirationWorld); ok {
		expUpdater.SetEffectExpiration(target.creature.ID, "PPROTE", timeNow().Unix()+interval)
	}

	if recalc, ok := world.(interface {
		RecalculateAC(model.CreatureID) error
	}); ok {
		_ = recalc.RecalculateAC(target.creature.ID)
	}

	actorName := attackCreatureName(actor)
	targetName := attackCreatureName(target.creature)

	if isSelf {
		if how == howPotion {
			ctx.WriteString("\n빛의 수호령들이 당신 주위를 둘러싸며 방어의 진을 형성했습니다.\n")
		} else {
			if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
				ctx.WriteString("이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
			}
			ctx.WriteString("두손으로 인을 맺은 뒤 수호진의 주문을 외웁니다.\n빛의 수호령들이 당신 주위를 둘러싸며 방어의 진을 형성했습니다.\n")
			roomBroadcast(ctx, actor.RoomID, fmt.Sprintf("\n%s이 두손을 모으며 수호진의 주문을 외웁니다.\n빛의 수호령들이 그의 주위를 둘러싸며 방어의 진을 형성했습니다.\n", actorName))
		}
	} else {
		if how == howCast && magicEffectRoomExtendsMagic(world, actor) {
			ctx.WriteString("이 방의 기운이 당신의 주술력을 강화시킵니다.\n")
		}
		ctx.WriteString(fmt.Sprintf("%s의 몸에 수호인을 그리며 수호진의 주문을 걸었습니다.\n빛의 수호령들이 그의 주위를 둘러싸며 방어의 진을 형성했습니다.\n", targetName))
		if target.hasPlayer {
			_ = sendToPlayerAgent3(ctx, target.player.ID, fmt.Sprintf("%s이 당신의 몸에 수호인을 그리며 주문을 걸었습니다.\n빛의 수호령들이 당신의 주위를 둘러싸며 방어의 진을 형성했습니다.\n", actorName))
		}
		var targetPlayerID model.PlayerID
		if target.hasPlayer {
			targetPlayerID = target.player.ID
		}
		_ = roomBroadcast2(ctx, world, actor.RoomID, ctx.SessionID, targetPlayerID, fmt.Sprintf("%s이 %s의 몸에 수호인을 그리며 주문을 걸었습니다.\n빛의 수호령들이 그의 주위를 둘러싸며 방어의 진을 형성했습니다.\n", actorName, targetName))
	}

	return true, nil
}

package game

import (
	"fmt"

	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// PursuePlayerAfterMove ports the synchronous monster chase C runs at the end of
// move() (command2.c:604-641) and go() (command6.c:328-359): once a player has
// stepped from fromRoomID into their new room, every monster left in the old room
// that hates the mover follows them in, subject to the follow/invisibility gates
// and a dexterity escape roll. Running it in the move command (rather than the
// periodic loop) matches C — there is no trail-loss window and a player crossing
// several rooms in one tick is followed room by room.
//
// It returns the player-facing "…이 당신을 따라옵니다." lines for the mover; the
// "…이 …를 따라갑니다." notices are broadcast to the old room here. handler selects
// the C path: "go" uses the go() gates (MFOLLO required, dex threshold 10); any
// other value uses the move() gates (invisibility exemption, dex threshold 15).
func PursuePlayerAfterMove(world UpdateActiveWorld, playerID model.PlayerID, fromRoomID model.RoomID, handler string, now int64) ([]string, error) {
	if world == nil || playerID.IsZero() || fromRoomID.IsZero() {
		return nil, nil
	}
	player, ok := world.Player(playerID)
	if !ok {
		return nil, nil
	}
	mover, ok := world.Creature(player.CreatureID)
	if !ok {
		return nil, nil
	}
	toRoomID := mover.RoomID
	if toRoomID.IsZero() || toRoomID == fromRoomID {
		return nil, nil
	}
	fromRoom, ok := world.Room(fromRoomID)
	if !ok {
		return nil, nil
	}

	isGo := handler == "go"
	dexBase := 15
	if isGo {
		dexBase = 10
	}
	moverName := mover.DisplayName

	var followMessages []string
	for _, cid := range fromRoom.CreatureIDs {
		monster, ok := world.Creature(cid)
		if !ok || monster.Kind != model.CreatureKindMonster || monster.RoomID != fromRoomID {
			continue
		}

		// C treats a monster as a "loose" chaser when it is not a normal follower
		// or is a DM-follower (!MFOLLO || MDMFOL).
		loose := !creatureHasAnyFlag(monster, "MFOLLO") || creatureHasAnyFlag(monster, "MDMFOL", "dmFollow")
		if loose {
			if isGo {
				// go() (command6.c:330-333): loose monsters never pursue.
				continue
			}
			// move() (command2.c:607-613): loose monsters skip an invisible mover
			// unless they can see the invisible; nobody chases a DM-invisible mover.
			if (!creatureHasAnyFlag(monster, "MDINVI", "detectInvisible") && creatureHasAnyFlag(mover, "PINVIS", "invisible")) ||
				creatureHasAnyFlag(mover, "PDMINV", "dmInvisible") {
				continue
			}
		}

		// C checks the monster's primary enemy (first_enm) by name. The Go enemy
		// list does not preserve C's head ordering, so — consistent with
		// findCurrentEnemy — the mover pursues if it is any of the monster's enemies.
		enemies, _ := world.CreatureEnemies(monster.ID)
		if !pursuitCreatureIsEnemy(mover, enemies) {
			continue
		}

		// Dexterity escape roll (command2.c:623 / command6.c:344).
		if mrand(1, 50) > dexBase-mover.Stats["dexterity"]+monster.Stats["dexterity"] {
			continue
		}

		// C move() runs die_perm_crt() on a permanent monster that abandons its
		// room (respawn bookkeeping + MDEATH/quest/summon side effects); go() only
		// clears the flag. Either way MPERMT is dropped once it follows the mover out.
		if creatureHasAnyFlag(monster, "MPERMT", "permanent") {
			if !isGo {
				_, _ = HandlePermanentCreatureDeath(world, playerID, monster.ID, now)
			}
			_, _ = world.UpdateCreatureTags(monster.ID, nil, []string{"MPERMT", "permanent"})
		}

		if err := world.MoveCreatureToRoom(monster.ID, toRoomID); err != nil {
			continue
		}

		mPart := krtext.Particle(monster.DisplayName, '1')
		pPart := krtext.Particle(moverName, '3')
		_ = world.BroadcastRoom(fromRoomID, "", fmt.Sprintf("\n%s%s %s%s 따라갑니다.", monster.DisplayName, mPart, moverName, pPart))
		lead := "\n"
		if isGo {
			lead = ""
		}
		followMessages = append(followMessages, fmt.Sprintf("%s%s%s 당신을 따라옵니다.", lead, monster.DisplayName, mPart))
	}
	return followMessages, nil
}

func pursuitCreatureIsEnemy(pc model.Creature, enemies []string) bool {
	for _, enm := range enemies {
		if pc.DisplayName == enm {
			return true
		}
	}
	return false
}

// anyEnemyOnline reports whether any of the monster's hated enemies is currently
// logged in (find_who in C update_active). While true, C keeps the monster's aggro
// (end_enm_crt) even though the enemy is not in the room; when it goes false the
// caller drops the aggro (del_enm_crt).
func anyEnemyOnline(world UpdateActiveWorld, c model.Creature) bool {
	enemies, err := world.CreatureEnemies(c.ID)
	if err != nil || len(enemies) == 0 {
		return false
	}
	enemySet := make(map[string]struct{}, len(enemies))
	for _, e := range enemies {
		enemySet[e] = struct{}{}
	}
	for _, sess := range world.ActiveSessions() {
		if sess.ActorID == "" {
			continue
		}
		player, ok := world.Player(model.PlayerID(sess.ActorID))
		if !ok {
			continue
		}
		if _, hit := enemySet[activePlayerDisplayName(world, player)]; hit {
			return true
		}
	}
	return false
}

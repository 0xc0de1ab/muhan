package game

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/0xc0de1ab/muhan/internal/engine/legacy"
	"github.com/0xc0de1ab/muhan/internal/krtext"
	"github.com/0xc0de1ab/muhan/internal/session"
	"github.com/0xc0de1ab/muhan/internal/world/model"
)

// UpdateActiveWorld defines the required methods for active monster ticking, combat, AI, and player death handling.
type UpdateActiveWorld interface {
	ActiveCreatures() []model.Creature
	Creature(model.CreatureID) (model.Creature, bool)
	Player(model.PlayerID) (model.Player, bool)
	Room(model.RoomID) (model.Room, bool)
	MovePlayerToRoom(model.PlayerID, model.RoomID) error
	ApplyCreatureDamage(model.CreatureID, int) (model.Creature, int, bool, error)
	RecordCreatureDamage(victimID, attackerID model.CreatureID, damage int) error
	UpdateCreatureTags(model.CreatureID, []string, []string) (model.Creature, error)
	UpdatePlayerTags(model.PlayerID, []string, []string) (model.Player, error)
	SetCreatureStat(model.CreatureID, string, int) error
	CreatureEnemies(model.CreatureID) ([]string, error)
	AddEnemy(attacker, defender model.CreatureID) (bool, error)
	RemoveEnemy(creatureID model.CreatureID, enemyName string) error
	ClearCreatureEnemies(creatureID model.CreatureID) error
	RemoveCreature(model.CreatureID) error
	FinalizeMonsterDeath(model.CreatureID) (bool, error)
	MoveCreatureToRoom(creatureID model.CreatureID, roomID model.RoomID) error
	UseCreatureCooldown(model.CreatureID, string, int64, int64) (int64, bool, error)
	SetCreatureCooldown(model.CreatureID, string, int64, int64) error
	ActiveSessions() []ActiveSession
	WriteToSession(sessionID session.ID, text string, isPrompt bool) error
	BroadcastAll(text string) error
	BroadcastRoom(roomID model.RoomID, excludeSessionID session.ID, text string) error
	SavePlayer(playerID model.PlayerID) error
	MoveObjectToCreatureInventory(objectID model.ObjectInstanceID, creatureID model.CreatureID) error
	DestroyObject(model.ObjectInstanceID) error
	Object(objectID model.ObjectInstanceID) (model.ObjectInstance, bool)
	ObjectPrototype(model.PrototypeID) (model.ObjectPrototype, bool)
	RecalculateAC(model.CreatureID) error
	RecalculateTHACO(model.CreatureID) error
}

type studySpell struct {
	power int
	name  string
	tag   string
}

const monsterRecallTargetRoomID = model.RoomID("room:00001")
const legacyMonsterKnownSpellLimit = 10

var studySpells = []studySpell{
	{power: 1, name: "회복", tag: "SVIGOR"},
	{power: 2, name: "삭풍", tag: "SHURTS"},
	{power: 3, name: "발광", tag: "SLIGHT"},
	{power: 4, name: "해독", tag: "SCUREP"},
	{power: 5, name: "성현진", tag: "SBLESS"},
	{power: 6, name: "수호진", tag: "SPROTE"},
	{power: 7, name: "화궁", tag: "SFIREB"},
	{power: 8, name: "은둔법", tag: "SINVIS"},
	{power: 9, name: "도력반", tag: "SRESTO"},
	{power: 10, name: "은둔감지술", tag: "SDINVI"},
	{power: 11, name: "주문감지술", tag: "SDMAGI"},
	{power: 12, tag: "STELEP"},
	{power: 13, name: "혼동", tag: "SBEFUD"},
	{power: 14, name: "뇌전", tag: "SLGHTN"},
	{power: 15, name: "동설주", tag: "SICEBL"},
	{power: 16, name: "빙의", tag: "SENCHA"},
	{power: 17, name: "귀환", tag: "SRECAL"},
	{power: 18, name: "소환", tag: "SSUMMO"},
	{power: 19, name: "원기회복", tag: "SMENDW"},
	{power: 20, name: "완치", tag: "SFHEAL"},
	{power: 21, name: "추적", tag: "STRACK"},
	{power: 22, name: "부양술", tag: "SLEVIT"},
	{power: 23, name: "방열진", tag: "SRFIRE"},
	{power: 24, name: "비상술", tag: "SFLYSP"},
	{power: 25, name: "보마진", tag: "SRMAGI"},
	{power: 26, name: "권풍술", tag: "SSHOCK"},
	{power: 27, name: "지동술", tag: "SRUMBL"},
	{power: 28, name: "화선도", tag: "SBURNS"},
	{power: 29, name: "탄수공", tag: "SBLIST"},
	{power: 30, name: "풍마현", tag: "SDUSTG"},
	{power: 31, name: "파초식", tag: "SWBOLT"},
	{power: 32, name: "폭진", tag: "SCRUSH"},
	{power: 33, name: "낙석", tag: "SENGUL"},
	{power: 34, name: "화풍술", tag: "SBURST"},
	{power: 35, name: "화룡대천", tag: "SSTEAM"},
	{power: 36, name: "토합술", tag: "SSHATT"},
	{power: 37, name: "주작현", tag: "SIMMOL"},
	{power: 38, name: "열사천", tag: "SBLOOD"},
	{power: 39, name: "파천풍", tag: "STHUND"},
	{power: 40, name: "지옥패", tag: "SEQUAK"},
	{power: 41, name: "태양안", tag: "SFLFIL"},
	{power: 42, name: "선악감지", tag: "SKNOWA"},
	{power: 43, name: "저주해소", tag: "SREMOV"},
	{power: 44, name: "방한진", tag: "SRCOLD"},
	{power: 45, name: "수생술", tag: "SBRWAT"},
	{power: 46, name: "지방호", tag: "SSSHLD"},
	{power: 47, name: "천리안", tag: "SLOCAT"},
	{power: 48, name: "백치술", tag: "SDREXP"},
	{power: 49, name: "치료", tag: "SRMDIS"},
	{power: 50, name: "개안술", tag: "SRMBLD"},
	{power: 51, name: "공포", tag: "SFEARS"},
	{power: 52, name: "전회복", tag: "SRVIGO"},
	{power: 53, name: "전송", tag: "STRANO"},
	{power: 54, name: "실명", tag: "SBLIND"},
	{power: 55, name: "봉합구", tag: "SSILNC"},
	{power: 56, name: "이혼대법", tag: "SCHARM"},
	{power: 57, name: "저주", tag: "SCURSE"},
	{power: 58, name: "천지진동", tag: "SISIX1"},
	{power: 59, name: "천상풍", tag: "SISIX2"},
	{power: 60, name: "천마강기", tag: "SISIX3"},
	{power: 61, name: "빙천파", tag: "SISIX4"},
	{power: 62, name: "공포해소", tag: "SRMGONG"},
	{power: 63, name: "혈사천", tag: "XIXIX1"},
	{power: 64, name: "빙설검", tag: "XIXIX2"},
	{power: 65, name: "멸겁화궁", tag: "XIXIX3"},
	{power: 66, name: "탄지수통", tag: "XIXIX4"},
}

type osp_t struct {
	tag   string
	name  string
	mp    int
	ndice int
	sdice int
	pdice int
}

var ospell = []osp_t{
	{tag: "SHURTS", name: "삭풍", mp: 3, ndice: 1, sdice: 8, pdice: 0},
	{tag: "SRUMBL", name: "지동술", mp: 3, ndice: 1, sdice: 8, pdice: 0},
	{tag: "SBURNS", name: "화선도", mp: 3, ndice: 1, sdice: 7, pdice: 1},
	{tag: "SBLIST", name: "탄수공", mp: 3, ndice: 1, sdice: 8, pdice: 0},
	{tag: "SDUSTG", name: "풍마현", mp: 7, ndice: 2, sdice: 5, pdice: 7},
	{tag: "SCRUSH", name: "폭진", mp: 7, ndice: 2, sdice: 5, pdice: 7},
	{tag: "SFIREB", name: "화궁", mp: 7, ndice: 2, sdice: 5, pdice: 8},
	{tag: "SWBOLT", name: "파초식", mp: 7, ndice: 2, sdice: 5, pdice: 8},
	{tag: "SSHOCK", name: "권풍술", mp: 10, ndice: 2, sdice: 5, pdice: 13},
	{tag: "SENGUL", name: "낙석", mp: 10, ndice: 2, sdice: 5, pdice: 13},
	{tag: "SBURST", name: "화풍술", mp: 10, ndice: 2, sdice: 5, pdice: 13},
	{tag: "SSTEAM", name: "화룡대천", mp: 10, ndice: 2, sdice: 5, pdice: 13},
	{tag: "SLGHTN", name: "뇌전", mp: 15, ndice: 3, sdice: 4, pdice: 18},
	{tag: "SSHATT", name: "토합술", mp: 15, ndice: 3, sdice: 4, pdice: 19},
	{tag: "SIMMOL", name: "주작현", mp: 15, ndice: 3, sdice: 4, pdice: 18},
	{tag: "SBLOOD", name: "열사천", mp: 15, ndice: 3, sdice: 4, pdice: 18},
	{tag: "STHUND", name: "파천풍", mp: 25, ndice: 4, sdice: 5, pdice: 30},
	{tag: "SEQUAK", name: "지옥패", mp: 25, ndice: 4, sdice: 5, pdice: 30},
	{tag: "SFLFIL", name: "태양안", mp: 25, ndice: 4, sdice: 5, pdice: 30},
	{tag: "SICEBL", name: "동설주", mp: 25, ndice: 4, sdice: 5, pdice: 30},
	{tag: "SISIX1", name: "천지진동", mp: 35, ndice: 5, sdice: 6, pdice: 50},
	{tag: "SISIX2", name: "천상풍", mp: 35, ndice: 5, sdice: 6, pdice: 50},
	{tag: "SISIX3", name: "천마강기", mp: 35, ndice: 5, sdice: 6, pdice: 50},
	{tag: "SISIX4", name: "빙천파", mp: 35, ndice: 5, sdice: 6, pdice: 50},
	{tag: "XIXIX1", name: "혈사천", mp: 60, ndice: 11, sdice: 12, pdice: 70},
	{tag: "XIXIX2", name: "빙설검", mp: 60, ndice: 11, sdice: 12, pdice: 70},
	{tag: "XIXIX3", name: "멸겁화궁", mp: 60, ndice: 11, sdice: 12, pdice: 70},
	{tag: "XIXIX4", name: "탄지수통", mp: 60, ndice: 11, sdice: 12, pdice: 70},
}

var legacyBonus = [...]int{
	-4, -4, -4, -3, -3, -2, -2, -1, -1, -1,
	0, 0, 0, 0, 1, 1, 1, 2, 2, 2,
	3, 3, 3, 3, 4, 4, 4, 4, 4, 5,
	5, 5, 5, 5, 5, 6, 6, 6, 6, 6,
	6, 6, 6, 6, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	9, 9, 9, 9, 9, 9, 9, 9, 9, 9,
	9, 9, 9, 9, 9, 9,
}

// UpdateActiveMonsters iterates all active monsters and runs AI, ticking, and combat logic.
func UpdateActiveMonsters(world UpdateActiveWorld, t int64) {
	if world == nil {
		return
	}

	creatures := world.ActiveCreatures()
	for _, c := range creatures {
		// Verify monster is alive and has stats
		if c.Stats == nil {
			continue
		}
		hpCur := c.Stats["hpCurrent"]
		if hpCur <= 0 {
			continue
		}

		room, ok := world.Room(c.RoomID)
		if !ok {
			continue
		}

		// Fidelity: only tick "active" monsters (those sharing room with >=1 player).
		// Matches C update_active: if(!rom_ptr->first_ply){ del_active; continue; }
		// Critical for exact monster movement/wander despawn, scavenge, regen timing
		// (only when players present/visible). Prevents distant mobs from wandering
		// off or scavenging in empty areas.
		playerCount := len(room.PlayerIDs)
		if playerCount == 0 {
			for _, cid := range room.CreatureIDs {
				if crt, ok := world.Creature(cid); ok {
					if crt.Kind == model.CreatureKindPlayer || !crt.PlayerID.IsZero() {
						playerCount++
					}
				}
			}
		}
		if playerCount == 0 {
			continue
		}

		// 1. Handle monster HP/MP regeneration.
		hpMax := c.Stats["hpMax"]
		mpCur := c.Stats["mpCurrent"]
		mpMax := c.Stats["mpMax"]
		if hpCur < hpMax || mpCur < mpMax {
			_, ok, _ := world.UseCreatureCooldown(c.ID, "heal", t, 60)
			if ok {
				hpAdd := hpMax / 10
				if hpAdd < 1 {
					hpAdd = 1
				}
				mpAdd := mpMax / 6
				if mpAdd < 1 {
					mpAdd = 1
				}
				newHp := hpCur + hpAdd
				if newHp > hpMax {
					newHp = hpMax
				}
				newMp := mpCur + mpAdd
				if newMp > mpMax {
					newMp = mpMax
				}
				_ = world.SetCreatureStat(c.ID, "hpCurrent", newHp)
				_ = world.SetCreatureStat(c.ID, "mpCurrent", newMp)
				// Re-fetch c since stats updated
				if updated, ok := world.Creature(c.ID); ok {
					c = updated
				}
			}
		}

		// Clear befuddled flag when timer expires (C: lines 227-230)
		if creatureHasAnyFlag(c, "MBEFUD", "befuddled") {
			_, timerExpired, _ := world.UseCreatureCooldown(c.ID, "befuddled", t, 0)
			if timerExpired {
				if updated, err := world.UpdateCreatureTags(c.ID, nil, []string{"MBEFUD", "befuddled"}); err == nil {
					c = updated
					_ = world.RecalculateAC(c.ID)
					_ = world.RecalculateTHACO(c.ID)
				}
			}
		}

		// Clear charm flag when timer expires (C: lines 245-246)
		if creatureHasAnyFlag(c, "MCHARM", "charm") {
			_, timerExpired, _ := world.UseCreatureCooldown(c.ID, "charmed", t, 0)
			if timerExpired {
				if updated, err := world.UpdateCreatureTags(c.ID, nil, []string{"MCHARM", "charm"}); err == nil {
					c = updated
					_ = world.RecalculateAC(c.ID)
					_ = world.RecalculateTHACO(c.ID)
				}
			}
		}

		// 2. Scavenger behavior ("MSCAVE"): pick up the first floor item.
		// C checks only rom_ptr->first_obj after the random gate.
		if creatureHasAnyFlag(c, "MSCAVE", "scavenger") {
			_, ok, _ := world.UseCreatureCooldown(c.ID, "scavenge", t, 20)
			if ok && mrand(1, 100) <= 15 {
				var targetObj model.ObjectInstance
				foundObj := false
				if len(room.Objects.ObjectIDs) > 0 {
					if obj, ok := world.Object(room.Objects.ObjectIDs[0]); ok {
						// C checks: !ONOTAK && !OSCENE && !OHIDDN && !OPERM2 && !OPERMT
						if !objectHasAnyFlag(world, obj, "ONOTAK", "noTake", "OSCENE", "scenery", "OHIDDN", "hidden", "OPERM2", "permanent2", "OPERMT", "permanent") {
							targetObj = obj
							foundObj = true
						}
					}
				}
				if foundObj {
					if err := world.MoveObjectToCreatureInventory(targetObj.ID, c.ID); err == nil {
						_, _ = world.UpdateCreatureTags(c.ID, []string{"MHASSC", "hasScavenged"}, nil)
						mPart := krtext.Particle(c.DisplayName, '1')
						objName := activeObjectDisplayName(world, targetObj)
						oPart := krtext.Particle(objName, '3')
						roomMsg := fmt.Sprintf("%s%s %s%s 줍습니다.", c.DisplayName, mPart, objName, oPart)
						_ = world.BroadcastRoom(room.ID, "", roomMsg)
					}
				}
			}
		}

		// 3. Wandering: despawn monster if conditions match.
		// C: !MHASSC && !MPERMT && !MDMFOL, checks traffic roll, requires no enemies.
		if !creatureHasAnyFlag(c, "MHASSC", "hasScavenged") &&
			!creatureHasAnyFlag(c, "MPERMT", "permanent") &&
			!creatureHasAnyFlag(c, "MDMFOL", "dmFollow") {
			_, ok, _ := world.UseCreatureCooldown(c.ID, "wander", t, 20)
			if ok {
				enemies, _ := world.CreatureEnemies(c.ID)
				trafficStr := room.Properties["traffic"]
				traffic := 0
				if v, err := strconv.Atoi(trafficStr); err == nil {
					traffic = v
				}
				if len(enemies) == 0 && mrand(1, 100) <= traffic {
					mPart := krtext.Particle(c.DisplayName, '1')
					roomMsg := fmt.Sprintf("%s%s 당신 주위를 방황하고 있습니다.", c.DisplayName, mPart)
					_ = world.BroadcastRoom(room.ID, "", roomMsg)
					_ = world.RemoveCreature(c.ID)
					continue
				}
			}
		}

		// 4. Aggro and combat tick
		enemies, _ := world.CreatureEnemies(c.ID)
		if len(enemies) == 0 {
			// Aggro check
			if creatureCanInitiateAggro(c) {

				interval := int64(2)
				if c.Stats["dexterity"] < 20 {
					interval = 3
				}

				_, ok, _ := world.UseCreatureCooldown(c.ID, "attack", t, interval)
				if ok {
					targetPlayer, found := findAggroTarget(world, room, c)
					if found {
						targetPC, ok := world.Creature(targetPlayer.CreatureID)
						if ok && targetPC.Stats["dexterity"] > c.Stats["dexterity"] && mrand(1, 10) < 4 {
							continue
						}

						_ = world.SetCreatureCooldown(c.ID, "attack", t, 0)
						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						targetName := activePlayerDisplayName(world, targetPlayer)
						if sessionID != "" {
							_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신을 공격합니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1')), false)
						}
						roomMsg := fmt.Sprintf("\n%s%s %s%s 공격합니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1'), targetName, krtext.Particle(targetName, '3'))
						_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
						wasNew, _ := world.AddEnemy(c.ID, targetPlayer.CreatureID)
						_ = wasNew // broadcast of aggro gain already performed above for new hatred
					}
				}
			}
		} else {
			// Combat tick: monster has targets
			hasWasAttackedTag := false
			for _, tag := range c.Metadata.Tags {
				if strings.EqualFold(tag, "was_attacked") {
					hasWasAttackedTag = true
					break
				}
			}
			if hasWasAttackedTag {
				_ = world.SetCreatureCooldown(c.ID, "attack", t, 0)
				if updated, err := world.UpdateCreatureTags(c.ID, nil, []string{"was_attacked"}); err == nil {
					c = updated
				}
			}

			targetPlayer, found := findCurrentEnemy(world, room, c)
			if !found {
				// Target left or died. Attempt basic pursuit (추적) to adjacent if hated player nearby; else clear (aggro decay on far).
				if !attemptPursuit(world, c, room) {
					_ = world.ClearCreatureEnemies(c.ID)
				}
				continue
			}

			targetPC, ok := world.Creature(targetPlayer.CreatureID)
			if !ok {
				_ = world.ClearCreatureEnemies(c.ID)
				continue
			}
			targetName := activePlayerDisplayName(world, targetPlayer)

			// Wire AC/THACO recalc for fresh combat stats (P0-3 integration)
			_ = world.RecalculateTHACO(c.ID)
			_ = world.RecalculateAC(targetPC.ID)
			_ = world.RecalculateAC(c.ID)

			interval := int64(2)
			if c.Stats["dexterity"] < 20 {
				interval = 3
			}

			_, ok, _ = world.UseCreatureCooldown(c.ID, "attack", t, interval)
			if !ok {
				continue
			}

			charmed := creatureHasAnyFlag(c, "MCHARM", "charm")
			n := 20
			if creatureHasAnyFlag(c, "MMAGIO", "magic-only") {
				n = creatureProficiency(c, 0)
			}

			isCasting := false
			if creatureHasAnyFlag(c, "MMAGIC", "magic") && mrand(1, 100) <= n && !charmed {
				rtn := monsterCastSpell(world, c, targetPlayer, t)
				if rtn == 2 {
					continue
				} else if rtn == 1 {
					isCasting = true
				}
			}

			if !isCasting {
				// To-hit check
				toHit := c.Stats["thaco"] - targetPC.Stats["armor"]/10
				if toHit < 1 {
					toHit = 1
				}

				if mrand(1, 20) >= toHit && !charmed {
					// Hit!
					if creatureHasAnyFlag(c, "MBRETH", "breath") && mrand(1, 30) < 5 {
						// Breath attack
						wp1 := creatureHasAnyFlag(c, "MBRWP1", "breath-type-1")
						wp2 := creatureHasAnyFlag(c, "MBRWP2", "breath-type-2")
						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						mPart := krtext.Particle(c.DisplayName, '1')

						if wp1 && !wp2 {
							// Spit
							dmg := rollDice((c.Level+3)/4, 3, 0)
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 퉤! 침을 뱉습니다!", c.DisplayName, mPart), false)
							}
							roomMsg := fmt.Sprintf("\n%s%s %s에게 퉤! 침을 뱉습니다!", c.DisplayName, mPart, targetName)
							_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
							applyDamageToPlayer(world, targetPlayer, c, dmg)
						} else if wp1 && wp2 {
							// Poison gas
							dmg := rollDice((c.Level+3)/4, 2, 1)
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 고약한 냄새가 나는 입김을 뿜어댑니다!", c.DisplayName, mPart), false)
							}
							roomMsg := fmt.Sprintf("\n%s%s %s에게 고약한 냄새가 나는 입김을 뿜어댑니다!", c.DisplayName, mPart, targetName)
							_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신을 중독시켰습니다.", c.DisplayName, mPart), false)
							}
							_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PPOISN", "poisoned"}, nil)
							applyDamageToPlayer(world, targetPlayer, c, dmg)
						} else if !wp1 && wp2 {
							// Cold breath
							resistCold := creatureHasAnyFlag(targetPC, "PRCOLD", "resist-cold")
							dice := 4
							if resistCold {
								dice = 2
							}
							dmg := rollDice((c.Level+3)/4, dice, 0)
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 냉기를 내뿜습니다!", c.DisplayName, mPart), false)
							}
							roomMsg := fmt.Sprintf("\n%s%s %s에게 냉기를 내뿜습니다!", c.DisplayName, mPart, targetName)
							_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
							applyDamageToPlayer(world, targetPlayer, c, dmg)
						} else {
							// Fire breath
							resistFire := creatureHasAnyFlag(targetPC, "PRFIRE", "resist-fire")
							dice := 4
							if resistFire {
								dice = 2
							}
							dmg := rollDice((c.Level+3)/4, dice, 0)
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 불을 뿜습니다!", c.DisplayName, mPart), false)
							}
							roomMsg := fmt.Sprintf("\n%s%s %s에게 불을 뿜습니다!", c.DisplayName, mPart, targetName)
							_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
							applyDamageToPlayer(world, targetPlayer, c, dmg)
						}
					} else if creatureHasAnyFlag(c, "MENEDR", "energy-drain") && mrand(1, 100) < 10 {
						// Energy drain
						dmg := rollDice((c.Level+3)/4, 5, ((c.Level+3)/4)*5)
						currExp := targetPC.Stats["experience"]
						if dmg > currExp {
							dmg = currExp
						}
						if dmg < 0 {
							dmg = 0
						}
						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						mPart := krtext.Particle(c.DisplayName, '1')
						if sessionID != "" {
							_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신의 경험치를 갉아먹습니다!", c.DisplayName, mPart), false)
						}
						roomMsg := fmt.Sprintf("\n%s%s %s의 경험치를 갉아먹습니다!", c.DisplayName, mPart, targetName)
						_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
						_ = world.SetCreatureStat(targetPC.ID, "experience", currExp-dmg)
						if sessionID != "" {
							_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신의 경험치 %d 를 줄었습니다.", c.DisplayName, mPart, dmg), false)
						}
					} else {
						// Physical attack
						dmg := rollDice(c.Stats["nDice"], c.Stats["sDice"], c.Stats["pDice"]) - (70-targetPC.Stats["armor"])/10
						if dmg < 1 {
							dmg = 1
						}

						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						if creatureHasAnyFlag(c, "MBEFUD", "befuddled") {
							dmg = dmg / 3
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 혼비백산합니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1')), false)
							}
							roomMsg := fmt.Sprintf("\n%s%s 혼비백산합니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1'))
							_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
						}

						// C: chance2 = MIN(80, ((att_ptr->level+3)/4) + (20-att_ptr->thaco))*2;
						chance2 := (targetPC.Level+3)/4 + 20 - targetPC.Stats["thaco"]
						if chance2 > 80 {
							chance2 = 80
						}
						chance2 *= 2

						if creatureHasAnyFlag(targetPC, "PREFLECT", "reflect") && mrand(1, 160) <= chance2 {
							// Deflected
							if sessionID != "" {
								_ = world.WriteToSession(sessionID, fmt.Sprintf("\n당신은 %s의 공격을 튕겨냅니다.", c.DisplayName), false)
							}
							roomMsg := fmt.Sprintf("\n%s%s %s의 공격을 튕겨냅니다.", targetName, krtext.Particle(targetName, '1'), c.DisplayName)
							_ = world.BroadcastRoom(room.ID, sessionID, roomMsg)
							if mrand(1, 300) <= chance2 {
								refDmg := dmg
								if refDmg > c.Stats["hpCurrent"] {
									refDmg = c.Stats["hpCurrent"]
								}
								_, applied, dead, _ := world.ApplyCreatureDamage(c.ID, refDmg)
								_ = world.RecordCreatureDamage(c.ID, targetPC.ID, applied)
								if sessionID != "" {
									_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s의 공격을 튕겨내어 %d만큼의 상처를 입혔습니다.", c.DisplayName, applied), false)
								}
								if dead {
									_, _ = HandlePermanentCreatureDeath(world, targetPlayer.ID, c.ID, t)
									_, _ = world.FinalizeMonsterDeath(c.ID)
									deathMsg := fmt.Sprintf("\n%s이(가) 쓰러졌습니다.\n", c.DisplayName)
									if sessionID != "" {
										_ = world.WriteToSession(sessionID, deathMsg, false)
									}
									_ = world.BroadcastRoom(room.ID, sessionID, deathMsg)
									_ = world.ClearCreatureEnemies(c.ID)
									continue
								}
							}
						} else {
							// Apply damage
							applyDamageToPlayer(world, targetPlayer, c, dmg)
						}
					}

					// Poison check
					if creatureHasAnyFlag(c, "MPOISS", "poisonous") && mrand(1, 100) <= 10 {
						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						if sessionID != "" {
							_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신을 중독시켰습니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1')), false)
						}
						_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PPOISN", "poisoned"}, nil)
					}
					// Disease check
					if creatureHasAnyFlag(c, "MDISEA", "diseased") && mrand(1, 100) <= 10 {
						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						if sessionID != "" {
							_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 감염시켰습니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1')), false)
						}
						_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PDISEA", "diseased"}, nil)
					}
					// Blindness check
					if creatureHasAnyFlag(c, "MBLNDR", "blinding") && mrand(1, 100) <= 10 {
						sessionID := activePlayerSessionID(world, targetPlayer.ID)
						if sessionID != "" {
							_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신을 실명시켰습니다.", c.DisplayName, krtext.Particle(c.DisplayName, '1')), false)
						}
						_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PBLIND", "blind"}, nil)
					}

					// Class Skills
					class := creatureClass(c)
					if class >= 9 {
						class = mrand(1, 8)
					}

					switch class {
					case model.ClassAssassin:
						chance := (((c.Level+3)/4)-((targetPC.Level+3)/4))*5 + legacyBonus[c.Stats["dexterity"]]*6
						if chance > 80 {
							chance = 80
						}
						if mrand(1, 100) <= chance && mrand(1, 100) <= 15 {
							crtPoison(world, c, targetPlayer, t)
						}
					case model.ClassBarbarian:
						chance := 50 + (((c.Level+3)/4)-((targetPC.Level+3)/4))*5 + legacyBonus[c.Stats["strength"]]*3 + (legacyBonus[c.Stats["dexterity"]]-legacyBonus[targetPC.Stats["dexterity"]])*2
						if chance > 85 {
							chance = 85
						}
						if mrand(1, 100) <= chance && mrand(1, 100) <= 20 {
							crtKick(world, c, targetPlayer, t)
						}
					case model.ClassCleric, model.ClassPaladin:
						chance := 50 + (((c.Level+3)/4)-((targetPC.Level+3)/4))*5 + legacyBonus[c.Stats["piety"]]*3
						if class == model.ClassPaladin {
							chance += 15
						} else {
							chance += 25
						}
						if chance > 80 {
							chance = 80
						}
						if mrand(1, 100) <= chance && mrand(1, 100) <= 10 {
							crtTurn(world, c, targetPlayer, t)
						}
					case model.ClassFighter:
						chance := 50 + (((c.Level+3)/4)-((targetPC.Level+3)/4))*3 + legacyBonus[c.Stats["strength"]]*3 + (legacyBonus[c.Stats["dexterity"]]-legacyBonus[targetPC.Stats["dexterity"]])*2
						if chance > 85 {
							chance = 85
						}
						if mrand(1, 100) <= chance && mrand(1, 100) <= 15 {
							crtBash(world, c, targetPlayer, t)
						}
					case model.ClassMage:
						chance := (((c.Level+3)/4)-((targetPC.Level+3)/4))*5 + legacyBonus[c.Stats["intelligence"]]*5
						if chance > 80 {
							chance = 80
						}
						if mrand(1, 100) <= chance && mrand(1, 100) <= 15 {
							crtAbsorb(world, c, targetPlayer, t)
						}
					case model.ClassRanger:
						chance := (((c.Level+3)/4)-((targetPC.Level+3)/4))*5 + legacyBonus[c.Stats["dexterity"]]*6
						if chance > 80 {
							chance = 80
						}
						if mrand(20, 100) <= chance && mrand(1, 100) <= 10 {
							crtMagicStop(world, c, targetPlayer, t)
						}
					}

					if creatureHasAnyFlag(c, "MDISIT", "dissolveItems") && mrand(1, 100) <= 15 {
						legacyMonsterDissolveItem(world, c, targetPlayer)
					}
				} else {
					// Missed
					sessionID := activePlayerSessionID(world, targetPlayer.ID)
					if sessionID != "" {
						_ = world.WriteToSession(sessionID, fmt.Sprintf("\n당신은 %s의 공격을 피했습니다.", c.DisplayName), false)
					}
				}

				// Player auto-retaliation
				playerRetaliate(world, targetPlayer, c, t)
			}
		}
	}
}

func findAggroTarget(world UpdateActiveWorld, room model.Room, c model.Creature) (model.Player, bool) {
	var targetPlayer model.Player
	hasTarget := false
	lowestPietyVal := 99999

	for _, pid := range room.PlayerIDs {
		player, ok := world.Player(pid)
		if !ok {
			continue
		}
		pc, ok := world.Creature(player.CreatureID)
		if !ok {
			continue
		}

		if pc.Stats != nil {
			if hp, ok := pc.Stats["hpCurrent"]; ok && hp <= 0 {
				continue
			}
		}

		isInvis := creatureHasAnyFlag(pc, "invisible", "pinvis")
		canDetectInvis := creatureHasAnyFlag(c, "detectInvisible", "MDINVI")
		if isInvis && !canDetectInvis {
			continue
		}

		isHidden := creatureHasAnyFlag(pc, "hidden", "phiddn")
		if isHidden {
			continue
		}

		if creatureHasAnyFlag(pc, "dmInvisible", "pdminv", "PDMINV") {
			continue
		}

		targetAlign := pc.Stats["alignment"]

		if !creatureShouldAggroTarget(c, pc, targetAlign) {
			continue
		}

		// Skip attack if charmed by player
		if is_charm_crt(c.DisplayName, pc) && creatureHasAnyFlag(c, "MCHARM", "charm") {
			continue
		}

		piety := pc.Stats["piety"]
		if piety < lowestPietyVal {
			lowestPietyVal = piety
			targetPlayer = player
			hasTarget = true
		}
	}
	return targetPlayer, hasTarget
}

func creatureCanInitiateAggro(c model.Creature) bool {
	return creatureHasAnyFlag(c, "MAGGRE", "aggressive") ||
		creatureHasAnyFlag(c, "MGAGGR", "good-aggr") ||
		creatureHasAnyFlag(c, "MEAGGR", "evil-aggr")
}

func creatureShouldAggroTarget(monster, target model.Creature, targetAlign int) bool {
	if creatureHasAnyFlag(monster, "MAGGRE", "aggressive") {
		return true
	}

	monsterTier := (monster.Level + 3) / 4
	targetTier := (target.Level + 3) / 4
	if targetTier < monsterTier {
		return false
	}

	// C low_piety_alg uses MGAGGR to consider good-aligned players
	// (alignment >= 100), and MEAGGR for evil-aligned players
	// (alignment <= -100).
	if creatureHasAnyFlag(monster, "MGAGGR", "good-aggr") && targetAlign >= 100 {
		return true
	}
	if creatureHasAnyFlag(monster, "MEAGGR", "evil-aggr") && targetAlign <= -100 {
		return true
	}
	return false
}

func findCurrentEnemy(world UpdateActiveWorld, room model.Room, c model.Creature) (model.Player, bool) {
	enemies, err := world.CreatureEnemies(c.ID)
	if err != nil || len(enemies) == 0 {
		return model.Player{}, false
	}

	for _, pid := range room.PlayerIDs {
		player, ok := world.Player(pid)
		if !ok {
			continue
		}
		pc, ok := world.Creature(player.CreatureID)
		if !ok {
			continue
		}

		for _, enemyName := range enemies {
			if pc.DisplayName == enemyName {
				if pc.Stats != nil {
					if hp, ok := pc.Stats["hpCurrent"]; ok && hp > 0 {
						return player, true
					}
				}
			}
		}
	}
	return model.Player{}, false
}

// attemptPursuit ports the C move() cross-room chase gates (command2.c:604-641):
// a monster whose hated enemy has left follows it into an adjacent room unless an
// invisibility exemption or the dexterity escape roll blocks the chase.
//
// The return value tells the caller whether to retain the monster's aggro. C
// keeps first_enm whenever the hated player is still adjacent (it merely skips the
// chase when a gate blocks it), so a blocked-but-present target returns true; only
// when no hated player is adjacent at all does it return false, letting the caller
// decay the aggro.
func attemptPursuit(world UpdateActiveWorld, c model.Creature, room model.Room) bool {
	enemies, _ := world.CreatureEnemies(c.ID)
	if len(enemies) == 0 {
		return false
	}
	keepAggro := false
	for _, ex := range room.Exits {
		toRoom, ok := world.Room(ex.ToRoomID)
		if !ok {
			continue
		}
		for _, pid := range toRoom.PlayerIDs {
			pl, ok := world.Player(pid)
			if !ok {
				continue
			}
			pc, ok := world.Creature(pl.CreatureID)
			if !ok {
				continue
			}
			if pc.Stats != nil {
				if hp, ok := pc.Stats["hpCurrent"]; ok && hp <= 0 {
					continue
				}
			}
			if !pursuitCreatureIsEnemy(pc, enemies) {
				continue
			}
			// A hated player is adjacent: keep aggro even if a gate blocks this chase.
			keepAggro = true

			// Invisibility exemption (command2.c:607-613): non-follower monsters (or
			// DM-followers) do not chase invisible players unless they can see the
			// invisible; DM-invisible players are never chased by them.
			if !creatureHasAnyFlag(c, "MFOLLO") || creatureHasAnyFlag(c, "MDMFOL", "dmFollow") {
				if (!creatureHasAnyFlag(c, "MDINVI", "detectInvisible") && creatureHasAnyFlag(pc, "PINVIS", "invisible")) ||
					creatureHasAnyFlag(pc, "PDMINV", "dmInvisible") {
					continue
				}
			}

			// Dexterity escape roll (command2.c:623): a nimble player can shake a
			// clumsier pursuer.
			if mrand(1, 50) > 15-pc.Stats["dexterity"]+c.Stats["dexterity"] {
				continue
			}

			if err := world.MoveCreatureToRoom(c.ID, toRoom.ID); err == nil {
				mPart := krtext.Particle(c.DisplayName, '1')
				pPart := krtext.Particle(pl.DisplayName, '3')
				_ = world.BroadcastRoom(room.ID, "", fmt.Sprintf("\n%s%s %s%s 쫓아갑니다.\n", c.DisplayName, mPart, pl.DisplayName, pPart))
				_ = world.BroadcastRoom(toRoom.ID, "", fmt.Sprintf("\n%s%s %s%s 따라 들어옵니다.\n", c.DisplayName, mPart, pl.DisplayName, pPart))
				return true
			}
		}
	}
	return keepAggro
}

func pursuitCreatureIsEnemy(pc model.Creature, enemies []string) bool {
	for _, enm := range enemies {
		if pc.DisplayName == enm {
			return true
		}
	}
	return false
}

func applyDamageToPlayer(world UpdateActiveWorld, player model.Player, monster model.Creature, damage int) {
	pc, ok := world.Creature(player.CreatureID)
	if !ok {
		return
	}
	_, applied, dead, _ := world.ApplyCreatureDamage(pc.ID, damage)
	_ = world.RecordCreatureDamage(pc.ID, monster.ID, applied)

	sessionID := activePlayerSessionID(world, player.ID)
	mPart := krtext.Particle(monster.DisplayName, '1')
	targetName := activePlayerDisplayName(world, player)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 %d만큼의 상처를 입혔습니다.", monster.DisplayName, mPart, applied), false)
	}
	roomMsg := fmt.Sprintf("\n%s%s %s%s %d만큼의 피해를 입힙니다.", monster.DisplayName, mPart, targetName, krtext.Particle(targetName, '3'), applied)
	_ = world.BroadcastRoom(pc.RoomID, sessionID, roomMsg)

	if dead {
		_ = killPlayer(world, player, monster)
	} else {
		_, _ = world.AddEnemy(monster.ID, pc.ID)
		_, _ = world.AddEnemy(pc.ID, monster.ID)
		_ = world.RecalculateAC(monster.ID)
		_ = world.RecalculateTHACO(pc.ID)

		if creatureHasAnyFlag(pc, "PWIMPY") {
			wimpyValue := pc.Stats["wimpyValue"]
			if wimpyValue == 0 {
				wimpyValue = 10
			}
			if latest, ok := world.Creature(pc.ID); ok {
				hpCur := latest.Stats["hpCurrent"]
				if hpCur > 0 && hpCur <= wimpyValue {
					if disp, ok := world.(interface {
						DispatchCommand(sessionID session.ID, playerID model.PlayerID, line string) error
					}); ok {
						_ = disp.DispatchCommand(sessionID, player.ID, "도망")
					}
				}
			}
		}
	}
}

func killPlayer(world UpdateActiveWorld, player model.Player, monster model.Creature) error {
	pc, ok := world.Creature(player.CreatureID)
	if !ok {
		return nil
	}
	room, ok := world.Room(pc.RoomID)
	if !ok {
		return nil
	}

	hpMax := creatureStat(pc, "hpMax")
	mpMax := creatureStat(pc, "mpMax")
	mpCur := creatureStat(pc, "mpCurrent")

	if err := world.SetCreatureStat(pc.ID, "hpCurrent", hpMax); err != nil {
		return err
	}
	newMp := mpCur
	if mpMax/10 > newMp {
		newMp = mpMax / 10
	}
	if err := world.SetCreatureStat(pc.ID, "mpCurrent", newMp); err != nil {
		return err
	}

	if _, err := world.UpdatePlayerTags(player.ID, nil, []string{"PPOISN", "poison", "PDISEA", "disease"}); err != nil {
		return err
	}

	if err := world.MovePlayerToRoom(player.ID, model.RoomID("room:1008")); err != nil {
		return err
	}

	_ = world.SavePlayer(player.ID)

	sessionID := activePlayerSessionID(world, player.ID)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, "\n당신은 쓰러졌습니다.\n", false)
		_ = world.WriteToSession(sessionID, "당신은 죽으면서 몇가지 물건을 떨어뜨렸습니다.\n", false)
	}

	if !roomHasAnyFlag(room, "RSUVIV", "survival") {
		playerName := pc.DisplayName
		_ = world.BroadcastAll(fmt.Sprintf("\n### 애석하게도 %s님이 %s에게 죽었습니다.", playerName, monster.DisplayName))
	}

	return nil
}

func playerRetaliate(world UpdateActiveWorld, player model.Player, monster model.Creature, t int64) {
	pc, ok := world.Creature(player.CreatureID)
	if !ok {
		return
	}

	if pc.Stats != nil {
		if hp, ok := pc.Stats["hpCurrent"]; ok && hp <= 0 {
			return
		}
	}

	// Wire recalc for accurate player counterattack stats
	_ = world.RecalculateTHACO(pc.ID)
	_ = world.RecalculateAC(monster.ID)

	thaco := creatureStat(pc, "thaco")
	monsterArmor := creatureStat(monster, "armor")
	target := thaco - monsterArmor/10
	if creatureHasAnyFlag(pc, "fear", "fearful", "PFEARS") {
		target += 2
	}
	if creatureHasAnyFlag(pc, "blind", "pblind", "PBLIND") {
		target += 5
	}

	roll := mrand(1, 30)
	if roll < target {
		sessionID := activePlayerSessionID(world, player.ID)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n당신의 공격은 빗나갔습니다.\n"), false)
		}
		return
	}

	damage := 0
	hasWield := false
	var wieldObj model.ObjectInstance
	for key, slotID := range pc.Equipment {
		if key == "wield" {
			if obj, ok := world.Object(slotID); ok {
				wieldObj = obj
				hasWield = true
				break
			}
		}
	}

	if hasWield {
		nd := activeObjectIntProp(world, wieldObj, "nDice")
		sd := activeObjectIntProp(world, wieldObj, "sDice")
		pd := activeObjectIntProp(world, wieldObj, "pDice")
		damage = rollDice(nd, sd, pd) + strengthDamageBonus(pc)
	} else {
		nd := pc.Stats["nDice"]
		sd := pc.Stats["sDice"]
		pd := pc.Stats["pDice"]
		damage = rollDice(nd, sd, pd) + strengthDamageBonus(pc)
		class := creatureStat(pc, "class")
		if class == model.ClassBarbarian || class > model.ClassInvincible {
			damage += (pc.Level + 3) / 4
		}
	}

	if damage < 1 {
		damage = 1
	}

	_, applied, dead, err := world.ApplyCreatureDamage(monster.ID, damage)
	if err != nil {
		return
	}
	_ = world.RecordCreatureDamage(monster.ID, pc.ID, applied)

	sessionID := activePlayerSessionID(world, player.ID)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, fmt.Sprintf("\n당신은 %s에게 %d만큼의 피해를 주었습니다.\n", monster.DisplayName, applied), false)
	}

	roomMsg := fmt.Sprintf("\n%s%s %s에게 %d만큼의 피해를 입힙니다.\n", pc.DisplayName, krtext.Particle(pc.DisplayName, '1'), monster.DisplayName, applied)
	_ = world.BroadcastRoom(pc.RoomID, sessionID, roomMsg)

	if dead {
		_, _ = HandlePermanentCreatureDeath(world, player.ID, monster.ID, t)
		_, _ = world.FinalizeMonsterDeath(monster.ID)
		deathMsg := fmt.Sprintf("\n%s%s 쓰러졌습니다.\n", monster.DisplayName, krtext.Particle(monster.DisplayName, '1'))
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, deathMsg, false)
		}
		_ = world.BroadcastRoom(pc.RoomID, sessionID, deathMsg)
		_ = world.ClearCreatureEnemies(monster.ID)
	} else {
		wasNew1, _ := world.AddEnemy(monster.ID, pc.ID)
		wasNew2, _ := world.AddEnemy(pc.ID, monster.ID)
		_ = wasNew1 || wasNew2 // aggro gained (mutual hatred established on counter/retaliate)
	}
}

func monsterCastSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	known := monsterKnownSpells(c)

	var spl studySpell
	if len(known) == 0 {
		spl = studySpell{power: 2, name: "삭풍", tag: "SHURTS"}
	} else {
		spl = known[mrand(0, len(known)-1)]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)

	if spl.tag == "SVIGOR" || spl.tag == "SMENDW" || spl.tag == "SFHEAL" {
		return monsterCastHealingSpell(world, c, spl.tag, t)
	}
	if spl.tag == "SBLIND" {
		return monsterCastBlindSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SSILNC" {
		return monsterCastSilenceSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SFEARS" {
		return monsterCastFearSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SBEFUD" {
		return monsterCastBefuddleSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SDREXP" {
		return monsterCastDrainExpSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRMDIS" {
		return monsterCastRemoveDiseaseSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRMBLD" {
		return monsterCastRemoveBlindnessSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SCHARM" {
		return monsterCastCharmSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SCURSE" {
		return monsterCastCurseSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRVIGO" {
		return monsterCastRoomVigorSpell(world, c, t)
	}
	if spl.tag == "SRMGONG" {
		return monsterCastRemoveFearSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SCUREP" {
		return monsterCastCurePoisonSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SREMOV" {
		return monsterCastRemoveCurseSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SLIGHT" {
		return monsterCastLightSpell(world, c, t)
	}
	if spl.tag == "SBLESS" {
		return monsterCastBlessSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SPROTE" {
		return monsterCastProtectionSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SINVIS" {
		return monsterCastInvisibilitySpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SDINVI" {
		return monsterCastDetectInvisibleSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SDMAGI" {
		return monsterCastDetectMagicSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SLEVIT" {
		return monsterCastLevitateSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRFIRE" {
		return monsterCastResistFireSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SFLYSP" {
		return monsterCastFlySpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRMAGI" {
		return monsterCastResistMagicSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRCOLD" {
		return monsterCastResistColdSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SBRWAT" {
		return monsterCastBreatheWaterSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SSSHLD" {
		return monsterCastEarthShieldSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SKNOWA" {
		return monsterCastKnowAlignmentSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRESTO" {
		return monsterCastRestoreSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SRECAL" {
		return monsterCastRecallSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "STELEP" {
		return monsterCastTeleportSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SENCHA" {
		return monsterCastEnchantSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "SLOCAT" || spl.tag == "STRANO" {
		return 0
	}
	if spl.tag == "SSUMMO" {
		return monsterCastSummonSpell(world, c, targetPlayer, t)
	}
	if spl.tag == "STRACK" {
		return monsterCastTrackSpell(world, c, targetPlayer, t)
	}

	// Offensive spell
	var offensive osp_t
	foundOffensiveSpell := false
	for _, entry := range ospell {
		if entry.tag == spl.tag {
			offensive = entry
			foundOffensiveSpell = true
			break
		}
	}
	if !foundOffensiveSpell {
		return 0
	}
	if creatureStat(c, "mpCurrent") < offensive.mp {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-offensive.mp)
	if legacyMonsterSpellFails(c) {
		return 0
	}

	dmg := rollDice(offensive.ndice, offensive.sdice, offensive.pdice)
	if creatureHasAnyFlag(targetPC, "PRMAGI") {
		resist := targetPC.Stats["piety"] + targetPC.Stats["intelligence"]
		if resist > 50 {
			resist = 50
		}
		dmg -= (dmg * 2 * resist) / 100
	}
	roomMsg := fmt.Sprintf("\n%s이 %s 주문을 %s에게 외웁니다.", c.DisplayName, spl.name, activePlayerDisplayName(world, targetPlayer))
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if detail := legacyMonsterOffensiveSpellRoomDetail(c, spl.tag); detail != "" {
		_ = world.BroadcastRoom(c.RoomID, "", detail)
	}

	applyMonsterSpellDamageToPlayer(world, targetPlayer, c, spl.name, dmg)
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterKnownSpells(c model.Creature) []studySpell {
	known := make([]studySpell, 0, legacyMonsterKnownSpellLimit)
	for _, spell := range studySpells {
		if len(known) >= legacyMonsterKnownSpellLimit {
			break
		}
		if creatureHasAnyFlag(c, spell.tag) {
			known = append(known, spell)
		}
	}
	return known
}

type monsterTargetBuffSpec struct {
	addTags     []string
	expireTag   string
	targetLine  string
	roomLine    string
	recalculate string
}

type monsterTargetEffectSpec struct {
	cost             int
	addTags          []string
	expireTag        string
	duration         func(UpdateActiveWorld, model.Creature) int64
	blockOnRoomEnemy bool
	skipSpellFail    bool
	targetLine       string
	roomLine         string
}

func monsterCastInvisibilitySpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:             15,
		addTags:          []string{"PINVIS", "invisible"},
		expireTag:        "PINVIS",
		duration:         monsterLegacyInvisibilityDuration,
		blockOnRoomEnemy: true,
		targetLine:       "\n%s이 당신에게 소명부를 먹이고 은둔법의 주문을 겁니다.\n몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 사라졌습니다.\n",
		roomLine:         "\n%s이 %s에게 소명부를 먹이고 은둔법의 주문을 겁니다.\n그의 몸이 눈부실 정도로 강렬한 빛을 내다가 갑자기 사라졌습니다.\n",
	})
}

func monsterCastDetectInvisibleSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       10,
		addTags:    []string{"PDINVI", "detectInvisible"},
		expireTag:  "PDINVI",
		duration:   monsterLegacyMageSightDuration,
		targetLine: "\n%s이 당신의 인당혈을 찍으며 은둔감지술을 외웠습니다.\n갑자기 두눈에 푸른광안이 떠오르며 숨어있는 자들을 볼수\n있게 되었습니다.\n",
		roomLine:   "\n%s이 %s의 인당혈을 찍으며 은둔감지술을 외웁니다.\n그의 눈에서 푸른광안이 떠오릅니다.\n",
	})
}

func monsterCastDetectMagicSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       10,
		addTags:    []string{"PDMAGI", "detectMagic"},
		expireTag:  "PDMAGI",
		duration:   monsterLegacyMageSightDuration,
		targetLine: "\n%s이 당신의 백회혈을 찍으며 주문감지술의 \n주문을 외웁니다.\n갑자기 두눈에 은빛광안이 떠오르며 주술에 관한 안목이 넓어졌습니다.\n",
		roomLine:   "\n%s이 %s의 백회혈을 찍으며 주문감지술의 \n주문을 외웁니다.\n갑자기 그의 두눈에 은빛광안이 떠오릅니다.\n.",
	})
}

func monsterCastLevitateSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       10,
		addTags:    []string{"PLEVIT", "levitate"},
		expireTag:  "PLEVIT",
		duration:   monsterLegacyLevitateDuration,
		targetLine: "\n%s이 당신에게 부양부적을 붙히며 주문을 외웁니다.\n당신의 몸이 살짝 떠오르기 시작 합니다.\n",
		roomLine:   "\n%s이 %s에게 부양부적을 붙히며 주문을 외웁니다.\n주문을 외우자 그의 몸이 살짝 떠오릅니다.\n",
	})
}

func monsterCastResistFireSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       12,
		addTags:    []string{"PRFIRE", "resistFire"},
		expireTag:  "PRFIRE",
		duration:   monsterLegacyElementalDuration,
		targetLine: "\n%s이 당신에게 방열부적을 붙이며 주문을 외웁니다.\n갑자기 오행중 수의 수호령들이 나타나 당신주위에 \n진을 형성합니다.\n",
		roomLine:   "%s이 %s에게 방열부적을 붙이며 주문을 외웁니다.\n오행중 수의 수호령들이 나타나 그의 주위에 진을 형성합니다.\n",
	})
}

func monsterCastFlySpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       15,
		addTags:    []string{"PFLYSP", "fly"},
		expireTag:  "PFLYSP",
		duration:   monsterLegacyFlyDuration,
		targetLine: "\n%s이 당신에게 비상부를 붙히며 주문을 외웠습니다.\n갑자기 당신의 몸이 공기처럼 가벼워지며 하늘로 떠올라\n날기 시작합니다.\n",
		roomLine:   "\n%s이 %s에게 비상부를 붙히며 주문을 외웠습니다.\n그의 몸이 하늘로 떠오르며 날기 시작합니다.\n",
	})
}

func monsterCastResistMagicSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       12,
		addTags:    []string{"PRMAGI", "resistMagic"},
		expireTag:  "PRMAGI",
		duration:   monsterLegacyElementalDuration,
		targetLine: "\n%s이 당신의 몸에 보마부를 그리며 주문을\n외웠습니다.\n갑자기 땅속에서 금의 수호령들이 올라와 보마진을 \n형성합니다.\n",
		roomLine:   "%s이 %s의 몸에 보마부를 그리며 주문을\n외웠습니다.\n갑자기 땅속에서 금의 수호령들이 올라와 보마진을 \n형성합니다.\n",
	})
}

func monsterCastResistColdSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       12,
		addTags:    []string{"PRCOLD", "resistCold"},
		expireTag:  "PRCOLD",
		duration:   monsterLegacyElementalDuration,
		targetLine: "\n%s이 당신의 입에 불타오르는 부적을 집어 넣으며\n방한주룰 외웁니다.\n당신의 주위에 오행중 화의 수호령들이 진을 형성하며\n주위를 둘러쌉니다.\n",
		roomLine:   "\n%s이 %s의 입에 불타오르는 부적을 집어넣으며 \n방한진 주문을 외웁니다.",
	})
}

func monsterCastBreatheWaterSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       12,
		addTags:    []string{"PBRWAT", "breatheWater"},
		expireTag:  "PBRWAT",
		duration:   monsterLegacyElementalDuration,
		targetLine: "\n%s이 당신에게 수생부를 먹이며 주문을 외웠습니다.\n당신의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜 수 있을 것 같습니다.\n",
		roomLine:   "\n%s이 %s에게 수생부를 먹이며 주문을 외웠습니다.\n그의 가슴이 평소보다 두배나 커져 물속에서 오랫동안\n견딜수 있을 것 같습니다.\n",
	})
}

func monsterCastEarthShieldSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:       12,
		addTags:    []string{"PSSHLD", "earthShield"},
		expireTag:  "PSSHLD",
		duration:   monsterLegacyElementalDuration,
		targetLine: "\n%s이 당신에게 토흙을 뿌리며 지방호 주문을 외웠습니다.\n갑자기 땅에서 오행중 토의 수호령들이 올라와 당신주위에\n진을 형성했습니다.\n",
		roomLine:   "\n%s이 %s에게 토흙을 뿌리며 지방호 주문을 외웁니다.\n땅에서 오행중 토의 수호령들이 올라와 그의 주위에\n진을 형성합니다.\n",
	})
}

func monsterCastKnowAlignmentSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetEffectSpell(world, c, targetPlayer, t, monsterTargetEffectSpec{
		cost:          6,
		addTags:       []string{"PKNOWA", "knowAlignment"},
		expireTag:     "PKNOWA",
		duration:      monsterLegacyElementalDuration,
		skipSpellFail: true,
		targetLine:    "\n%s이 당신에게 선악감지 주문을 외웁니다.\n당신은 선악을 감지할 수 있는 식별력이 높아졌습니다.\n",
		roomLine:      "\n%s이 %s에게 선악감지 주문을 외웁니다.\n그는 선악을 감지할 수 있는 식별력이 높아졌습니다.\n",
	})
}

func monsterCastRestoreSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureClass(c) < model.ClassInvincible {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok || targetPC.ID == c.ID {
		return 0
	}

	hp := creatureStat(targetPC, "hpCurrent") + dice(2, 10, 0)
	hpMax := creatureStat(targetPC, "hpMax")
	if hpMax > 0 && hp > hpMax {
		hp = hpMax
	}
	_ = world.SetCreatureStat(targetPC.ID, "hpCurrent", hp)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	if mrand(1, 100) < 60 {
		_ = world.SetCreatureStat(targetPC.ID, "mpCurrent", creatureStat(targetPC, "mpMax"))
		_ = world.BroadcastRoom(c.RoomID, sessionID, "\n"+c.DisplayName+"이 "+targetName+"에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n그의 도력이 회복되었습니다.\n")
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, "\n"+c.DisplayName+"이 당신에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n당신의 단전에 화기가 모이면서 도력이 회복됩니다.\n", false)
		}
	} else {
		_ = world.BroadcastRoom(c.RoomID, sessionID, "\n"+c.DisplayName+"이 "+targetName+"에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n하지만 아무런 반응도 일어나지 않습니다.\n")
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, "\n"+c.DisplayName+"이 당신에게 무화연 잎을 먹이며 도주천의 주문을 \n외웁니다.\n하지만 아무런 반응도 일어나지 않습니다.\n", false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastRecallSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	class := creatureClass(c)
	if creatureStat(c, "mpCurrent") < 30 || (class != model.ClassCleric && class < model.ClassInvincible) {
		return 0
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SCLERIC") {
		return 0
	}
	if _, ok := world.Creature(targetPlayer.CreatureID); !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-30)
	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, c.DisplayName+"이 당신에게 귀환 주문을 외웠습니다.\n", false)
	}
	_ = world.BroadcastRoom(c.RoomID, sessionID, c.DisplayName+"이 "+targetName+"에게 귀환 주문을 외웠습니다.")

	if _, ok := world.Room(monsterRecallTargetRoomID); !ok {
		return 0
	}
	if err := world.MovePlayerToRoom(targetPlayer.ID, monsterRecallTargetRoomID); err != nil {
		return 0
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastTeleportSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 20 {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-20)
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	if creatureHasAnyFlag(targetPC, "PRMAGI", "resistMagic") &&
		creatureLegacyLevel(c) < 128 &&
		((((creatureLegacyLevel(c)+3)/4)-((creatureLegacyLevel(targetPC)+3)/4))*10) < 50 {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-20)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, "\n"+c.DisplayName+"이 공간이동술을 사용하여 당신을 이동 시키려 합니다.\n", false)
		}
		return 0
	}

	destRoomID, ok := monsterRandomTeleportRoom(world, creatureLegacyLevel(c))
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-20)
	targetName := activePlayerDisplayName(world, targetPlayer)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, "\n"+c.DisplayName+"이 당신에게 공간이동술 주문을 외웠습니다.\n당신의 몸이 안개에 휩싸이며 어디론가로 이동됩니다.\n", false)
	}
	_ = world.BroadcastRoom(c.RoomID, sessionID, "\n"+c.DisplayName+"이 "+targetName+"에게 공간이동술 주문을 외웠습니다.\n그의 몸이 안개에 휩싸이며 모습이 사라졌습니다.\n")
	if err := world.MovePlayerToRoom(targetPlayer.ID, destRoomID); err != nil {
		return 0
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastEnchantSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	class := creatureClass(c)
	if class != model.ClassMage && class < model.ClassInvincible {
		return 0
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SMAGE") {
		return 0
	}
	if creatureStat(c, "mpCurrent") < 25 {
		return 0
	}

	object, objectName, ok := monsterFindInventoryObjectByName(world, c, activePlayerDisplayName(world, targetPlayer))
	if !ok {
		return 0
	}
	if objectHasAnyMonsterTag(world, object, "OENCHA", "oencha", "enchanted") {
		_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
		return 1
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-25)
	if !monsterApplyEnchantObject(world, c, object) {
		return 0
	}
	_ = world.BroadcastRoom(c.RoomID, "", c.DisplayName+"이 "+objectName+"에다가 주술을 걸었습니다.")
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastSummonSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	class := creatureClass(c)
	requiredMP := 50
	if class == model.ClassInvincible || class == model.ClassCaretaker {
		requiredMP = 100
	}
	if creatureStat(c, "mpCurrent") < requiredMP {
		return 0
	}
	if mrand(1, 100) < 51 {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-50)
		return 0
	}

	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok || targetPC.ID == c.ID || creatureHasAnyFlag(targetPC, "PDMINV", "dmInvisible") {
		return 0
	}
	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-requiredMP)

	room, ok := world.Room(c.RoomID)
	if !ok || monsterSummonRoomBlocksTarget(world, room, targetPC) {
		return 0
	}
	if minLevel, ok := monsterRoomIntProperty(room, "lolevel", "minLevel", "lowLevel"); ok && minLevel > creatureLegacyLevel(targetPC) {
		return 0
	}
	if maxLevel, ok := monsterRoomIntProperty(room, "hilevel", "maxLevel", "highLevel"); ok && maxLevel > 0 && creatureLegacyLevel(targetPC) > maxLevel {
		return 0
	}
	if creatureHasAnyFlag(targetPC, "PNOSUM", "noSummon") {
		return 0
	}
	if sourceRoom, ok := world.Room(targetPC.RoomID); ok && roomHasAnyFlag(sourceRoom, "RNOLEA", "rnolea", "noLeave") {
		return 0
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, "\n당신주위에 짙은 안개가 끼더니 알 수 없는 힘에 이끌려 어디론가 날라갑니다.\n안개가 걷히자 "+c.DisplayName+"이 당신앞에 서 있습니다.\n", false)
	}
	_ = world.BroadcastRoom(c.RoomID, sessionID, "\n"+c.DisplayName+"이 소환주문을 외우자 짙은 안개가 깔리더니 갑자기 "+targetName+"이 나타났습니다.\n")
	if err := world.MovePlayerToRoom(targetPlayer.ID, c.RoomID); err != nil {
		return 0
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastTrackSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	class := creatureClass(c)
	if class != model.ClassRanger && class < model.ClassInvincible {
		return 0
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SRANGER") {
		return 0
	}
	if creatureStat(c, "mpCurrent") < 13 {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok || targetPC.ID == c.ID || creatureHasAnyFlag(targetPC, "PDMINV", "dmInvisible") {
		return 0
	}
	if creatureClass(targetPC) > model.ClassCaretaker {
		return 0
	}
	targetRoom, ok := world.Room(targetPC.RoomID)
	if !ok || monsterTrackRoomBlocksCaster(world, targetRoom, c) {
		return 0
	}
	if minLevel, ok := monsterRoomIntProperty(targetRoom, "lolevel", "minLevel", "lowLevel"); ok && minLevel > creatureLegacyLevel(c) {
		return 0
	}
	if maxLevel, ok := monsterRoomIntProperty(targetRoom, "hilevel", "maxLevel", "highLevel"); ok && maxLevel > 0 && creatureLegacyLevel(c) > maxLevel {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-13)
	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, "\n"+c.DisplayName+"이 당신의 흔적을 찾아 내는데 성공하여 당신을 \n찾아 왔습니다.\n", false)
	}
	_ = world.BroadcastRoom(c.RoomID, sessionID, "\n"+c.DisplayName+"이 "+targetName+"의 흔적을 찾아내는데 성공하여 \n추적을 시작했습니다.\n")
	if err := world.MoveCreatureToRoom(c.ID, targetPC.RoomID); err != nil {
		return 0
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterTrackRoomBlocksCaster(world UpdateActiveWorld, room model.Room, caster model.Creature) bool {
	playerCount := activeVisiblePlayerCount(world, room)
	if roomHasAnyFlag(room, "RNOTEL", "rnotel", "noTeleport") ||
		(roomHasAnyFlag(room, "RONEPL", "ronepl", "onePlayer") && playerCount > 0) ||
		(roomHasAnyFlag(room, "RTWOPL", "rtwopl", "twoPlayers") && playerCount > 1) ||
		(roomHasAnyFlag(room, "RTHREE", "rthree", "threePlayers") && playerCount > 2) {
		return true
	}
	if roomHasAnyFlag(room, "RFAMIL", "rfamil", "family") && !creatureHasAnyFlag(caster, "PFAMIL", "family") {
		return true
	}
	if roomHasAnyFlag(room, "RONFML", "ronfml", "onlyFamily", "familyOnly") {
		roomSpecial, roomOK := monsterRoomIntProperty(room, "special")
		casterFamily, casterOK := creatureStatValue(caster, "dailyExpndMax")
		if !casterOK {
			casterFamily, casterOK = creatureStatValue(caster, "legacyDailyExpndMax")
		}
		if roomOK && (!casterOK || casterFamily != roomSpecial) {
			return true
		}
	}
	return false
}

func monsterSummonRoomBlocksTarget(world UpdateActiveWorld, room model.Room, target model.Creature) bool {
	playerCount := activeVisiblePlayerCount(world, room)
	if roomHasAnyFlag(room, "RNOTEL", "rnotel", "noTeleport") ||
		(roomHasAnyFlag(room, "RONEPL", "ronepl", "onePlayer") && playerCount > 0) ||
		(roomHasAnyFlag(room, "RTWOPL", "rtwopl", "twoPlayers") && playerCount > 1) ||
		(roomHasAnyFlag(room, "RTHREE", "rthree", "threePlayers") && playerCount > 2) {
		return true
	}
	if roomHasAnyFlag(room, "RFAMIL", "rfamil", "family") && !creatureHasAnyFlag(target, "PFAMIL", "family") {
		return true
	}
	if roomHasAnyFlag(room, "RONFML", "ronfml", "onlyFamily", "familyOnly") {
		roomSpecial, roomOK := monsterRoomIntProperty(room, "special")
		targetFamily, targetOK := creatureStatValue(target, "dailyExpndMax")
		if !targetOK {
			targetFamily, targetOK = creatureStatValue(target, "legacyDailyExpndMax")
		}
		if roomOK && (!targetOK || targetFamily != roomSpecial) {
			return true
		}
	}
	return false
}

func activeVisiblePlayerCount(world UpdateActiveWorld, room model.Room) int {
	if world == nil {
		return len(room.PlayerIDs)
	}
	count := 0
	seen := map[model.PlayerID]struct{}{}
	for _, playerID := range room.PlayerIDs {
		if playerID.IsZero() {
			continue
		}
		seen[playerID] = struct{}{}
		player, ok := world.Player(playerID)
		if !ok {
			count++
			continue
		}
		if activePlayerDMInvisible(world, player) {
			continue
		}
		count++
	}
	for _, creatureID := range room.CreatureIDs {
		creature, ok := world.Creature(creatureID)
		if !ok || creature.PlayerID.IsZero() {
			continue
		}
		if _, ok := seen[creature.PlayerID]; ok {
			continue
		}
		if creatureHasAnyFlag(creature, "PDMINV", "dmInvisible") {
			continue
		}
		count++
	}
	return count
}

func activePlayerDMInvisible(world UpdateActiveWorld, player model.Player) bool {
	if hasAnyNormalizedFlag(player.Metadata.Tags, "PDMINV", "dmInvisible") {
		return true
	}
	if player.CreatureID.IsZero() {
		return false
	}
	creature, ok := world.Creature(player.CreatureID)
	return ok && creatureHasAnyFlag(creature, "PDMINV", "dmInvisible")
}

func monsterFindInventoryObjectByName(world UpdateActiveWorld, c model.Creature, name string) (model.ObjectInstance, string, bool) {
	name = strings.TrimSpace(name)
	for _, objectID := range c.Inventory.ObjectIDs {
		object, ok := world.Object(objectID)
		if !ok {
			continue
		}
		displayName := monsterObjectDisplayName(world, object)
		if displayName == name || strings.EqualFold(displayName, name) {
			return object, displayName, true
		}
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			for _, keyword := range proto.Keywords {
				if keyword == name || strings.EqualFold(keyword, name) {
					if displayName == "" {
						displayName = keyword
					}
					return object, displayName, true
				}
			}
		}
	}
	return model.ObjectInstance{}, "", false
}

func monsterObjectDisplayName(world UpdateActiveWorld, object model.ObjectInstance) string {
	if strings.TrimSpace(object.DisplayNameOverride) != "" {
		return strings.TrimSpace(object.DisplayNameOverride)
	}
	if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
		return strings.TrimSpace(proto.DisplayName)
	}
	return strings.TrimSpace(string(object.ID))
}

func monsterApplyEnchantObject(world UpdateActiveWorld, c model.Creature, object model.ObjectInstance) bool {
	mutator, ok := world.(interface {
		UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
		SetObjectProperty(model.ObjectInstanceID, string, string) (model.ObjectInstance, error)
	})
	if !ok {
		return false
	}

	adj := 2
	class := creatureClass(c)
	if class == model.ClassMage || class >= model.ClassInvincible {
		adj = (((creatureLegacyLevel(c)+3)/4)-5)/5 + 1
		if adj > 3 {
			adj = 3
		}
	}
	if creatureHasAnyFlag(c, "YELLOWI", "yellowI") {
		adj = 4
	}
	if class >= model.ClassBulsa {
		adj = 5
	}

	currentAdj := monsterObjectIntProperty(world, object, "adjustment", "adjust")
	newAdj := adj
	if currentAdj > newAdj {
		newAdj = currentAdj
	}
	if _, err := mutator.SetObjectProperty(object.ID, "adjustment", strconv.Itoa(newAdj)); err != nil {
		return false
	}

	switch monsterObjectKind(world, object) {
	case model.ObjectKindArmor:
		armorInc := adj
		if monsterObjectIntProperty(world, object, "wearFlag", "wearflag", "wear") == 1 {
			armorInc = adj * 2
		}
		armor := monsterObjectIntProperty(world, object, "armor") + armorInc
		if _, err := mutator.SetObjectProperty(object.ID, "armor", strconv.Itoa(armor)); err != nil {
			return false
		}
	case model.ObjectKindWeapon:
		shotsMax := monsterObjectIntProperty(world, object, "shotsMax", "shotsmax") + adj*10
		shotsCurrent := monsterObjectIntProperty(world, object, "shotsCurrent", "shotscur") + adj*10
		pDice := monsterObjectIntProperty(world, object, "pDice", "pdice") + adj
		if _, err := mutator.SetObjectProperty(object.ID, "shotsMax", strconv.Itoa(shotsMax)); err != nil {
			return false
		}
		if _, err := mutator.SetObjectProperty(object.ID, "shotsCurrent", strconv.Itoa(shotsCurrent)); err != nil {
			return false
		}
		if _, err := mutator.SetObjectProperty(object.ID, "pDice", strconv.Itoa(pDice)); err != nil {
			return false
		}
	}

	value := monsterObjectIntProperty(world, object, "value") + 500*adj
	if _, err := mutator.SetObjectProperty(object.ID, "value", strconv.Itoa(value)); err != nil {
		return false
	}
	if _, err := mutator.UpdateObjectTags(object.ID, []string{"OENCHA", "oencha", "enchanted"}, nil); err != nil {
		return false
	}
	return true
}

func monsterObjectKind(world UpdateActiveWorld, object model.ObjectInstance) model.ObjectKind {
	if kind := strings.TrimSpace(object.Properties["kind"]); kind != "" {
		return model.ObjectKind(kind)
	}
	if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
		if proto.Kind != "" {
			return proto.Kind
		}
		if kind := strings.TrimSpace(proto.Properties["kind"]); kind != "" {
			return model.ObjectKind(kind)
		}
	}
	if typ := monsterObjectIntProperty(world, object, "type"); typ > 0 && typ <= 3 {
		return model.ObjectKindWeapon
	}
	return ""
}

func monsterObjectIntProperty(world UpdateActiveWorld, object model.ObjectInstance, keys ...string) int {
	for _, key := range keys {
		if n, ok := monsterParseIntProperty(object.Properties, key); ok {
			return n
		}
	}
	if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
		for _, key := range keys {
			if n, ok := monsterParseIntProperty(proto.Properties, key); ok {
				return n
			}
		}
	}
	return 0
}

func monsterParseIntProperty(properties map[string]string, key string) (int, bool) {
	if properties == nil {
		return 0, false
	}
	for propKey, value := range properties {
		if strings.EqualFold(propKey, key) {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			return n, err == nil
		}
	}
	return 0, false
}

func objectHasAnyMonsterTag(world UpdateActiveWorld, object model.ObjectInstance, names ...string) bool {
	if objectHasAnyFlag(world, object, names...) {
		return true
	}
	for _, name := range names {
		if monsterObjectIntProperty(world, object, name) != 0 {
			return true
		}
	}
	return false
}

func monsterRandomTeleportRoom(world UpdateActiveWorld, casterLevel int) (model.RoomID, bool) {
	lister, ok := world.(interface {
		AllRoomIDs() []model.RoomID
	})
	if !ok {
		return "", false
	}
	var valid []model.RoomID
	for _, roomID := range lister.AllRoomIDs() {
		room, ok := world.Room(roomID)
		if !ok {
			continue
		}
		if roomHasAnyFlag(room, "RNOTEL", "rnotel", "noTeleport") {
			continue
		}
		if minLevel, ok := monsterRoomIntProperty(room, "lolevel", "minLevel", "lowLevel"); ok && minLevel > casterLevel {
			continue
		}
		if maxLevel, ok := monsterRoomIntProperty(room, "hilevel", "maxLevel", "highLevel"); ok && maxLevel > 0 && casterLevel > maxLevel {
			continue
		}
		valid = append(valid, roomID)
	}
	if len(valid) == 0 {
		return "", false
	}
	return valid[mrand(0, len(valid)-1)], true
}

func monsterRoomIntProperty(room model.Room, keys ...string) (int, bool) {
	for _, key := range keys {
		if value, ok := room.Properties[key]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				return n, true
			}
		}
	}
	for _, tag := range room.Metadata.Tags {
		for _, key := range keys {
			prefix := key + ":"
			if strings.HasPrefix(tag, prefix) {
				if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(tag, prefix))); err == nil {
					return n, true
				}
			}
		}
	}
	return 0, false
}

func monsterCastTargetEffectSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64, spec monsterTargetEffectSpec) int {
	if creatureStat(c, "mpCurrent") < spec.cost {
		return 0
	}
	if !spec.skipSpellFail && legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-spec.cost)
		return 0
	}
	if spec.blockOnRoomEnemy && monsterActorHasRoomEnemy(world, c) {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-spec.cost)
	monsterAddPlayerStatusTags(world, targetPlayer, spec.addTags)
	if spec.expireTag != "" && spec.duration != nil {
		monsterSetEffectExpiration(world, targetPC.ID, spec.expireTag, t+spec.duration(world, c))
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	_ = world.BroadcastRoom(c.RoomID, sessionID, fmt.Sprintf(spec.roomLine, c.DisplayName, targetName))
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, fmt.Sprintf(spec.targetLine, c.DisplayName), false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastBlessSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetBuffSpell(world, c, targetPlayer, t, 10, monsterTargetBuffSpec{
		addTags:     []string{"PBLESS", "blessed"},
		expireTag:   "PBLESS",
		targetLine:  "\n%s이 당신의 머리에 한쪽손을 얹으며 성현주를 외웁니다.\n당신의 머리에서 삼매광이 뿜어져 나와 성스러운 기운이 몸을\n휘감습니다.\n",
		roomLine:    "\n%s이 %s의 머리에 한쪽손을 얹으며 성현주를 \n외웁니다.\n그의 머리에서 삼매광이 뿜어져 나와 성스러운 기운이 몸을\n휘감습니다.\n",
		recalculate: "thaco",
	})
}

func monsterCastProtectionSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	return monsterCastTargetBuffSpell(world, c, targetPlayer, t, 10, monsterTargetBuffSpec{
		addTags:     []string{"PPROTE", "protection"},
		expireTag:   "PPROTE",
		targetLine:  "%s이 당신의 몸에 수호인을 그리며 주문을 걸었습니다.\n빛의 수호령들이 당신의 주위를 둘러싸며 방어의 진을 형성했습니다.\n",
		roomLine:    "%s이 %s의 몸에 수호인을 그리며 주문을 걸었습니다.\n빛의 수호령들이 그의 주위를 둘러싸며 방어의 진을 형성했습니다.\n",
		recalculate: "ac",
	})
}

func monsterCastTargetBuffSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64, cost int, spec monsterTargetBuffSpec) int {
	if creatureStat(c, "mpCurrent") < cost {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-cost)
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-cost)
	monsterAddPlayerStatusTags(world, targetPlayer, spec.addTags)
	monsterSetEffectExpiration(world, targetPC.ID, spec.expireTag, t+monsterLegacyBuffDuration(world, c))
	switch spec.recalculate {
	case "thaco":
		_ = world.RecalculateTHACO(targetPC.ID)
	case "ac":
		_ = world.RecalculateAC(targetPC.ID)
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	_ = world.BroadcastRoom(c.RoomID, sessionID, fmt.Sprintf(spec.roomLine, c.DisplayName, targetName))
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, fmt.Sprintf(spec.targetLine, c.DisplayName), false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterLegacyBuffDuration(world UpdateActiveWorld, c model.Creature) int64 {
	interval := 1200 + legacyStatBonus(creatureStat(c, "intelligence"))*600
	if interval < 300 {
		interval = 300
	}
	class := creatureClass(c)
	if class == model.ClassCleric || class == model.ClassPaladin {
		interval += 60 * ((creatureLegacyLevel(c) + 3) / 4)
	}
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		interval += 800
	}
	return int64(interval)
}

func monsterLegacyLevitateDuration(world UpdateActiveWorld, c model.Creature) int64 {
	interval := 2400 + legacyStatBonus(creatureStat(c, "intelligence"))*600
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		interval += 800
	}
	return int64(interval)
}

func monsterLegacyElementalDuration(world UpdateActiveWorld, c model.Creature) int64 {
	interval := 1200 + legacyStatBonus(creatureStat(c, "intelligence"))*600
	if interval < 300 {
		interval = 300
	}
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		interval += 800
	}
	return int64(interval)
}

func monsterLegacyFlyDuration(world UpdateActiveWorld, c model.Creature) int64 {
	interval := 1200 + legacyStatBonus(creatureStat(c, "intelligence"))*600
	if interval < 300 {
		interval = 300
	}
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		interval += 600
	}
	return int64(interval)
}

func monsterLegacyInvisibilityDuration(world UpdateActiveWorld, c model.Creature) int64 {
	interval := 1200 + legacyStatBonus(creatureStat(c, "intelligence"))*600
	if creatureClass(c) == model.ClassMage {
		interval += 60 * ((creatureLegacyLevel(c) + 3) / 4)
	}
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		interval += 600
	}
	return int64(interval)
}

func monsterLegacyMageSightDuration(world UpdateActiveWorld, c model.Creature) int64 {
	interval := 1200 + legacyStatBonus(creatureStat(c, "intelligence"))*600
	if interval < 300 {
		interval = 300
	}
	if creatureClass(c) == model.ClassMage {
		interval += 60 * ((creatureLegacyLevel(c) + 3) / 4)
	}
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		interval += 600
	}
	return int64(interval)
}

func monsterActorHasRoomEnemy(world UpdateActiveWorld, actor model.Creature) bool {
	room, ok := world.Room(actor.RoomID)
	if !ok {
		return false
	}
	actorName := strings.TrimSpace(actor.DisplayName)
	if actorName == "" {
		return false
	}
	for _, id := range room.CreatureIDs {
		if id.IsZero() || id == actor.ID {
			continue
		}
		creature, ok := world.Creature(id)
		if !ok || creature.RoomID != room.ID || creature.Kind == model.CreatureKindPlayer || !creature.PlayerID.IsZero() {
			continue
		}
		enemies, err := world.CreatureEnemies(id)
		if err != nil {
			continue
		}
		for _, enemy := range enemies {
			if strings.TrimSpace(enemy) == actorName {
				return true
			}
		}
	}
	return false
}

func monsterCastLightSpell(world UpdateActiveWorld, c model.Creature, t int64) int {
	if creatureStat(c, "mpCurrent") < 5 {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-5)
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-5)
	_, _ = world.UpdateCreatureTags(c.ID, []string{"PLIGHT"}, nil)
	monsterSetEffectExpiration(world, c.ID, "PLIGHT", t+600)
	_ = world.BroadcastRoom(c.RoomID, "", "\n"+c.DisplayName+"이 한쪽 손에 발광 주문을 걸었습니다.\n그의 손에서 황금색의 찬란한 빛이 뿜어져 나옵니다.\n")
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastCurePoisonSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 6 {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-6)
		return 0
	}
	if _, ok := world.Creature(targetPlayer.CreatureID); !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-6)
	monsterRemovePlayerStatusTags(world, targetPlayer, []string{"PPOISN", "poison", "poisoned"})

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "의 혈도를 짚으면서 해독 주문을 외웁니다.\n그의 손가락 끝으로 검은 독기운이 빠져나오는 것이 보입니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := c.DisplayName + "이 당신의 혈도를 짚으면서 해독 주문을 외웁니다.\n당신의 손가락 끝으로 독기운이 빠져나가는 것이 느껴집니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastRemoveCurseSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 18 {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-18)
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-18)
	monsterRemoveCurseEquippedObjects(world, targetPC)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "의 등에 손을 대고 성스러운 \n기운을 주입합니다.\n그의 몸에서 느껴졌던 저주의 기운이 사라지는 것을\n느낄수 있습니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 당신의 몸에 손을 통해 성스러운 기운을\n주입합니다.\n당신의 몸에서 저주가 물러가는 것이 느껴집니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastRoomVigorSpell(world UpdateActiveWorld, c model.Creature, t int64) int {
	class := creatureClass(c)
	if class != model.ClassCleric && class < model.ClassInvincible {
		return 0
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SCLERIC") {
		return 0
	}
	if creatureStat(c, "mpCurrent") < 12 {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-12)
	if legacyMonsterSpellFails(c) {
		return 0
	}

	_ = world.BroadcastRoom(c.RoomID, "", "\n"+c.DisplayName+"이 가부좌를 틀고서 전회복 주문을 외웁니다.\n방안에 눈이 뜰 수 없을 정도의 빛이 가득차다가 사라집니다.\n방안의 모든사람이 체력이 회복되었는 것을 느낄수 있습니다.\n")

	heal := mrand(1, 6) + legacyStatBonus(creatureStat(c, "piety"))
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
		heal += mrand(1, 3)
	}
	if heal < 1 {
		heal = 1
	}

	room, ok := world.Room(c.RoomID)
	if ok {
		for _, playerID := range room.PlayerIDs {
			player, ok := world.Player(playerID)
			if !ok {
				continue
			}
			pc, ok := world.Creature(player.CreatureID)
			if !ok || pc.Kind == model.CreatureKindMonster {
				continue
			}
			if sessionID := activePlayerSessionID(world, player.ID); sessionID != "" {
				_ = world.WriteToSession(sessionID, "당신의 몸에서도 회복의 기운이 솟아오름을 느낄 수 있습니다.\n", false)
			}
			currentHP := creatureStat(pc, "hpCurrent")
			maxHP := creatureStat(pc, "hpMax")
			if maxHP < 1 {
				maxHP = 1
			}
			nextHP := currentHP + heal
			if nextHP > maxHP {
				nextHP = maxHP
			}
			_ = world.SetCreatureStat(pc.ID, "hpCurrent", nextHP)
		}
	}

	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastRemoveFearSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureClass(c) < model.ClassBulsa || creatureStat(c, "mpCurrent") < 100 {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-100)
		return 0
	}
	if _, ok := world.Creature(targetPlayer.CreatureID); !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-100)
	monsterRemovePlayerStatusTags(world, targetPlayer, []string{"PFEARS", "fear", "fearful"})

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "의 회복을 기원하며 공포해소 주문을 외우자\n" + targetName + "의 공포가 사라짐을 느낍니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := c.DisplayName + "이 당신에게 공포해소 주문을 외우자 당신의 겁이 사라집니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastCharmSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 15 {
		return 0
	}
	if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RSUVIV", "rsuviv", "survival") {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	duration := int64(100 + mrand(1, 30)*5 + legacyStatBonus(creatureStat(c, "intelligence"))*20)
	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-15)
	if creatureHasAnyFlag(targetPC, "PRMAGI", "resistMagic") {
		duration /= 2
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	if creatureLegacyLevel(c) < creatureLegacyLevel(targetPC) || creatureHasAnyFlag(targetPC, "MNOCHA", "noCharm") {
		roomMsg := c.DisplayName + "이 이혼대법을 " + targetName + "에게 걸려고 합니다.\n"
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			directMsg := c.DisplayName + "이 당신에게 이혼대법을 걸으려 합니다.\n"
			_ = world.WriteToSession(sessionID, directMsg, false)
		}
		return 0
	}

	monsterAddPlayerStatusTags(world, targetPlayer, []string{"PCHARM", "charmed"})
	_ = world.SetCreatureCooldown(targetPC.ID, "charmed", t, duration)

	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "에게 거울을 비추며 이혼대법을 겁니다.\n거울을 보고나자 당신을 보면서 괜히 히죽히죽\n거립니다. 저 자식이 미쳤나?"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 당신에게 거울을 비추며 이혼대법을 겁니다.\n괜히 기분이 좋아지면서 맞아도 황홀한 기분이\n듭니다. 나 좀 때려줘.."
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastCurseSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 25 {
		return 0
	}
	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-25)
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-25)
	monsterCurseEquippedObjects(world, targetPC)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "의 등에 손을 대고 저주의 기운을 불어 넣습니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 당신의 몸에 손을 통해 저주의 기운을 불어 넣습니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastRemoveDiseaseSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	class := creatureClass(c)
	if creatureStat(c, "mpCurrent") < 12 || (class != model.ClassCleric && class < model.ClassInvincible) {
		return 0
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SCLERIC") {
		return 0
	}

	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-12)
		return 0
	}
	if _, ok := world.Creature(targetPlayer.CreatureID); !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-12)
	monsterRemovePlayerStatusTags(world, targetPlayer, []string{"PDISEA", "disease", "diseased"})

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "의 혈도를 누르고 내공의 힘을 통해\n치료를 시작합니다.\n그의 몸이 차츰 활기를 띄기 시작하는 것이 보입니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 당신의 혈도를 누르고 내공의 힘을 통해 치료를 시작합니다.\n당신의 몸에 기공이 들어와 막힌 혈을 풀자 차츰 \n활기를 띄기 시작합니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastRemoveBlindnessSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	class := creatureClass(c)
	if creatureStat(c, "mpCurrent") < 12 || (class != model.ClassCleric && class != model.ClassPaladin && class < model.ClassInvincible) {
		return 0
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SCLERIC", "SPALADIN") {
		return 0
	}

	if legacyMonsterSpellFails(c) {
		_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-12)
		return 0
	}
	if _, ok := world.Creature(targetPlayer.CreatureID); !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-12)
	monsterRemovePlayerStatusTags(world, targetPlayer, []string{"PBLIND", "blind", "blinded"})

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "의 이마에 개안부를 붙히고서 \n주문을 외웁니다.\n그의 감겼던 눈이 움찔거리다가 갑자기 확 뜹니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := c.DisplayName + "이 당신의 이마에 개안부를 붙히고서 주문을\n외웁니다.\n감겼던 당신의 눈이 움찔거리다가 갑자기 밝아집니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterRemovePlayerStatusTags(world UpdateActiveWorld, player model.Player, remove []string) {
	_, _ = world.UpdatePlayerTags(player.ID, nil, remove)
	if !player.CreatureID.IsZero() {
		_, _ = world.UpdateCreatureTags(player.CreatureID, nil, remove)
	}
}

func monsterAddPlayerStatusTags(world UpdateActiveWorld, player model.Player, add []string) {
	_, _ = world.UpdatePlayerTags(player.ID, add, nil)
	if !player.CreatureID.IsZero() {
		_, _ = world.UpdateCreatureTags(player.CreatureID, add, nil)
	}
}

func monsterCurseEquippedObjects(world UpdateActiveWorld, c model.Creature) {
	tagger, ok := world.(interface {
		UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
	})
	if !ok {
		return
	}
	for _, objectID := range c.Equipment {
		if objectID.IsZero() {
			continue
		}
		_, _ = tagger.UpdateObjectTags(objectID, []string{"cursed", "ocurse"}, nil)
	}
}

func monsterRemoveCurseEquippedObjects(world UpdateActiveWorld, c model.Creature) {
	tagger, ok := world.(interface {
		UpdateObjectTags(model.ObjectInstanceID, []string, []string) (model.ObjectInstance, error)
	})
	if !ok {
		return
	}
	for _, objectID := range c.Equipment {
		if objectID.IsZero() {
			continue
		}
		_, _ = tagger.UpdateObjectTags(objectID, nil, []string{"cursed", "ocurse"})
	}
}

var legacyDissolveEquipmentSlotOrder = []string{
	"body",
	"arms",
	"legs",
	"neck1",
	"neck2",
	"hands",
	"head",
	"feet",
	"finger1",
	"finger2",
	"finger3",
	"finger4",
	"finger5",
	"finger6",
	"finger7",
	"finger8",
	"held",
	"shield",
	"face",
	"wield",
	"weapon",
	"mainHand",
	"right",
}

func legacyMonsterDissolveItem(world UpdateActiveWorld, monster model.Creature, targetPlayer model.Player) bool {
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok || len(targetPC.Equipment) == 0 {
		return false
	}

	candidates := make([]model.ObjectInstanceID, 0, len(targetPC.Equipment))
	seen := make(map[model.ObjectInstanceID]struct{}, len(targetPC.Equipment))
	for _, slot := range legacyDissolveEquipmentSlotOrder {
		objectID := targetPC.Equipment[slot]
		if objectID.IsZero() {
			continue
		}
		if _, ok := seen[objectID]; ok {
			continue
		}
		seen[objectID] = struct{}{}
		candidates = append(candidates, objectID)
	}
	if len(candidates) == 0 {
		return false
	}

	objectID := candidates[mrand(0, len(candidates)-1)]
	object, ok := world.Object(objectID)
	if !ok || objectHasAnyFlag(world, object, "OEVENT", "event", "eventItem") {
		return false
	}

	objectName := activeObjectDisplayName(world, object)
	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	_ = world.BroadcastRoom(monster.RoomID, sessionID, fmt.Sprintf("%s가 %s님의 %s를 부숴버립니다.", monster.DisplayName, targetName, objectName))
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, fmt.Sprintf("%s가 당신의 %s를 부숴버립니다.\n", monster.DisplayName, objectName), false)
	}
	if err := world.DestroyObject(objectID); err != nil {
		return false
	}
	_ = world.RecalculateAC(targetPC.ID)
	return true
}

func monsterSetEffectExpiration(world UpdateActiveWorld, creatureID model.CreatureID, tag string, expires int64) {
	expirer, ok := world.(interface {
		SetEffectExpiration(model.CreatureID, string, int64)
	})
	if !ok {
		return
	}
	expirer.SetEffectExpiration(creatureID, tag, expires)
}

func monsterCastBlindSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 15 || creatureClass(c) < model.ClassSubDM {
		return 0
	}
	if _, ok := world.Creature(targetPlayer.CreatureID); !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-15)
	if legacyMonsterSpellFails(c) {
		return 0
	}
	_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PBLIND", "blind"}, nil)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 손가락을 " + targetName + "의 눈을 향하고서 실명\n주문를 외웠습니다.\n검은안개같은 기운이 손가락에서 나와 그의 눈을 \n찌르자 괴성을 지릅니다. 악~~ 내눈..\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 손가락을 당신의 눈을 향하고서 실명 주문를 외웁니다.\n검은안개같은 기운이 손가락에서 나와 당신의 눈을\n 찌르자 괴성을 지릅니다. 악~~ 내눈..\n당신의 앞이 눈이 감겨서 보이질 않습니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastSilenceSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 12 || creatureClass(c) < model.ClassSubDM {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	duration := int64(3600)
	if creatureHasAnyFlag(targetPC, "PRMAGI", "resistMagic") {
		duration /= 2
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-12)
	if legacyMonsterSpellFails(c) {
		return 0
	}
	_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PSILNC", "silenced"}, nil)
	_ = world.SetCreatureCooldown(targetPC.ID, "silenced", t, duration)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 잽싸게 쫓아가 " + targetName + "의 목을 치면서 \n봉합구 주문을 외웁니다.\n그는 입을 벌려 말을 하려 하지만 목소리가 들이지 않습니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 잽싸게 쫓아와 당신의 목을 치면서 봉합구\n주문을 외웁니다.\n당신은 입을 벌려 말을 하려 하지만 목소리가 들이지 않습니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastFearSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 15 {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	duration := int64(600 + mrand(1, 30)*10 + legacyStatBonus(creatureStat(c, "intelligence"))*150)
	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-15)
	if legacyMonsterSpellFails(c) {
		return 0
	}
	if creatureHasAnyFlag(targetPC, "PRMAGI", "resistMagic") {
		duration /= 2
	}

	_, _ = world.UpdatePlayerTags(targetPlayer.ID, []string{"PFEARS", "fearful"}, nil)
	_ = world.SetCreatureCooldown(targetPC.ID, "fearful", t, duration)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "에게 지옥구술을 던졌습니다.\n구슬이 펑하고 터지자 갑자기 그가 괴성을 지릅니다. 악~~~ 저리가~~\n그는 공포에 떨지만 당신의 눈에는 아무것도 보이지 않습니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 당신에게 지옥구슬을 던졌습니다.\n갑자기 당신이 무서워하던 것들이 나타나 당신을 둘러쌉니다.\n\"악~~~ 저리가~~\" 당신은 괴성을 지르며 공포에 떨기\n시작합니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastBefuddleSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureStat(c, "mpCurrent") < 10 {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-10)
	if legacyMonsterSpellFails(c) {
		return 0
	}

	duration := legacyStatBonus(creatureStat(c, "intelligence")) + rollDice(2, 6, 0)
	if creatureHasAnyFlag(targetPC, "PRMAGI", "resistMagic") {
		duration = 3
	} else if duration < 5 {
		duration = 5
	}
	spellDuration := duration
	if spellDuration > 9 {
		spellDuration = 9
	}
	_ = world.SetCreatureCooldown(targetPC.ID, "befuddled", t, int64(duration))
	_ = world.SetCreatureCooldown(targetPC.ID, "spell", t, int64(spellDuration))
	_ = world.SetCreatureCooldown(targetPC.ID, "attack", t, int64(spellDuration))

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 흑기를 땅에 꼿으며 혼동술의 일종인 흑안법을 \n" + targetName + "에게 걸었습니다.\n주술을 걸자 갑자기 흑기에서 검은기류가 피어올라 그의\n정신을 혼수상태에 빠뜨립니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := "\n" + c.DisplayName + "이 흑기를 땅에 꼿으며 혼동술의 일종인 흑안법을 당신에게 걸었습니다.\n주술을 걸자 갑자기 흑기에서 검은기류가 피어올라 당신의\n정신을 혼수상태에 빠뜨립니다.\n"
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func monsterCastDrainExpSpell(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) int {
	if creatureClass(c) < model.ClassDM {
		return 0
	}
	targetPC, ok := world.Creature(targetPlayer.CreatureID)
	if !ok {
		return 0
	}

	levelTier := (creatureLegacyLevel(c) + 3) / 4
	loss := rollDice(levelTier, levelTier, 1) * 30
	currentExp := creatureStat(targetPC, "experience")
	if loss > currentExp {
		loss = currentExp
	}
	if loss < 0 {
		loss = 0
	}
	_ = world.SetCreatureStat(targetPC.ID, "experience", currentExp-loss)
	legacyMonsterLowerProficiency(world, targetPC, loss)

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	targetName := activePlayerDisplayName(world, targetPlayer)
	roomMsg := "\n" + c.DisplayName + "이 " + targetName + "에게 백치술의 주문을 외웁니다.\n그는 갑자기 멍청해진듯 주위를 두리번 거립니다.\n"
	_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
	if sessionID != "" {
		directMsg := fmt.Sprintf("\n%s이 당신에게 백치술의 주문을 외웁니다.\n당신은 갑자기 멍청해지면서 지금까지 싸워왔던 경험들이\n생각나지 않습니다.\n!!악~~~ 경험치가 얼마인지도 모르겠다.!!\n\n당신은 %d만큼의 경험들이 생각나지 않습니다.\n", c.DisplayName, loss)
		_ = world.WriteToSession(sessionID, directMsg, false)
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func legacyMonsterLowerProficiency(world UpdateActiveWorld, target model.Creature, loss int) {
	var proficiency [5]int
	var realms [4]int
	for i := range proficiency {
		proficiency[i] = creatureProficiency(target, i)
	}
	for i := range realms {
		realms[i] = creatureRealm(target, i)
	}
	proficiency, realms = legacy.LowerProficiency(proficiency, realms, loss)

	propWorld, _ := world.(updateActiveCreaturePropertyWorld)
	for i, value := range proficiency {
		keys := legacyMonsterWeaponProficiencyKeys(i)
		legacyMonsterWriteLowerProficiencySlot(world, propWorld, target, keys, keys[0], value)
	}
	for i, value := range realms {
		keys := legacyMonsterRealmKeys(i)
		legacyMonsterWriteLowerProficiencySlot(world, propWorld, target, keys, keys[0], value)
	}
}

type updateActiveCreaturePropertyWorld interface {
	SetCreatureProperty(model.CreatureID, string, string) (model.Creature, error)
}

func legacyMonsterWeaponProficiencyKeys(idx int) []string {
	if idx < 0 || idx >= len(legacyWeaponProficiencyStatKeys) {
		return []string{fmt.Sprintf("proficiency/%d", idx)}
	}
	part := legacyWeaponProficiencyPropertyKeys[idx]
	return []string{
		legacyWeaponProficiencyStatKeys[idx],
		fmt.Sprintf("proficiency/%s", part),
		fmt.Sprintf("proficiency.%s", part),
		fmt.Sprintf("proficiency_%s", part),
		fmt.Sprintf("proficiency/%d", idx),
		fmt.Sprintf("proficiency.%d", idx),
		fmt.Sprintf("proficiency_%d", idx),
		fmt.Sprintf("proficiency%d", idx),
	}
}

func legacyMonsterRealmKeys(idx int) []string {
	keys := []string{"realmEarth", "realmWind", "realmFire", "realmWater"}
	if idx >= 0 && idx < len(keys) {
		return []string{
			keys[idx],
			fmt.Sprintf("realm/%d", idx+1),
			fmt.Sprintf("realm.%d", idx+1),
			fmt.Sprintf("realm_%d", idx+1),
			fmt.Sprintf("realm%d", idx+1),
		}
	}
	return []string{fmt.Sprintf("realm/%d", idx+1)}
}

func legacyMonsterWriteLowerProficiencySlot(world UpdateActiveWorld, propWorld updateActiveCreaturePropertyWorld, target model.Creature, keys []string, defaultStatKey string, value int) {
	wrote := false
	for _, key := range keys {
		if _, ok := target.Stats[key]; ok {
			_ = world.SetCreatureStat(target.ID, key, value)
			wrote = true
		}
	}
	if propWorld != nil {
		for _, key := range keys {
			if _, ok := target.Properties[key]; ok {
				_, _ = propWorld.SetCreatureProperty(target.ID, key, strconv.Itoa(value))
				wrote = true
			}
		}
	}
	if !wrote && value != 0 {
		_ = world.SetCreatureStat(target.ID, defaultStatKey, value)
	}
}

func legacyMonsterSpellFails(c model.Creature) bool {
	class := creatureClass(c)
	level := creatureLegacyLevel(c)
	bns := legacyStatBonus(creatureStat(c, "intelligence"))

	chance := 0
	switch class {
	case model.ClassAssassin:
		chance = (((level+3)/4)+bns)*5 + 30
	case model.ClassBarbarian:
		chance = (((level + 3) / 4) + bns) * 5
	case model.ClassCleric:
		chance = (((level+3)/4)+bns)*5 + 65
	case model.ClassFighter:
		chance = (((level+3)/4)+bns)*5 + 10
	case model.ClassMage:
		chance = (((level+3)/4)+bns)*5 + 75
	case model.ClassPaladin:
		chance = (((level+3)/4)+bns)*5 + 50
	case model.ClassRanger:
		chance = (((level+3)/4)+bns)*4 + 56
	case model.ClassThief:
		chance = (((level+3)/4)+bns)*6 + 22
	default:
		return false
	}
	return mrand(1, 100) > chance
}

func monsterCastHealingSpell(world UpdateActiveWorld, c model.Creature, tag string, t int64) int {
	nextHP, cost, roomMsg, ok := legacyMonsterHealingResult(world, c, tag)
	if !ok {
		return 0
	}
	_ = world.SetCreatureStat(c.ID, "hpCurrent", nextHP)
	_ = world.SetCreatureStat(c.ID, "mpCurrent", creatureStat(c, "mpCurrent")-cost)
	_ = world.BroadcastRoom(c.RoomID, "", roomMsg)
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
	return 1
}

func legacyMonsterHealingResult(world UpdateActiveWorld, c model.Creature, tag string) (int, int, string, bool) {
	currentHP := creatureStat(c, "hpCurrent")
	maxHP := creatureStat(c, "hpMax")
	if maxHP < 1 {
		maxHP = 1
	}

	switch tag {
	case "SVIGOR":
		if creatureStat(c, "mpCurrent") < 5 || !legacyMonsterCanUseClericHeal(c) {
			return 0, 0, "", false
		}
		class := creatureClass(c)
		levelTier := (c.Level + 3) / 4
		clericBonus := 0
		if class == model.ClassCleric {
			clericBonus = levelTier + mrand(1, 1+levelTier/2)
		}
		paladinBonus := 0
		if class == model.ClassPaladin {
			paladinBonus = levelTier/2 + mrand(1, 1+levelTier/4)
		}
		maxBonus := legacyStatBonus(creatureStat(c, "intelligence"))
		if pietyBonus := legacyStatBonus(creatureStat(c, "piety")); pietyBonus > maxBonus {
			maxBonus = pietyBonus
		}
		heal := rollDice(maxBonus+10, clericBonus+paladinBonus+1, mrand(1, 6))
		if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			heal += mrand(1, 10)
		}
		if heal < 1 {
			heal = 1
		}
		nextHP := currentHP + heal
		if nextHP > maxHP {
			nextHP = maxHP
		}
		return nextHP, 5, "\n" + c.DisplayName + "이 합장을 하고서 주문을 외웁니다.\n빛의 정기가 그의 몸으로 모이는 것이 보입니다.\n", true
	case "SMENDW":
		if creatureStat(c, "mpCurrent") < 10 {
			return 0, 0, "", false
		}
		class := creatureClass(c)
		levelTier := (c.Level + 3) / 4
		invincibleBonus := 0
		if class >= model.ClassInvincible {
			invincibleBonus = mrand(1, (c.Level+24)/25)
		}
		clericBonus := 0
		if class == model.ClassCleric {
			clericBonus = levelTier*2 + mrand(1, 1+levelTier/2)
		}
		paladinBonus := 0
		if class == model.ClassPaladin {
			paladinBonus = levelTier + mrand(1, 1+levelTier/3)
		}
		heal := rollDice(legacyStatBonus(creatureStat(c, "intelligence"))+legacyStatBonus(creatureStat(c, "piety"))+20, invincibleBonus+clericBonus+paladinBonus+1, rollDice(2, 6, 5))
		if room, ok := world.Room(c.RoomID); ok && roomHasAnyFlag(room, "RPMEXT", "rpmext") {
			heal += mrand(1, 10) + 1
		}
		if heal < 1 {
			heal = 1
		}
		if c.Kind != model.CreatureKindPlayer {
			heal /= 2
		}
		if heal < 1 {
			heal = 1
		}
		nextHP := currentHP + heal
		if nextHP > maxHP {
			nextHP = maxHP
		}
		return nextHP, 10, "\n" + c.DisplayName + "이 기공팔식의 자세를 취하며 원기회복의 주문을 외웁니다.\n지기의 뜨거운 기운이 그에게 흘러가는 것이 느껴집니다.\n", true
	case "SFHEAL":
		if creatureStat(c, "mpCurrent") < 50 || !legacyMonsterCanUseClericHeal(c) || currentHP == maxHP {
			return 0, 0, "", false
		}
		return maxHP, 50, "\n" + c.DisplayName + "이 천부공 자세를 취하면서 완치주문을 외웠습니다.\n천상의 기운들이 그에게로 모이는 것이 느껴집니다.\n", true
	default:
		return 0, 0, "", false
	}
}

func legacyMonsterCanUseClericHeal(c model.Creature) bool {
	class := creatureClass(c)
	if class != model.ClassCleric && class != model.ClassPaladin && class < model.ClassInvincible {
		return false
	}
	if class >= model.ClassInvincible && !creatureHasAnyFlag(c, "SCLERIC", "SPALADIN") {
		return false
	}
	return true
}

func legacyMonsterOffensiveSpellRoomDetail(c model.Creature, tag string) string {
	subject := c.DisplayName + "이"
	possessive := c.DisplayName + "의"
	switch tag {
	case "SHURTS":
		return "\n" + subject + " 주문을 외우자 북방으로부터 칼날과 같은 거센 바람이\n불어 명령에 따라 공격합니다."
	case "SFIREB":
		return "\n" + subject + " 손을 끌어 기를 모으자 손 끝에서 적색의 불꽃이 공간을\n가르며 적에게 날라갑니다."
	case "SLGHTN":
		return "\n" + subject + " 양손의 검지를 하늘로 향해 모으자 양쪽눈에서 푸른번개가\n작렬하면서 날라 갑니다."
	case "SICEBL":
		return "\n" + subject + " 주문을 외우며 부적에 도력을 모아 하늘로 날리자 모든 것을\n얼릴 듯 한 눈보라가 휘몰아 칩니다."
	case "SSHOCK":
		return "\n" + subject + " 주문을 외우며 손을 내밀자 한풍이 소용둘이를 일으키며\n주의를 쓸어 버립니다."
	case "SRUMBL":
		return "\n" + subject + " 주문을 외우자 땅위의 있는 수만마리의 벌레들이 적의 \n기운을 쫓아 공격을 합니다."
	case "SBURNS":
		return "\n" + possessive + " 등에 숨겨져 있던 붉은 깃발들이 하늘로 날아올라 진을\n형성하자 적의 몸이 불타 오릅니다."
	case "SBLIST":
		return "\n" + possessive + " 손 끝에 빛나는 이슬이 맺히면서 그것을 튕기자 총알같이\n날라가 적의 몸을 꿰뚫어 버립니다."
	case "SDUSTG":
		return "\n" + subject + " 주문을 외무며 몸을 돌리자 갑자기 검은 태풍이 날라와\n적의 몸을 삼켜 버립니다."
	case "SWBOLT":
		return "\n" + possessive + " 소매에 숨겨져있던 얇은 검을 뽑으며 검초를 뿌리자 \n그 안에 숨겨져 있던 수의 기운들이 상대방에게 분출됩니다."
	case "SCRUSH":
		return "\n" + subject + " 목검을 땅에 꽂자 땅이 갈라지면서 지룡이 올라와 날카로운\n손톱으로 적을 공격합니다."
	case "SENGUL":
		return "\n" + possessive + " 옆에 있던 산만한 바위가 갑자기 폭파하면서 커다란\n바위들이 적에게 떨어집니다."
	case "SBURST":
		return "\n" + subject + " 수많은 부적을 태우며 하늘로 날리자 화염이 불타오르는\n커다란 회오리 바람이 적을 둘러쌉니다."
	case "SSTEAM":
		return "\n" + possessive + " 몸이 화룡으로 변하면서 불꽃이 타오르는 몸으로\n적을 공격합니다."
	case "SSHATT":
		return "\n" + subject + " 갑자기 적의 바로 밑으로 지진이 일어나 땅이 갈라지면서\n적을 삼켜버립니다."
	case "SIMMOL":
		return "\n" + subject + " 주문을 외우자 갑자기 불꽃을 내뿜는 주작이 내려와\n대지를 불태워 버립니다."
	case "SBLOOD":
		return "\n" + subject + " 주문을 외우자 잠잠하던 땅이 흔들리더니 갑자기 용암이\n분출하자 적은 놀라 그곳에 빠집니다."
	case "STHUND":
		return "\n" + subject + " 주문을 외우자 갑자기 검은 구름이 나타나 천지를 \n붉은 벼락이 진동하면서 적을 강타합니다."
	case "SEQUAK":
		return "\n" + possessive + " 주위가 검은 안개로 싸이며 검을 든 33명의 야차가 나타나 적을 무참히 도륙해 버립니다."
	case "SFLFIL":
		return "\n" + subject + " 눈을 감고 주문을 외우자 강렬한 빛을 내뿜는 삼지안이 열리면서 모든 것을 불태워 버립니다."
	default:
		return ""
	}
}

func applyMonsterSpellDamageToPlayer(world UpdateActiveWorld, player model.Player, monster model.Creature, spellName string, damage int) {
	pc, ok := world.Creature(player.CreatureID)
	if !ok {
		return
	}
	_, applied, dead, _ := world.ApplyCreatureDamage(pc.ID, damage)
	_ = world.RecordCreatureDamage(pc.ID, monster.ID, applied)

	sessionID := activePlayerSessionID(world, player.ID)
	if sessionID != "" {
		_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s이 %s 주술로 당신에게 %d만큼의 피해를 주었습니다.\n", monster.DisplayName, spellName, damage), false)
	}

	if dead {
		_ = killPlayer(world, player, monster)
	} else {
		_, _ = world.AddEnemy(monster.ID, pc.ID)
		_, _ = world.AddEnemy(pc.ID, monster.ID)
		_ = world.RecalculateAC(monster.ID)
		_ = world.RecalculateTHACO(pc.ID)

		if creatureHasAnyFlag(pc, "PWIMPY") {
			wimpyValue := pc.Stats["wimpyValue"]
			if wimpyValue == 0 {
				wimpyValue = 10
			}
			if latest, ok := world.Creature(pc.ID); ok {
				hpCur := latest.Stats["hpCurrent"]
				if hpCur > 0 && hpCur <= wimpyValue {
					if disp, ok := world.(interface {
						DispatchCommand(sessionID session.ID, playerID model.PlayerID, line string) error
					}); ok {
						_ = disp.DispatchCommand(sessionID, player.ID, "도망")
					}
				}
			}
		}
	}
}

func crtPoison(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) {
	targetPC, _ := world.Creature(targetPlayer.CreatureID)
	n := targetPC.Stats["hpCurrent"] / mrand(3, 6)
	if n > targetPC.Stats["hpCurrent"] {
		n = targetPC.Stats["hpCurrent"]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	mPart := krtext.Particle(c.DisplayName, '1')
	targetName := activePlayerDisplayName(world, targetPlayer)

	if n > 30 {
		_, applied, dead, _ := world.ApplyCreatureDamage(targetPC.ID, n)
		_ = world.RecordCreatureDamage(targetPC.ID, c.ID, applied)
		_ = world.SetCreatureCooldown(targetPC.ID, "spell", t, int64(applied/10))
		_ = world.SetCreatureCooldown(targetPC.ID, "attack", t, int64(applied/20))

		roomMsg := fmt.Sprintf("\n%s%s %s에게 독을 뿌려서 %d의 피해를 입혔습니다.\n%s의 몸이 중독되었습니다.", c.DisplayName, mPart, targetName, applied, targetName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 독을 뿌려서 %d의 피해를 입혔습니다.\n 당신의 몸이 중독되었습니다.", c.DisplayName, mPart, applied), false)
		}
		if dead {
			_ = killPlayer(world, targetPlayer, c)
		}
	} else {
		roomMsg := fmt.Sprintf("\n%s의 독살포가 실패했습니다.", c.DisplayName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s  당신에게 독살포로 공격하려고 합니다.", c.DisplayName, mPart), false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 5)
}

func crtKick(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) {
	targetPC, _ := world.Creature(targetPlayer.CreatureID)
	n := rollDice(c.Stats["nDice"], c.Stats["sDice"], c.Stats["pDice"]) * 4
	if n > targetPC.Stats["hpCurrent"] {
		n = targetPC.Stats["hpCurrent"]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	mPart := krtext.Particle(c.DisplayName, '1')
	targetName := activePlayerDisplayName(world, targetPlayer)

	if n > 30 {
		if creatureClass(targetPC) > model.ClassCaretaker {
			n = 1
		}
		_, applied, dead, _ := world.ApplyCreatureDamage(targetPC.ID, n)
		_ = world.RecordCreatureDamage(targetPC.ID, c.ID, applied)

		roomMsg := fmt.Sprintf("\n%s%s %s에게 발차기로 %d점의 공격을 가합니다.", c.DisplayName, mPart, targetName, applied)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 발차기로 당신에게 %d점의 공격을 가했습니다.", c.DisplayName, mPart, applied), false)
		}
		if dead {
			_ = killPlayer(world, targetPlayer, c)
		}
	} else {
		roomMsg := fmt.Sprintf("\n%s의 발차기가 실패했습니다.", c.DisplayName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s  당신에게 발차기로 공격하려고 합니다.", c.DisplayName, mPart), false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 3)
}

func crtTurn(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) {
	targetPC, _ := world.Creature(targetPlayer.CreatureID)
	n := targetPC.Stats["hpCurrent"] / mrand(3, 6)
	if n > targetPC.Stats["hpCurrent"] {
		n = targetPC.Stats["hpCurrent"]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	mPart := krtext.Particle(c.DisplayName, '1')
	targetName := activePlayerDisplayName(world, targetPlayer)

	if n > 30 {
		_, applied, dead, _ := world.ApplyCreatureDamage(targetPC.ID, n)
		_ = world.RecordCreatureDamage(targetPC.ID, c.ID, applied)

		roomMsg := fmt.Sprintf("\n%s%s 부적을 하늘로 날리며 혼을 소환시키는 방혼술의 주문을 외칩니다.\n부적이 %s의 몸을 공격하며%d만큼의 타격을 입혔습니다.\n", c.DisplayName, mPart, targetName, applied)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 부적을 하늘로 날리며 혼을 소환하는 방혼술의 주문을 외칩니다.\n부적이 당신을 공격하며 %d만큼의 타격을 입혔습니다..", c.DisplayName, mPart, applied), false)
		}
		if dead {
			_ = killPlayer(world, targetPlayer, c)
		}
	} else {
		roomMsg := fmt.Sprintf("\n%s%s 부적을 하늘로 날리며 혼을 소환시키는 방혼술의 주문을 외칩니다.\n하지만 주문이 튕겨져 나오면서 %s이 그의 주술을 견뎌냈습니다.\n", c.DisplayName, mPart, targetName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n당신은 %s의 방혼술을 견뎌냈습니다.", c.DisplayName), false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 15)
}

func crtBash(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) {
	targetPC, _ := world.Creature(targetPlayer.CreatureID)
	n := 0
	weapon, hasWeapon := findMonsterWeapon(world, c)
	if hasWeapon {
		nd := activeObjectIntProp(world, weapon, "nDice")
		sd := activeObjectIntProp(world, weapon, "sDice")
		pd := activeObjectIntProp(world, weapon, "pDice")
		n = rollDice(nd, sd, pd)*3 + rollDice(c.Stats["nDice"], c.Stats["sDice"], c.Stats["pDice"])
	} else {
		n = rollDice(c.Stats["nDice"], c.Stats["sDice"], c.Stats["pDice"]) * 3
	}

	if n > targetPC.Stats["hpCurrent"] {
		n = targetPC.Stats["hpCurrent"]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	mPart := krtext.Particle(c.DisplayName, '1')
	targetName := activePlayerDisplayName(world, targetPlayer)

	if n > 20 {
		_, applied, dead, _ := world.ApplyCreatureDamage(targetPC.ID, n)
		_ = world.RecordCreatureDamage(targetPC.ID, c.ID, applied)

		roomMsg := fmt.Sprintf("\n%s%s %s에게 칼을 휘둘러 %d점의 맹공을 가합니다.", c.DisplayName, mPart, targetName, applied)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신에게 %d점의 맹공을 가합니다.", c.DisplayName, mPart, applied), false)
		}
		if dead {
			_ = killPlayer(world, targetPlayer, c)
		}
	} else {
		roomMsg := fmt.Sprintf("\n%s의 맹공이 실패했습니다.", c.DisplayName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s  당신에게 맹공으로 공격하려고 합니다.", c.DisplayName, mPart), false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 5)
}

func crtAbsorb(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) {
	targetPC, _ := world.Creature(targetPlayer.CreatureID)
	n := ((c.Level + 3) / 4) * mrand(2, 5)
	if n > targetPC.Stats["hpCurrent"] {
		n = targetPC.Stats["hpCurrent"]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	mPart := krtext.Particle(c.DisplayName, '1')
	targetName := activePlayerDisplayName(world, targetPlayer)

	if n > 20 {
		_, applied, dead, _ := world.ApplyCreatureDamage(targetPC.ID, n)
		_ = world.RecordCreatureDamage(targetPC.ID, c.ID, applied)

		newHp := c.Stats["hpCurrent"] + applied
		if newHp > c.Stats["hpMax"] {
			newHp = c.Stats["hpMax"]
		}
		_ = world.SetCreatureStat(c.ID, "hpCurrent", newHp)

		roomMsg := fmt.Sprintf("\n%s%s %s의 기를 %d만큼 흡수했습니다.", c.DisplayName, mPart, targetName, applied)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신의 기를 %d만큼 흡수했습니다.", c.DisplayName, mPart, applied), false)
		}
		if dead {
			_ = killPlayer(world, targetPlayer, c)
		}
	} else {
		roomMsg := fmt.Sprintf("\n%s의 흡성대법이 실패했습니다.", c.DisplayName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s  당신에게 흡성대법으로 공격하려고 합니다.", c.DisplayName, mPart), false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 7)
}

func crtMagicStop(world UpdateActiveWorld, c model.Creature, targetPlayer model.Player, t int64) {
	targetPC, _ := world.Creature(targetPlayer.CreatureID)
	n := targetPC.Stats["hpCurrent"] / mrand(2, 5)
	if n > targetPC.Stats["hpCurrent"] {
		n = targetPC.Stats["hpCurrent"]
	}

	sessionID := activePlayerSessionID(world, targetPlayer.ID)
	mPart := krtext.Particle(c.DisplayName, '1')
	targetName := activePlayerDisplayName(world, targetPlayer)

	if n > 50 {
		_, applied, dead, _ := world.ApplyCreatureDamage(targetPC.ID, n)
		_ = world.RecordCreatureDamage(targetPC.ID, c.ID, applied)
		_ = world.SetCreatureCooldown(targetPC.ID, "spell", t, int64(applied/20))

		roomMsg := fmt.Sprintf("\n%s%s %s의 급소를 짚어서 %d점의 피해를 입혔습니다.", c.DisplayName, mPart, targetName, applied)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s 당신의 급소를 짚어서 %d점의 피해를 입혔습니다.", c.DisplayName, mPart, applied), false)
		}
		if dead {
			_ = killPlayer(world, targetPlayer, c)
		}
	} else {
		roomMsg := fmt.Sprintf("\n%s의 혈도봉괘가 실패했습니다.", c.DisplayName)
		_ = world.BroadcastRoom(c.RoomID, sessionID, roomMsg)
		if sessionID != "" {
			_ = world.WriteToSession(sessionID, fmt.Sprintf("\n%s%s  당신에게 혈도봉쇄로 공격하려고 합니다.", c.DisplayName, mPart), false)
		}
	}
	_ = world.SetCreatureCooldown(c.ID, "attack", t, 10)
}

// Helpers

func activeObjectDisplayName(world UpdateActiveWorld, object model.ObjectInstance) string {
	if name := strings.TrimSpace(object.DisplayNameOverride); name != "" {
		return name
	}
	if name := strings.TrimSpace(object.Properties["name"]); name != "" {
		return name
	}
	if !object.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(object.PrototypeID); ok {
			if name := strings.TrimSpace(proto.DisplayName); name != "" {
				return name
			}
			if name := strings.TrimSpace(proto.Properties["name"]); name != "" {
				return name
			}
		}
	}
	return "물건"
}

func activeObjectIntProp(world UpdateActiveWorld, obj model.ObjectInstance, key string) int {
	if val, ok := obj.Properties[key]; ok {
		if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return i
		}
	}
	if !obj.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(obj.PrototypeID); ok {
			if val, ok := proto.Properties[key]; ok {
				if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
					return i
				}
			}
		}
	}
	return 0
}

func findMonsterWeapon(world UpdateActiveWorld, c model.Creature) (model.ObjectInstance, bool) {
	for _, oid := range c.Inventory.ObjectIDs {
		if obj, ok := world.Object(oid); ok {
			displayName := activeObjectDisplayName(world, obj)
			if strings.Contains(displayName, "검") || strings.Contains(displayName, "도") {
				return obj, true
			}
		}
	}
	return model.ObjectInstance{}, false
}

func rollDice(nDice, sDice, pDice int) int {
	if nDice < 0 {
		nDice = 0
	}
	if sDice < 0 {
		sDice = 0
	}
	damage := pDice
	if nDice > 0 && sDice > 0 {
		for i := 0; i < nDice; i++ {
			damage += mrand(1, sDice)
		}
	}
	if damage < 0 {
		return 0
	}
	return damage
}

func strengthDamageBonus(attacker model.Creature) int {
	strength, ok := attacker.Stats["strength"]
	if !ok {
		return 0
	}
	if strength < 0 {
		strength = 0
	}
	if strength >= len(legacyBonus) {
		strength = len(legacyBonus) - 1
	}
	return legacyBonus[strength]
}

// C-MUD utility check
func is_charm_crt(cName string, pc model.Creature) bool {
	// Simple mock or tag check for charm
	for _, tag := range pc.Metadata.Tags {
		if strings.HasPrefix(strings.ToLower(tag), "charm:") {
			return strings.TrimPrefix(strings.ToLower(tag), "charm:") == strings.ToLower(cName)
		}
	}
	return false
}

func objectHasAnyFlag(world UpdateActiveWorld, obj model.ObjectInstance, flags ...string) bool {
	targets := normalizedFlagSet(flags...)
	if hasAnyNormalizedFlag(obj.Metadata.Tags, flags...) {
		return true
	}
	for key, val := range obj.Properties {
		if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(val) {
			return true
		}
		if objectFlagContainerProperty(key) {
			for _, tok := range strings.FieldsFunc(val, func(r rune) bool {
				return r == ',' || r == ';' || r == '|' || r == ' '
			}) {
				if _, ok := targets[normalizeFlagName(tok)]; ok {
					return true
				}
			}
		}
	}
	if !obj.PrototypeID.IsZero() {
		if proto, ok := world.ObjectPrototype(obj.PrototypeID); ok {
			if hasAnyNormalizedFlag(proto.Metadata.Tags, flags...) {
				return true
			}
			for key, val := range proto.Properties {
				if _, ok := targets[normalizeFlagName(key)]; ok && propertyFlagEnabled(val) {
					return true
				}
				if objectFlagContainerProperty(key) {
					for _, tok := range strings.FieldsFunc(val, func(r rune) bool {
						return r == ',' || r == ';' || r == '|' || r == ' '
					}) {
						if _, ok := targets[normalizeFlagName(tok)]; ok {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func activePlayerSessionID(world UpdateActiveWorld, playerID model.PlayerID) session.ID {
	for _, active := range world.ActiveSessions() {
		if active.ActorID == string(playerID) {
			return active.ID
		}
	}
	return ""
}

func activePlayerDisplayName(world UpdateActiveWorld, player model.Player) string {
	if !player.CreatureID.IsZero() {
		if creature, ok := world.Creature(player.CreatureID); ok {
			if name := strings.TrimSpace(creature.DisplayName); name != "" {
				return name
			}
		}
	}
	if name := strings.TrimSpace(player.DisplayName); name != "" {
		return name
	}
	return string(player.ID)
}

// Methods on *loopUpdateActiveWorld to satisfy UpdateActiveWorld interface

func (w *loopUpdateActiveWorld) ActiveCreatures() []model.Creature {
	return w.w.ActiveCreatures()
}
func (w *loopUpdateActiveWorld) Creature(id model.CreatureID) (model.Creature, bool) {
	return w.w.Creature(id)
}
func (w *loopUpdateActiveWorld) Player(id model.PlayerID) (model.Player, bool) {
	return w.w.Player(id)
}
func (w *loopUpdateActiveWorld) Room(id model.RoomID) (model.Room, bool) {
	return w.w.Room(id)
}
func (w *loopUpdateActiveWorld) MovePlayerToRoom(playerID model.PlayerID, roomID model.RoomID) error {
	return w.w.MovePlayerToRoom(playerID, roomID)
}
func (w *loopUpdateActiveWorld) MoveCreatureToRoom(creatureID model.CreatureID, roomID model.RoomID) error {
	return w.w.MoveCreatureToRoom(creatureID, roomID)
}
func (w *loopUpdateActiveWorld) ApplyCreatureDamage(creatureID model.CreatureID, damage int) (model.Creature, int, bool, error) {
	return w.w.ApplyCreatureDamage(creatureID, damage)
}
func (w *loopUpdateActiveWorld) RecordCreatureDamage(victimID, attackerID model.CreatureID, damage int) error {
	return w.w.RecordCreatureDamage(victimID, attackerID, damage)
}
func (w *loopUpdateActiveWorld) UpdateCreatureTags(creatureID model.CreatureID, add, remove []string) (model.Creature, error) {
	return w.w.UpdateCreatureTags(creatureID, add, remove)
}
func (w *loopUpdateActiveWorld) UpdatePlayerTags(playerID model.PlayerID, add, remove []string) (model.Player, error) {
	return w.w.UpdatePlayerTags(playerID, add, remove)
}
func (w *loopUpdateActiveWorld) SetCreatureStat(creatureID model.CreatureID, name string, val int) error {
	return w.w.SetCreatureStat(creatureID, name, val)
}
func (w *loopUpdateActiveWorld) SetCreatureProperty(creatureID model.CreatureID, key string, value string) (model.Creature, error) {
	return w.l.SetCreatureProperty(creatureID, key, value)
}
func (w *loopUpdateActiveWorld) UpdateRoomProperty(roomID model.RoomID, key string, value string) error {
	return w.l.UpdateRoomProperty(roomID, key, value)
}
func (w *loopUpdateActiveWorld) SpawnCreature(protoID model.CreatureID, roomID model.RoomID, carryItems bool) (model.CreatureID, error) {
	return w.l.SpawnCreature(protoID, roomID, carryItems)
}
func (w *loopUpdateActiveWorld) DBRoot() string {
	if w == nil || w.w == nil {
		return ""
	}
	return w.w.DBRoot()
}
func (w *loopUpdateActiveWorld) CreatureEnemies(creatureID model.CreatureID) ([]string, error) {
	return w.w.CreatureEnemies(creatureID)
}
func (w *loopUpdateActiveWorld) AddEnemy(attacker, defender model.CreatureID) (bool, error) {
	return w.w.AddEnemy(attacker, defender)
}
func (w *loopUpdateActiveWorld) RemoveEnemy(creatureID model.CreatureID, enemyName string) error {
	return w.w.RemoveEnemy(creatureID, enemyName)
}
func (w *loopUpdateActiveWorld) ClearCreatureEnemies(creatureID model.CreatureID) error {
	return w.w.ClearCreatureEnemies(creatureID)
}
func (w *loopUpdateActiveWorld) RemoveCreature(creatureID model.CreatureID) error {
	return w.w.RemoveCreature(creatureID)
}
func (w *loopUpdateActiveWorld) FinalizeMonsterDeath(creatureID model.CreatureID) (bool, error) {
	return w.w.FinalizeMonsterDeath(creatureID)
}
func (w *loopUpdateActiveWorld) UseCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) (int64, bool, error) {
	return w.w.UseCreatureCooldown(creatureID, key, nowUnix, intervalSeconds)
}
func (w *loopUpdateActiveWorld) SetCreatureCooldown(creatureID model.CreatureID, key string, nowUnix int64, intervalSeconds int64) error {
	return w.w.SetCreatureCooldown(creatureID, key, nowUnix, intervalSeconds)
}
func (w *loopUpdateActiveWorld) ActiveSessions() []ActiveSession {
	return w.l.ActiveSessions()
}
func (w *loopUpdateActiveWorld) WriteToSession(sessionID session.ID, text string, isPrompt bool) error {
	return w.l.WriteToSession(sessionID, text, isPrompt)
}
func (w *loopUpdateActiveWorld) BroadcastAll(text string) error {
	return w.l.BroadcastAll(text)
}
func (w *loopUpdateActiveWorld) BroadcastRoom(roomID model.RoomID, excludeSessionID session.ID, text string) error {
	return w.l.BroadcastRoom(roomID, excludeSessionID, text)
}
func (w *loopUpdateActiveWorld) SavePlayer(playerID model.PlayerID) error {
	return w.l.SavePlayer(playerID)
}
func (w *loopUpdateActiveWorld) MoveObjectToCreatureInventory(objectID model.ObjectInstanceID, creatureID model.CreatureID) error {
	return w.w.MoveObjectToCreatureInventory(objectID, creatureID)
}
func (w *loopUpdateActiveWorld) DestroyObject(objectID model.ObjectInstanceID) error {
	return w.w.DestroyObject(objectID)
}
func (w *loopUpdateActiveWorld) Object(objectID model.ObjectInstanceID) (model.ObjectInstance, bool) {
	return w.w.Object(objectID)
}
func (w *loopUpdateActiveWorld) ObjectPrototype(id model.PrototypeID) (model.ObjectPrototype, bool) {
	return w.w.ObjectPrototype(id)
}
func (w *loopUpdateActiveWorld) RecalculateAC(creatureID model.CreatureID) error {
	return w.l.RecalculateAC(creatureID)
}
func (w *loopUpdateActiveWorld) RecalculateTHACO(creatureID model.CreatureID) error {
	return w.l.RecalculateTHACO(creatureID)
}

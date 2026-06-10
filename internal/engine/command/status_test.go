package command

import (
	"strings"
	"testing"
	"time"

	"muhan/internal/commandspec"
	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestStatusHandlersRenderPlayerBasics(t *testing.T) {
	loaded := inventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:plaza",
		DisplayName: "광장",
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:remote",
		DisplayName: "외진 방",
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:secret",
		DisplayName: "숨은 방",
	})
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:admin",
		DisplayName: "관리방",
	})
	player := loaded.Players["player:alice"]
	player.RoomID = "room:plaza"
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:plaza"
	creature.Level = 7
	creature.Description = "건강해 보이고 "
	creature.Stats = map[string]int{
		"hpCurrent":           12,
		"hpMax":               34,
		"mpCurrent":           5,
		"mpMax":               8,
		"strength":            10,
		"dexterity":           11,
		"constitution":        12,
		"intelligence":        13,
		"piety":               14,
		"armor":               40,
		"thaco":               15,
		"experience":          12345,
		"gold":                678,
		"class":               model.ClassThief,
		"race":                legacyRaceHuman,
		"PMALES":              1,
		"legacyHoursInterval": 0,
	}
	loaded.Creatures[creature.ID] = creature
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:bob",
		DisplayName: "Bob",
		CreatureID:  "creature:bob",
		RoomID:      "room:remote",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:bob",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Bob",
		PlayerID:    "player:bob",
		RoomID:      "room:remote",
		Level:       24,
		Stats: map[string]int{
			"class":               model.ClassFighter,
			"PMALES":              1,
			"legacyHoursInterval": 3 * 86400,
		},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:hidden",
		DisplayName: "Hidden",
		CreatureID:  "creature:hidden",
		RoomID:      "room:secret",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:hidden",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Hidden",
		PlayerID:    "player:hidden",
		RoomID:      "room:secret",
		Level:       12,
		Stats: map[string]int{
			"class":  model.ClassFighter,
			"PINVIS": 1,
		},
	})
	mustAddLookPlayer(t, loaded, model.Player{
		ID:          "player:keeper",
		DisplayName: "Keeper",
		CreatureID:  "creature:keeper",
		RoomID:      "room:admin",
	})
	mustAddLookCreature(t, loaded, model.Creature{
		ID:          "creature:keeper",
		Kind:        model.CreatureKindPlayer,
		DisplayName: "Keeper",
		PlayerID:    "player:keeper",
		RoomID:      "room:admin",
		Level:       101,
		Stats: map[string]int{
			"class":  model.ClassCaretaker,
			"PMALES": 1,
		},
	})
	world := state.NewWorld(loaded)
	defer world.Close()
	registry := mustRegistry(t, []commandspec.CommandSpec{
		{Name: "장비", Number: 11, Handler: "equipment"},
		{Name: "점수", Number: 15, Handler: "health"},
		{Name: "건강", Number: 15, Handler: "health"},
		{Name: "정보", Number: 16, Handler: "info"},
		{Name: "어디", Number: 8, Handler: "where"},
		{Name: "상태", Number: 172, Handler: "effect_flag_list"},
		{Name: "시간", Number: 49, Handler: "prt_time"},
	})
	dispatcher := Dispatcher{
		Registry: registry,
		Handlers: map[string]Handler{
			"equipment":        NewEquipmentHandler(world),
			"health":           NewHealthHandler(world),
			"info":             NewInfoHandler(world),
			"where":            NewWhereHandler(world),
			"effect_flag_list": NewEffectStatusHandler(world),
			"prt_time":         NewTimeHandler(func() time.Time { return time.Date(2026, 5, 21, 15, 4, 5, 0, time.FixedZone("KST", 9*60*60)) }),
		},
	}

	cases := []struct {
		line string
		want []string
	}{
		{line: "장비", want: []string{"  <<<  착용 장비  >>>  \n", "right: 빛나는 검\n"}},
		{line: "점수", want: []string{"Alice :", "(레벨 7)", "[체  력] 12/34", "[도  력] 5/8", "[방어력] 60\n", "[목표치] 0", "[  돈  ] 678", "[용  기] 5\n", "당신은 건강해 보이고 있습니다."}},
		{line: "건강", want: []string{"[체  력] 12/34", "[도  력] 5/8"}},
		{line: "정보", want: []string{
			"\n[이  름] Alice        [배우자] 없음\n",
			"[칭  호]",
			"[레  벨] 7",
			"[종  족] 인간족",
			"[직  업] 도둑",
			"[성  향] 선  (평범합니다)",
			"접속시간 : 0분\n\n",
			"[  힘  ] 10",
			"[민  첩] 11",
			"[맷  집] 12",
			"[지  식] 13",
			"[신앙심] 14",
			"[체  력] 12",
			"[경험치] 12345",
			"[소지품 무게]",
			"총",
			"## 무기사용능력 ##",
			"[엔터]를 누르세요. 그만보시려면 [.]을 치세요: ",
		}},
		{line: "어디", want: []string{
			"사용자",
			"레벨",
			"장소",
			"----------------------------------------------------------------------\n",
			"Alice",
			"광장\n",
			"Bob",
			"외진 방\n",
			"Keeper",
			"총 3명의 사용자가 통계무한을 이용하고 있습니다.",
		}},
		{line: "상태", want: []string{"========================================================================\n", "현재 Alice님의 상태\n"}},
		{line: "시간", want: []string{"현재 시간: 오후 3시.\n", "실제 시간: Thu May 21 15:04:05 2026 (KST).\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			ctx := &Context{ActorID: "player:alice"}
			if tc.line == "어디" {
				type statusActiveSession struct {
					ID      string
					ActorID string
				}
				ctx.Values = map[string]any{
					"game.activeSessions": func() []statusActiveSession {
						return []statusActiveSession{
							{ID: "s1", ActorID: "player:alice"},
							{ID: "s2", ActorID: "player:bob"},
							{ID: "s3", ActorID: "player:hidden"},
							{ID: "s4", ActorID: "player:keeper"},
						}
					},
				}
			}
			status, err := dispatcher.DispatchLine(ctx, tc.line)
			if err != nil {
				t.Fatalf("DispatchLine() error = %v", err)
			}
			if status != StatusDefault {
				t.Fatalf("status = %d, want default", status)
			}
			out := ctx.OutputString()
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q:\n%s", want, out)
				}
			}
			if tc.line == "어디" {
				for _, unwanted := range []string{"Hidden", "숨은 방", "관리방"} {
					if strings.Contains(out, unwanted) {
						t.Fatalf("output unexpectedly contains %q:\n%s", unwanted, out)
					}
				}
			}
		})
	}
}

func TestInfoHandlerRegistersPendingAndRendersSpellPage(t *testing.T) {
	loaded := inventoryWorld(t)
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"SVIGOR", "PBLESS", "PRFIRE", "SASSASSIN"}
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.Level = 10
	creature.Stats = map[string]int{
		"class":               model.ClassInvincible,
		"race":                legacyRaceHuman,
		"hpCurrent":           30,
		"hpMax":               40,
		"mpCurrent":           20,
		"mpMax":               25,
		"realmEarth":          1024,
		"realmWind":           2048,
		"realmFire":           4096,
		"realmWater":          8192,
		"legacyHoursInterval": 90 * 60,
	}
	creature.Properties = map[string]string{
		"quest_completed_1": "1",
		"quest_completed_2": "1",
	}
	loaded.Creatures[creature.ID] = creature

	world := state.NewWorld(loaded)
	defer world.Close()
	var pending PendingLineHandler
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}
	status, err := NewInfoHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want do-prompt", status)
	}
	if pending == nil {
		t.Fatal("pending handler was not registered")
	}
	if out := ctx.OutputString(); !strings.Contains(out, "[엔터]를 누르세요. 그만보시려면 [.]을 치세요: ") {
		t.Fatalf("first page missing prompt:\n%s", out)
	}

	ctx.Output = nil
	status, err = pending(ctx, "")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("pending status = %d, want prompt", status)
	}
	out := ctx.OutputString()
	for _, want := range []string{
		"## 주 술  계 열 ##",
		"[ 땅 ]",
		"주문: 회복.",
		"당신의 현주문: 성현진, 방열진.",
		"당신은 현재 임무 2까지 달성하였습니다.",
		"무적수련 : 자객",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("second page missing %q:\n%s", want, out)
		}
	}
}

func TestInfoHandlerPendingCancel(t *testing.T) {
	loaded := inventoryWorld(t)
	world := state.NewWorld(loaded)
	defer world.Close()
	var pending PendingLineHandler
	ctx := &Context{
		ActorID: "player:alice",
		Values: map[string]any{
			ContextPendingLineKey: func(handler PendingLineHandler) {
				pending = handler
			},
		},
	}
	status, err := NewInfoHandler(world)(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDoPrompt {
		t.Fatalf("status = %d, want do-prompt", status)
	}
	if pending == nil {
		t.Fatal("pending handler was not registered")
	}

	ctx.Output = nil
	status, err = pending(ctx, ".")
	if err != nil {
		t.Fatalf("pending() error = %v", err)
	}
	if status != StatusPrompt {
		t.Fatalf("pending status = %d, want prompt", status)
	}
	if pending != nil {
		t.Fatal("pending handler was not cleared")
	}
	if got, want := ctx.OutputString(), "중단되었습니다.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestStatusWeaponProficiencyUsesCTables(t *testing.T) {
	cases := []struct {
		name  string
		class int
		raw   int
		want  int
	}{
		{name: "fighter threshold boundary", class: model.ClassFighter, raw: 1024, want: 20},
		{name: "sub-dm privileged table", class: model.ClassSubDM, raw: 1024, want: 20},
		{name: "thief slower table", class: model.ClassThief, raw: 1024, want: 4},
		{name: "mage threshold boundary", class: model.ClassMage, raw: 5376, want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			creature := model.Creature{Stats: map[string]int{
				"class":            tc.class,
				"proficiencySharp": tc.raw,
			}}
			if got := statusWeaponProficiency(creature, 0); got != tc.want {
				t.Fatalf("statusWeaponProficiency() = %d, want %d", got, tc.want)
			}
		})
	}

	creature := model.Creature{
		Stats:      map[string]int{"class": model.ClassFighter},
		Properties: map[string]string{"proficiency/sharp": "1024"},
	}
	if got := statusWeaponProficiency(creature, 0); got != 20 {
		t.Fatalf("statusWeaponProficiency(legacy property) = %d, want 20", got)
	}
}

func TestWhereCreatureIntNormalizesStatAndPropertyKeys(t *testing.T) {
	creature := model.Creature{
		Stats:      map[string]int{"LT-HOURS interval": 2 * 86400},
		Properties: map[string]string{"legacy-age-years": "23"},
	}

	if got, ok := whereCreatureInt(creature, "LT_HOURS_interval"); !ok || got != 2*86400 {
		t.Fatalf("normalized stat = %d/%v, want %d/true", got, ok, 2*86400)
	}
	if got, ok := whereCreatureInt(creature, "legacyAgeYears"); !ok || got != 23 {
		t.Fatalf("normalized property = %d/%v, want 23/true", got, ok)
	}
	if got := whereAgeYears(model.Creature{Properties: map[string]string{"legacy-age-years": "23"}}); got != 23 {
		t.Fatalf("whereAgeYears(normalized property) = %d, want 23", got)
	}
}

func TestCreatureStatNormalizesStatAndPropertyKeys(t *testing.T) {
	creature := model.Creature{
		Stats:      map[string]int{"HP-CURRENT": 12},
		Properties: map[string]string{"mp-current": "5"},
	}

	if got := creatureStat(creature, "hpCurrent"); got != 12 {
		t.Fatalf("normalized stat = %d, want 12", got)
	}
	if got := creatureStat(creature, "mpCurrent"); got != 5 {
		t.Fatalf("normalized property = %d, want 5", got)
	}
	if got := creatureClass(model.Creature{Stats: map[string]int{"CLASS": model.ClassDM}}); got != model.ClassDM {
		t.Fatalf("creatureClass(normalized stat) = %d, want %d", got, model.ClassDM)
	}
	if got := accountCreatureLevel(model.Creature{Properties: map[string]string{"LE-VEL": "6"}}); got != 6 {
		t.Fatalf("accountCreatureLevel(normalized property) = %d, want 6", got)
	}
	if got := getCreatureLevel(model.Creature{Properties: map[string]string{"le vel": "7"}}); got != 7 {
		t.Fatalf("getCreatureLevel(normalized property) = %d, want 7", got)
	}
}

func TestWhereHandlerReportsBlindActor(t *testing.T) {
	loaded := inventoryWorld(t)
	mustAddLookRoom(t, loaded, model.Room{
		ID:          "room:plaza",
		DisplayName: "광장",
	})
	player := loaded.Players["player:alice"]
	player.RoomID = "room:plaza"
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.RoomID = "room:plaza"
	creature.Stats = map[string]int{
		"PBLIND": 1,
	}
	loaded.Creatures[creature.ID] = creature

	handler := NewWhereHandler(state.NewWorld(loaded))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got, want := ctx.OutputString(), "당신은 눈이 멀어 있습니다!\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestHealthHandlerReportsBlindActor(t *testing.T) {
	loaded := inventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Stats = map[string]int{
		"PBLIND": 1,
	}
	loaded.Creatures[creature.ID] = creature

	handler := NewHealthHandler(state.NewWorld(loaded))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	if got, want := ctx.OutputString(), "당신은 눈이 멀어 있습니다!"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTimeHandlerRendersKSTClock(t *testing.T) {
	handler := NewTimeHandler(func() time.Time {
		return time.Date(2026, 5, 21, 15, 4, 5, 0, time.FixedZone("KST", 9*60*60))
	})
	ctx := &Context{}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	want := "현재 시간: 오후 3시.\n실제 시간: Thu May 21 15:04:05 2026 (KST).\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTimeHandlerUsesLegacyWorldClockForCurrentTime(t *testing.T) {
	world := state.NewWorld(inventoryWorld(t))
	defer world.Close()
	world.SetLegacyTime(23)
	handler := NewTimeHandlerWithWorld(world, func() time.Time {
		return time.Date(2026, 5, 21, 15, 4, 5, 0, time.FixedZone("KST", 9*60*60))
	})
	ctx := &Context{}

	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}
	want := "현재 시간: 오후 11시.\n실제 시간: Thu May 21 15:04:05 2026 (KST).\n"
	if got := ctx.OutputString(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestEffectStatusHandlerRendersMetadataAndStatFlags(t *testing.T) {
	loaded := inventoryWorld(t)
	player := loaded.Players["player:alice"]
	player.Metadata.Tags = []string{"detectMagic", "blessed", "poison"}
	loaded.Players[player.ID] = player
	creature := loaded.Creatures["creature:alice"]
	creature.Metadata.Tags = []string{"invisible", "blind", "silenced", "resistFire", "poisoned"}
	creature.Stats = map[string]int{
		"PDINVI": 1,
		"PFEARS": 1,
		"PDISEA": 0,
	}
	loaded.Creatures[creature.ID] = creature

	handler := NewEffectStatusHandler(state.NewWorld(loaded))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	out := ctx.OutputString()
	for _, want := range []string{
		"========================================================================\n",
		"현재 Alice님의 상태\n",
		"중독",
		"실명",
		"공포",
		"은둔",
		"은둔감지",
		"성현진",
		"방열진",
		"주문감지",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"상태 효과", "없음", "침묵"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("output unexpectedly contains %q:\n%s", unwanted, out)
		}
	}
}

func TestStatusEffectActiveReadsPropertyBackedLegacyFlags(t *testing.T) {
	creature := model.Creature{
		Properties: map[string]string{
			"PBLIND": "true",
			"flags":  "PFEARS|PRMAGI",
			"PDISEA": "false",
		},
	}

	for _, tc := range []struct {
		name    string
		aliases []string
		want    bool
	}{
		{name: "direct true property", aliases: []string{"blind", "PBLIND"}, want: true},
		{name: "token property fear", aliases: []string{"fear", "PFEARS"}, want: true},
		{name: "token property resist magic", aliases: []string{"resistMagic", "PRMAGI"}, want: true},
		{name: "direct false property", aliases: []string{"disease", "PDISEA"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusEffectActive(model.Player{}, creature, tc.aliases...); got != tc.want {
				t.Fatalf("statusEffectActive(%v) = %v, want %v", tc.aliases, got, tc.want)
			}
		})
	}
}

func TestEffectStatusHandlerRendersPropertyBackedLegacyFlags(t *testing.T) {
	loaded := inventoryWorld(t)
	creature := loaded.Creatures["creature:alice"]
	creature.Properties = map[string]string{
		"PBLIND": "true",
		"flags":  "PFEARS|PRMAGI",
	}
	loaded.Creatures[creature.ID] = creature

	handler := NewEffectStatusHandler(state.NewWorld(loaded))
	ctx := &Context{ActorID: "player:alice"}
	status, err := handler(ctx, ResolvedCommand{})
	if err != nil {
		t.Fatalf("handler() error = %v", err)
	}
	if status != StatusDefault {
		t.Fatalf("status = %d, want default", status)
	}

	out := ctx.OutputString()
	for _, want := range []string{"실명", "공포", "보마진"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestEffectStatusIgnoresLearnedSpellFlags(t *testing.T) {
	player := model.Player{
		Metadata: model.Metadata{Tags: []string{
			"SBLESS", "SLIGHT", "SPROTE", "SINVIS", "SDINVI", "SDMAGI", "SLEVIT",
			"SRFIRE", "SFLYSP", "SRMAGI", "SKNOWA", "SRCOLD", "SBRWAT", "SSSHLD",
			"STRANO", "SBLIND", "SFEARS", "SSILNC", "SBEFUD",
		}},
	}
	creature := model.Creature{
		Stats: map[string]int{
			"SBLESS": 1,
			"SLIGHT": 1,
			"SPROTE": 1,
			"SINVIS": 1,
			"SDINVI": 1,
			"SDMAGI": 1,
			"SLEVIT": 1,
			"SRFIRE": 1,
			"SFLYSP": 1,
			"SRMAGI": 1,
			"SKNOWA": 1,
			"SRCOLD": 1,
			"SBRWAT": 1,
			"SSSHLD": 1,
			"STRANO": 1,
			"SBLIND": 1,
			"SFEARS": 1,
			"SSILNC": 1,
			"SBEFUD": 1,
			"PRFIRE": 1,
		},
	}

	labels := activeStatusEffectLabels(player, creature)
	if got, want := strings.Join(labels, ","), "방열진"; got != want {
		t.Fatalf("activeStatusEffectLabels() = %q, want %q", got, want)
	}
}

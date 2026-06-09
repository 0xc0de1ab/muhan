package game

import (
	"slices"
	"testing"

	"muhan/internal/world/model"
	"muhan/internal/world/state"
)

func TestGroupMemorySnapshotForLeaderAndFollower(t *testing.T) {
	groups := NewGroupMemory()
	groups.Follow("player:bob", "player:alice")
	groups.Follow("player:charlie", "player:alice")

	for _, actorID := range []string{"player:alice", "player:bob"} {
		snapshot, ok := groups.Snapshot(actorID)
		if !ok {
			t.Fatalf("Snapshot(%q) ok = false, want true", actorID)
		}
		if snapshot.LeaderID != "player:alice" {
			t.Fatalf("Snapshot(%q) leader = %q, want player:alice", actorID, snapshot.LeaderID)
		}
		if want := []string{"player:charlie", "player:bob"}; !slices.Equal(snapshot.FollowerIDs, want) {
			t.Fatalf("Snapshot(%q) followers = %+v, want %+v", actorID, snapshot.FollowerIDs, want)
		}
	}

	if _, ok := groups.Snapshot("player:dave"); ok {
		t.Fatal("Snapshot(ungrouped) ok = true, want false")
	}
}

func TestGroupMemoryKeepsLegacyFollowerListOrder(t *testing.T) {
	groups := NewGroupMemory()
	groups.Follow("player:bob", "player:alice")
	groups.Follow("player:charlie", "player:alice")
	groups.Follow("player:dave", "player:alice")

	if got, want := groups.FollowersOf("player:alice"), []string{"player:dave", "player:charlie", "player:bob"}; !slices.Equal(got, want) {
		t.Fatalf("FollowersOf(alice) = %+v, want %+v", got, want)
	}

	groups.Follow("player:charlie", "player:bob")
	if got, want := groups.FollowersOf("player:alice"), []string{"player:dave", "player:bob"}; !slices.Equal(got, want) {
		t.Fatalf("FollowersOf(alice) after refollow = %+v, want %+v", got, want)
	}
	if got, want := groups.FollowersOf("player:bob"), []string{"player:charlie"}; !slices.Equal(got, want) {
		t.Fatalf("FollowersOf(bob) = %+v, want %+v", got, want)
	}

	if leader, ok := groups.Unfollow("player:charlie"); !ok || leader != "player:bob" {
		t.Fatalf("Unfollow(charlie) = %q, %v, want player:bob, true", leader, ok)
	}
	if got := groups.FollowersOf("player:bob"); len(got) != 0 {
		t.Fatalf("FollowersOf(bob) after unfollow = %+v, want empty", got)
	}
}

func TestGroupMembershipSnapshotCreatureSnapshot(t *testing.T) {
	world := state.NewWorld(socialWorld(t))
	snapshot := GroupMembershipSnapshot{
		LeaderID:    "player:alice",
		FollowerIDs: []string{"player:bob", "player:missing", "player:alice", "player:charlie"},
	}

	leaderID, followerIDs, ok := snapshot.CreatureSnapshot(world)
	if !ok {
		t.Fatal("CreatureSnapshot() ok = false, want true")
	}
	if leaderID != "creature:alice" {
		t.Fatalf("CreatureSnapshot() leader = %q, want creature:alice", leaderID)
	}
	if want := []model.CreatureID{"creature:bob", "creature:charlie"}; !slices.Equal(followerIDs, want) {
		t.Fatalf("CreatureSnapshot() followers = %+v, want %+v", followerIDs, want)
	}
}

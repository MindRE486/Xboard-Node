package kernel

import (
	"reflect"
	"testing"

	"github.com/cedar2025/xboard-node/internal/model"
)

func TestUserDiffReplacesRotatedUUID(t *testing.T) {
	oldUser := model.UserSpec{ID: 5, UUID: "old-uuid"}
	newUser := model.UserSpec{ID: 5, UUID: "new-uuid"}

	toAdd, toRemove := UserDiff([]model.UserSpec{oldUser}, []model.UserSpec{newUser})

	if len(toAdd) != 1 || toAdd[0] != newUser {
		t.Fatalf("toAdd = %#v, want %#v", toAdd, []model.UserSpec{newUser})
	}
	if len(toRemove) != 1 || toRemove[0] != oldUser {
		t.Fatalf("toRemove = %#v, want %#v", toRemove, []model.UserSpec{oldUser})
	}
}

func TestUserDiffKeepsUnchangedUsers(t *testing.T) {
	user := model.UserSpec{ID: 5, UUID: "same-uuid"}

	toAdd, toRemove := UserDiff([]model.UserSpec{user}, []model.UserSpec{user})

	if len(toAdd) != 0 || len(toRemove) != 0 {
		t.Fatalf("unchanged user diff = add %#v, remove %#v; want no changes", toAdd, toRemove)
	}
}

func TestUserDiffReplacesChangedUUID(t *testing.T) {
	old := model.UserSpec{ID: 1, UUID: "old-uuid"}
	replacement := model.UserSpec{ID: 1, UUID: "new-uuid"}
	unchanged := model.UserSpec{ID: 2, UUID: "unchanged"}
	added, removed := UserDiff([]model.UserSpec{old, unchanged}, []model.UserSpec{replacement, unchanged})
	if !reflect.DeepEqual(added, []model.UserSpec{replacement}) || !reflect.DeepEqual(removed, []model.UserSpec{old}) {
		t.Fatalf("UUID replacement diff = add %#v, remove %#v; want new identity added and old identity removed", added, removed)
	}
}

func TestUserDiffPreservesIdentityForLimitChanges(t *testing.T) {
	old := model.UserSpec{ID: 1, UUID: "same-uuid", SpeedLimit: 1, DeviceLimit: 1}
	updated := old
	updated.SpeedLimit, updated.DeviceLimit = 10, 3
	added, removed := UserDiff([]model.UserSpec{old}, []model.UserSpec{updated})
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("limit change replaced identity: +%d -%d", len(added), len(removed))
	}
}

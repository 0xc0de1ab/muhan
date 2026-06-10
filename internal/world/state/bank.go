package state

import (
	"fmt"
	"muhan/internal/world/model"
	"strconv"
	"strings"
	"time"
)

// MarkBankDirty marks a bank as needing persistence.
func (w *World) MarkBankDirty(bid model.BankID) {
	if bid == "" {
		return
	}
	w.dirtyMu.Lock()
	if w.bankDirty == nil {
		w.bankDirty = make(map[model.BankID]int64)
	}
	w.bankDirty[bid] = time.Now().Unix()
	w.dirtyMu.Unlock()
}

// Bank returns a copy of the bank account with id.
func (w *World) Bank(id model.BankID) (model.BankAccount, bool) {
	if w == nil {
		return model.BankAccount{}, false
	}
	w.rLockDomains(true, true, true, true, true, true, true)
	defer w.rUnlockDomains(true, true, true, true, true, true, true)

	account, ok := w.banks[id]
	if !ok {
		return model.BankAccount{}, false
	}
	return cloneBankAccount(account), true
}

// GetBank returns a copy of the bank account with id.
func (w *World) GetBank(id model.BankID) (model.BankAccount, bool) {
	return w.Bank(id)
}

// EnsureFamilyBankRoot creates the family bank root for the given special bucket
// when it does not exist, mirroring legacy load_family_bank() materialization.
func (w *World) EnsureFamilyBankRoot(familyID int, special int) (model.BankAccount, model.ObjectInstance, error) {
	if w == nil {
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure family bank root: world state is nil")
	}
	if familyID <= 0 {
		return model.BankAccount{}, model.ObjectInstance{}, fmt.Errorf("ensure family bank root: family id is required")
	}

	w.lockDomains(true, true, true, true, true, true, true)
	ownerName := w.stateFamilyBankOwnerNameLocked(familyID, special)
	bankID := model.BankID("bank:family:" + ownerName)
	if account, ok := w.banks[bankID]; ok {
		for _, objectID := range account.Objects.ObjectIDs {
			if objectID.IsZero() {
				continue
			}
			if root, found := w.objects[objectID]; found {
				w.unlockDomains(true, true, true, true, true, true, true)
				return cloneBankAccount(account), cloneObject(root), nil
			}
		}
	}

	protoID := model.PrototypeID("proto:family-bank-root")
	displayName := "패거리 창고"
	if special == 0 {
		displayName = "패거리 금고"
	}
	if _, ok := w.prototypes[protoID]; !ok {
		w.prototypes[protoID] = model.ObjectPrototype{
			ID:          protoID,
			Kind:        model.ObjectKindContainer,
			DisplayName: displayName,
			Properties: map[string]string{
				"kind":   string(model.ObjectKindContainer),
				"OCONTN": "1",
			},
		}
	}
	rootID := w.nextFamilyBankRootIDLocked(ownerName)
	root := model.ObjectInstance{
		ID:          rootID,
		PrototypeID: protoID,
		Location:    model.ObjectLocation{BankID: bankID, Slot: "bank"},
		Properties: map[string]string{
			"value":        "0",
			"shotsCurrent": "0",
			"shotsMax":     "200",
			"kind":         string(model.ObjectKindContainer),
			"OCONTN":       "1",
		},
	}
	account := model.BankAccount{
		ID:        bankID,
		Kind:      "family",
		OwnerName: ownerName,
		Objects:   model.ObjectRefList{ObjectIDs: []model.ObjectInstanceID{rootID}},
	}
	w.objects[rootID] = root
	w.banks[bankID] = account
	w.unlockDomains(true, true, true, true, true, true, true)

	w.MarkBankDirty(bankID)
	return cloneBankAccount(account), cloneObject(root), nil
}

func (w *World) stateFamilyBankOwnerNameLocked(familyID int, special int) string {
	base := ""
	for _, family := range w.families {
		if family.ID == familyID || family.Slot == familyID {
			base = strings.TrimSpace(family.DisplayName)
			break
		}
	}
	if base == "" {
		base = strconv.Itoa(familyID)
	}
	return fmt.Sprintf("%s_%d", base, special)
}

func (w *World) nextFamilyBankRootIDLocked(ownerName string) model.ObjectInstanceID {
	base := model.ObjectInstanceID("object:family-bank-root:" + ownerName)
	if _, exists := w.objects[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := model.ObjectInstanceID(fmt.Sprintf("%s:%d", base, i))
		if _, exists := w.objects[candidate]; !exists {
			return candidate
		}
	}
}

func cloneBankAccount(account model.BankAccount) model.BankAccount {
	account.Objects = cloneObjectRefList(account.Objects)
	account.Metadata = cloneMetadata(account.Metadata)
	return account
}

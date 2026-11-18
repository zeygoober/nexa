package store

import (
	"context"
	"encoding/json"
	"errors"

	"evilion.xyz/chat/internal/db"
	"evilion.xyz/chat/internal/ids"
)

// MaxOwnerCopies is the most copies of a collectable the owner gives or takes
// at once.
const MaxOwnerCopies = 100

// ErrNotCollectible is a key that names no collectible at a material.
var ErrNotCollectible = errors.New("not a collectible at a material")

// CollectibleCopies is how many copies of each collectible at each material
// someone holds, by key (event:<id>-<material>), for the owner's dialog.
// Copies of a badge since made no longer a collectible are left out.
func (s *Store) CollectibleCopies(ctx context.Context, uid ids.ID) (map[string]int, error) {
	rows, err := s.holdingRows(ctx, int64(uid))
	if err != nil {
		return nil, err
	}
	var want []ids.ID
	for k, r := range rows {
		if v, m, ok := eventKey(k); ok && m >= 1 && r.copies > 0 {
			want = append(want, v)
		}
	}
	out := map[string]int{}
	if len(want) == 0 {
		return out, nil
	}
	defs, err := s.badgeDefsByID(ctx, want)
	if err != nil {
		return nil, err
	}
	for k, r := range rows {
		v, m, ok := eventKey(k)
		if !ok || m < 1 || r.copies <= 0 {
			continue
		}
		if d := defs[v]; d != nil && d.Rarity != "" {
			out[k] += r.copies
		}
	}
	return out, nil
}

// OwnerCollectible gives someone copies of a collectible at one material (n
// above 0) or takes them (below 0) for the owner, with the log line in the
// same transaction. Taking more than they hold is ErrGemsShort; a copy offered
// in an open trade can be taken, and that trade then fails as one would for a
// copy traded away elsewhere. It answers how many they hold after.
func (s *Store) OwnerCollectible(ctx context.Context, uid ids.ID, key string, n int, log *ModLogEntry) (int, error) {
	v, m, ok := eventKey(key)
	if !ok || m < 1 || m > GemMaterials || n == 0 || n > MaxOwnerCopies || n < -MaxOwnerCopies {
		return 0, ErrNotCollectible
	}
	defs, err := s.badgeDefsByID(ctx, []ids.ID{v})
	if err != nil {
		return 0, err
	}
	d := defs[v]
	if d == nil || d.Rarity == "" {
		return 0, ErrNotCollectible
	}
	key = TradeKey(EventBadgePrefix+v.String(), m)
	var after int
	err = s.Tx(ctx, func(tx *db.Tx) error {
		if n > 0 {
			if err := give(ctx, tx, uid, key, n); err != nil {
				return err
			}
		} else {
			// collectibles are never earned, so only copies count
			ok, err := take(ctx, tx, uid, false, key, -n)
			if err != nil {
				return err
			}
			if !ok {
				return ErrGemsShort
			}
		}
		if err := tx.QueryRowContext(ctx, `SELECT copies FROM badge_holdings WHERE user_id = ? AND badge = ?`, int64(uid), key).Scan(&after); err != nil {
			return err
		}
		log.Details, _ = json.Marshal(map[string]any{"name": d.Name, "badge": key, "material": m, "amount": n, "copies": after})
		return appendModLogTx(ctx, tx, log)
	})
	if err != nil {
		return 0, err
	}
	s.exist.forget()
	return after, nil
}

// CollectibleSet is a collectible with how it looks at each material, bronze
// first, for the owner's dialog.
type CollectibleSet struct {
	ID        ids.ID  `json:"id"`
	Name      string  `json:"name"`
	Rarity    string  `json:"rarity"`
	Materials []Badge `json:"materials"`
}

// CollectibleSets is every collectible at every material, oldest first.
func (s *Store) CollectibleSets(ctx context.Context) ([]CollectibleSet, error) {
	defs, err := s.BadgeDefs(ctx)
	if err != nil {
		return nil, err
	}
	out := []CollectibleSet{}
	for i := len(defs) - 1; i >= 0; i-- {
		d := defs[i]
		if d.Rarity == "" {
			continue
		}
		set := CollectibleSet{ID: d.ID, Name: d.Name, Rarity: d.Rarity}
		for m := 1; m <= GemMaterials; m++ {
			set.Materials = append(set.Materials, collectibleBadge(d, m))
		}
		out = append(out, set)
	}
	return out, nil
}

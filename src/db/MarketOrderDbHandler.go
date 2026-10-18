package db

import (
	"MeadowWorksTools/src/esi"
	"context"

	"github.com/jackc/pgx/v5"
)

// SaveMarketOrders stores the owner's open orders and their order history.
// History rows carry each closed order's final state, so they overwrite the
// order's last "open" row.
func SaveMarketOrders(ctx context.Context, ownerID int64, ownerType string, open, history []esi.MarketOrder) error {
	batch := &pgx.Batch{}
	queue := func(order esi.MarketOrder, state string) {
		batch.Queue(`INSERT INTO lifetime_indy.market_orders
			(owner_id, owner_type, order_id, type_id, is_buy_order, price, volume_total, volume_remain, min_volume,
			escrow, range, duration, issued, location_id, region_id, is_corporation, issued_by, wallet_division, state)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
			ON CONFLICT (owner_id, order_id) DO UPDATE SET
			price = EXCLUDED.price, volume_remain = EXCLUDED.volume_remain, escrow = EXCLUDED.escrow,
			issued = EXCLUDED.issued, duration = EXCLUDED.duration, state = EXCLUDED.state, updated_at = now()`,
			ownerID, ownerType, order.OrderId, order.TypeId, order.IsBuyOrder, order.Price, order.VolumeTotal,
			order.VolumeRemain, nullIfZero(order.MinVolume), nullIfZero(order.Escrow), order.Range, order.Duration,
			order.Issued, order.LocationId, order.RegionId, isCorporation(order, ownerType),
			nullIfZero(order.IssuedBy), nullIfZero(order.WalletDivision), state)
	}
	for _, order := range open {
		queue(order, "open")
	}
	for _, order := range history {
		queue(order, order.State)
	}
	return sendBatch(ctx, batch)
}

// isCorporation is only meaningful on character orders.
func isCorporation(order esi.MarketOrder, ownerType string) any {
	if ownerType != OwnerCharacter {
		return nil
	}
	return order.IsCorporation
}

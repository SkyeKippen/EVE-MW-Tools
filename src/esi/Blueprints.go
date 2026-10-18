package esi

import "context"

// Blueprint is one blueprint, or a stack of unused originals, from the
// character or corporation blueprints routes.
type Blueprint struct {
	ItemId             int64  `json:"item_id"`
	LocationFlag       string `json:"location_flag"`
	LocationId         int64  `json:"location_id"`
	TypeId             int64  `json:"type_id"`
	Quantity           int64  `json:"quantity"` // -1 an original, -2 a copy, above 0 a stack of originals
	Runs               int64  `json:"runs"`     // runs left on a copy; -1 for an original
	MaterialEfficiency int    `json:"material_efficiency"`
	TimeEfficiency     int    `json:"time_efficiency"`
}

// IsCopy is true for a blueprint copy.
func (b Blueprint) IsCopy() bool {
	return b.Quantity == -2
}

// GetCharacterBlueprints needs the esi-characters.read_blueprints.v1 scope.
func (c *Client) GetCharacterBlueprints(ctx context.Context, characterId int64, accessToken string) ([]Blueprint, error) {
	return getPages[Blueprint](ctx, c, c.url("/characters/%d/blueprints", characterId), accessToken)
}

// GetCorporationBlueprints needs the esi-corporations.read_blueprints.v1
// scope and the Director role.
func (c *Client) GetCorporationBlueprints(ctx context.Context, corporationId int64, accessToken string) ([]Blueprint, error) {
	return getPages[Blueprint](ctx, c, c.url("/corporations/%d/blueprints", corporationId), accessToken)
}

package esi

import "context"

type Station struct {
	Name     string `json:"name"`
	Owner    int64  `json:"owner"` // NPC corporation
	SystemId int64  `json:"system_id"`
	TypeId   int64  `json:"type_id"`
}

type SolarSystem struct {
	Name            string `json:"name"`
	ConstellationId int64  `json:"constellation_id"`
}

type Constellation struct {
	Name     string `json:"name"`
	RegionId int64  `json:"region_id"`
}

// GetStation returns an NPC station. No scope needed.
func (c *Client) GetStation(ctx context.Context, stationId int64) (Station, error) {
	return get[Station](ctx, c, c.url("/universe/stations/%d", stationId), "")
}

// GetSolarSystem returns a solar system. No scope needed.
func (c *Client) GetSolarSystem(ctx context.Context, systemId int64) (SolarSystem, error) {
	return get[SolarSystem](ctx, c, c.url("/universe/systems/%d", systemId), "")
}

// GetConstellation returns a constellation, whose region a solar system is
// in. No scope needed.
func (c *Client) GetConstellation(ctx context.Context, constellationId int64) (Constellation, error) {
	return get[Constellation](ctx, c, c.url("/universe/constellations/%d", constellationId), "")
}

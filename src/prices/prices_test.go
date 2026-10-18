package prices

import (
	"MeadowWorksTools/src/esi"
	"testing"
)

func TestSellPricesAveragesCheapestFivePercent(t *testing.T) {
	orders := []esi.RegionOrder{
		{OrderId: 1, TypeId: 34, Price: 6, VolumeRemain: 900},
		{OrderId: 2, TypeId: 34, Price: 4, VolumeRemain: 30},
		{OrderId: 3, TypeId: 34, Price: 5, VolumeRemain: 70},
		{OrderId: 4, TypeId: 34, Price: 1, VolumeRemain: 500, IsBuyOrder: true}, // ignored
		{OrderId: 5, TypeId: 35, Price: 100, VolumeRemain: 1},
	}
	prices := SellPrices(orders)
	if len(prices) != 2 {
		t.Fatalf("got %d prices, want 2", len(prices))
	}
	// 1000 units for sale, so the cheapest 50: 30 at 4 and 20 at 5
	if got, want := prices[0].SellPrice, (30*4.0+20*5.0)/50; got != want {
		t.Errorf("type 34 price = %v, want %v", got, want)
	}
	if prices[0].MinPrice != 4 || prices[0].Volume != 1000 || prices[0].OrderCount != 3 {
		t.Errorf("type 34 = %+v", prices[0])
	}
	if prices[1].SellPrice != 100 {
		t.Errorf("type 35 price = %v, want 100", prices[1].SellPrice)
	}
}

func TestCapAtAverage(t *testing.T) {
	prices := CapAtAverage([]Price{
		{TypeId: 21594, SellPrice: 64_000_000}, // junk listings only
		{TypeId: 34, SellPrice: 3.89},
		{TypeId: 99, SellPrice: 50}, // no average
	}, map[int64]float64{21594: 9.36, 34: 3.73})
	if prices[0].SellPrice != 28.08 || prices[1].SellPrice != 3.89 || prices[2].SellPrice != 50 {
		t.Errorf("prices = %+v", prices)
	}
}

func TestCapBlueprints(t *testing.T) {
	prices := CapBlueprints([]Price{
		{TypeId: 34, SellPrice: 4.5}, {TypeId: 688, SellPrice: 2_000_000_000}, {TypeId: 691, SellPrice: 1_000_000},
	}, func(typeId int64) (bool, float64) {
		switch typeId {
		case 688:
			return true, 1_135_000_000
		case 691:
			return true, 2_800_000
		}
		return false, 0
	})
	if prices[0].SellPrice != 4.5 || prices[1].SellPrice != 1_135_000_000 || prices[2].SellPrice != 1_000_000 {
		t.Errorf("capped = %+v", prices)
	}
}

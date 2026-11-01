package prices

import (
	"MeadowWorksTools/src/esi"
	"testing"
)

func TestSummarize(t *testing.T) {
	orders := []esi.RegionOrder{
		// 100 units for sale: the cheapest 5 are 3 at 10 and 2 at 12
		{TypeId: 1, Price: 10, VolumeRemain: 3},
		{TypeId: 1, Price: 12, VolumeRemain: 7},
		{TypeId: 1, Price: 20, VolumeRemain: 90},
		// 40 units wanted: the best 2 are at 9
		{TypeId: 1, IsBuyOrder: true, Price: 9, VolumeRemain: 5},
		{TypeId: 1, IsBuyOrder: true, Price: 8, VolumeRemain: 35},
		// only wanted
		{TypeId: 2, IsBuyOrder: true, Price: 100, VolumeRemain: 1},
		{TypeId: 2, IsBuyOrder: true, Price: 50, VolumeRemain: 0},
	}
	list := Summarize(orders, nil)
	if len(list) != 2 {
		t.Fatalf("%d types, want 2", len(list))
	}
	a, b := list[0], list[1]
	if !a.HasSell || a.Sell != 10.8 || a.MinSell != 10 || a.SellVolume != 100 || a.SellOrders != 3 {
		t.Errorf("type 1 sell: %+v", a)
	}
	if !a.HasBuy || a.Buy != 9 || a.MaxBuy != 9 || a.BuyVolume != 40 || a.BuyOrders != 2 {
		t.Errorf("type 1 buy: %+v", a)
	}
	if b.HasSell || !b.HasBuy || b.Buy != 100 || b.BuyOrders != 1 {
		t.Errorf("type 2: %+v", b)
	}
}

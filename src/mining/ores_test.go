package mining

import (
	"strings"
	"testing"
)

func TestBaseOreName(t *testing.T) {
	for name, want := range map[string]string{
		"Veldspar":                       "Veldspar",
		"Veldspar III-Grade":             "Veldspar",
		"Dark Ochre IV-Grade":            "Dark Ochre",
		"Compressed Veldspar II-Grade":   "Veldspar",
		"Batch Compressed Scordite":      "Scordite",
		"Bitumens II-Grade":              "Bitumens",
		"Blue Ice":                       "Blue Ice",
		"Brimful Zeolites":               "Zeolites",
		"Glistening Bitumens":            "Bitumens",
		"Compressed Twinkling Cobaltite": "Cobaltite",
		"Shining Xenotime":               "Xenotime",
		"Glowing Zircon":                 "Zircon",
		"Glistening Xenotime":            "Glistening Xenotime", // R4 prefix on an R64 ore isn't a variant
	} {
		if got := BaseOreName(name); got != want {
			t.Errorf("BaseOreName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTotalsByBaseOre(t *testing.T) {
	got := TotalsByBaseOre(map[string]int64{
		"Zeolites": 100, "Brimful Zeolites": 50, "Glistening Zeolites": 10,
		"Veldspar": 500, "Veldspar II-Grade": 20,
	})
	if len(got) != 2 || got[0].BaseOre != "Veldspar" || got[0].Quantity != 520 ||
		got[1].BaseOre != "Zeolites" || got[1].Quantity != 160 ||
		strings.Join(got[1].Variants, ",") != "Brimful Zeolites,Glistening Zeolites,Zeolites" {
		t.Errorf("got %+v", got)
	}
}

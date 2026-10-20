// Package sde reads what each item refines into, each item's volume, group
// and category, and what each blueprint makes from EVE's static data export
// (SDE). ESI has no refine materials or blueprint outputs, so the app keeps
// the SDE files it needs in data/sde, downloading them from CCP the first
// time any is missing.
package sde

import (
	"MeadowWorksTools/src/esi"
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Where the SDE files are kept, relative to the working directory like
// config/.env.
const (
	TypeMaterialsPath = "data/sde/typeMaterials.jsonl"
	TypesPath         = "data/sde/types.jsonl"
	BlueprintsPath    = "data/sde/blueprints.jsonl"
	GroupsPath        = "data/sde/groups.jsonl"
	CategoriesPath    = "data/sde/categories.jsonl"
)

// defaultSdeURL is CCP's latest SDE in JSON Lines; SDE_URL in config/.env
// overrides it.
const defaultSdeURL = "https://developers.eveonline.com/static-data/eve-online-static-data-latest-jsonl.zip"

// ErrNotLoaded means the SDE hasn't been read yet, e.g. while it's still
// downloading.
var ErrNotLoaded = errors.New("EVE's static data (SDE) isn't loaded yet")

// Material is one thing a portion of an item refines into.
type Material struct {
	TypeID   int64
	Quantity int64
}

// Blueprint is what one run of a blueprint makes: product type ID to
// quantity, for manufacturing and for reactions.
type Blueprint struct {
	Manufacturing map[int64]int64
	Reaction      map[int64]int64
}

// Group is an item group, e.g. Fuel Block, and the category it's in, e.g.
// Material.
type Group struct {
	Name       string
	CategoryID int64
}

// sdeData is everything read from the SDE files.
type sdeData struct {
	materials  map[int64][]Material
	volumes    map[int64]float64
	typeGroups map[int64]int64  // type ID to group ID
	portions   map[int64]int64  // type ID to units refined at a time
	typeIDs    map[string]int64 // English name to type ID
	onMarket   map[int64]bool   // types that can be bought and sold on the market
	items      map[int64]*Item  // published types, for item search
	sorted     []*Item          // items by lowercase name
	byName     map[string]*Item // items by lowercase name
	blueprints map[int64]Blueprint
	groups     map[int64]Group
	categories map[int64]string
}

var (
	mu      sync.Mutex
	loaded  sdeData
	loadErr error = ErrNotLoaded
)

// Materials returns what each type refines into, per portion, keyed by type
// ID. Types that can't be refined, such as gas, aren't in it.
func Materials() (map[int64][]Material, error) {
	mu.Lock()
	defer mu.Unlock()
	return loaded.materials, loadErr
}

// Volumes returns the volume of one unit of each type in m3, keyed by type
// ID.
func Volumes() (map[int64]float64, error) {
	mu.Lock()
	defer mu.Unlock()
	return loaded.volumes, loadErr
}

// ItemType returns the category and group of a type, e.g. "Module" and
// "Missile Launcher Light", and false when the SDE doesn't know it or isn't
// loaded.
func ItemType(typeID int64) (category, group string, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	groupID, ok := loaded.typeGroups[typeID]
	if !ok {
		return "", "", false
	}
	g, ok := loaded.groups[groupID]
	if !ok {
		return "", "", false
	}
	return loaded.categories[g.CategoryID], g.Name, true
}

// TypeIDByName finds a type by its exact English name, preferring a
// published type when several share the name.
func TypeIDByName(name string) (int64, bool) {
	mu.Lock()
	defer mu.Unlock()
	typeID, ok := loaded.typeIDs[name]
	return typeID, ok
}

// OnMarket reports whether the type can be traded on the market, so has a
// price worth looking up. It is false for everything until the SDE loads.
func OnMarket(typeID int64) bool {
	mu.Lock()
	defer mu.Unlock()
	return loaded.onMarket[typeID]
}

// PortionSize returns how many units of a type are refined at a time, e.g.
// 100 for asteroid ore and 1 for ice.
func PortionSize(typeID int64) int64 {
	mu.Lock()
	defer mu.Unlock()
	return max(loaded.portions[typeID], 1)
}

// Blueprints returns what one run of each blueprint makes, keyed by the
// blueprint's type ID.
func Blueprints() (map[int64]Blueprint, error) {
	mu.Lock()
	defer mu.Unlock()
	return loaded.blueprints, loadErr
}

// LoadInBackground reads the SDE files, downloading the SDE first when
// data/sde is missing any of them, retrying every hour until it works.
func LoadInBackground() {
	go func() {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			err := load(ctx)
			cancel()
			if err == nil {
				return
			}
			log.Printf("sde: %v", err)
			time.Sleep(time.Hour)
		}
	}()
}

func load(ctx context.Context) error {
	var missing []string
	for _, dest := range []string{TypeMaterialsPath, TypesPath, BlueprintsPath, GroupsPath, CategoriesPath} {
		if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
			missing = append(missing, dest)
		}
	}
	if len(missing) > 0 {
		url := os.Getenv("SDE_URL")
		if url == "" {
			url = defaultSdeURL
		}
		log.Printf("sde: %v missing, downloading the SDE from %s", missing, url)
		if err := download(ctx, url, missing); err != nil {
			err = fmt.Errorf("downloading the SDE: %w", err)
			setLoaded(sdeData{}, err)
			return err
		}
	}

	var data sdeData
	var err error
	if data.materials, err = parseFile(TypeMaterialsPath, ParseTypeMaterials); err != nil {
		setLoaded(sdeData{}, err)
		return err
	}
	types, err := parseFile(TypesPath, ParseTypes)
	if err != nil {
		setLoaded(sdeData{}, err)
		return err
	}
	data.volumes = make(map[int64]float64, len(types))
	data.typeGroups = make(map[int64]int64, len(types))
	data.portions = make(map[int64]int64, len(types))
	data.typeIDs = make(map[string]int64, len(types))
	data.onMarket = make(map[int64]bool, len(types))
	data.items = make(map[int64]*Item, len(types))
	for typeID, info := range types {
		if info.Volume != nil {
			data.volumes[typeID] = *info.Volume
		}
		data.typeGroups[typeID] = info.GroupID
		data.portions[typeID] = info.PortionSize
		if info.Published && info.MarketGroupID != 0 {
			data.onMarket[typeID] = true
		}
		if other, taken := data.typeIDs[info.Name]; !taken || (info.Published && !types[other].Published) {
			data.typeIDs[info.Name] = typeID
		}
	}
	if data.blueprints, err = parseFile(BlueprintsPath, ParseBlueprints); err != nil {
		setLoaded(sdeData{}, err)
		return err
	}
	if data.groups, err = parseFile(GroupsPath, ParseGroups); err != nil {
		setLoaded(sdeData{}, err)
		return err
	}
	if data.categories, err = parseFile(CategoriesPath, ParseCategories); err != nil {
		setLoaded(sdeData{}, err)
		return err
	}
	data.indexItems(types)
	log.Printf("sde: loaded %d types, refine materials for %d and %d blueprints",
		len(types), len(data.materials), len(data.blueprints))
	setLoaded(data, nil)
	return nil
}

func parseFile[T any](name string, parse func(io.Reader) (T, error)) (T, error) {
	file, err := os.Open(name)
	if err != nil {
		var zero T
		return zero, err
	}
	defer file.Close()
	parsed, err := parse(file)
	if err != nil {
		return parsed, fmt.Errorf("reading %s: %w", name, err)
	}
	return parsed, nil
}

// setLoaded keeps what was loaded before when a later load fails.
func setLoaded(data sdeData, err error) {
	mu.Lock()
	defer mu.Unlock()
	if err == nil {
		loaded = data
	}
	if loaded.materials == nil || err == nil {
		loadErr = err
	}
}

// ParseTypeMaterials reads the SDE's typeMaterials.jsonl: one line per type,
// {"_key": typeID, "materials": [{"materialTypeID": ..., "quantity": ...}]}.
func ParseTypeMaterials(r io.Reader) (map[int64][]Material, error) {
	loaded := map[int64][]Material{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var entry struct {
			Key       int64 `json:"_key"`
			Materials []struct {
				MaterialTypeID int64 `json:"materialTypeID"`
				Quantity       int64 `json:"quantity"`
			} `json:"materials"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		for _, material := range entry.Materials {
			loaded[entry.Key] = append(loaded[entry.Key], Material{TypeID: material.MaterialTypeID, Quantity: material.Quantity})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, errors.New("no materials found")
	}
	return loaded, nil
}

// TypeInfo is what the app uses from a type in the SDE.
type TypeInfo struct {
	Name        string   // in English
	Volume      *float64 // m3 per unit; nil when the SDE gives none
	GroupID     int64
	PortionSize int64
	Published   bool
	// MarketGroupID is 0 for types that can't be traded on the market
	MarketGroupID int64
	BasePrice     float64 // what NPCs sell it for, where they do; 0 when the SDE gives none
}

// ParseTypes reads each type's name, volume, group and portion size from
// the SDE's types.jsonl: one line per type, {"_key": typeID, "groupID": ...,
// "basePrice": ..., "marketGroupID": ..., "name": {"en": ...},
// "portionSize": ..., "published": ..., "volume": m3}.
func ParseTypes(r io.Reader) (map[int64]TypeInfo, error) {
	loaded := map[int64]TypeInfo{}
	err := eachLine(r, func(line []byte) error {
		var entry struct {
			Key           int64             `json:"_key"`
			BasePrice     float64           `json:"basePrice"`
			GroupID       int64             `json:"groupID"`
			MarketGroupID int64             `json:"marketGroupID"`
			Name          map[string]string `json:"name"`
			PortionSize   int64             `json:"portionSize"`
			Published     bool              `json:"published"`
			Volume        *float64          `json:"volume"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			return err
		}
		loaded[entry.Key] = TypeInfo{Name: entry.Name["en"], Volume: entry.Volume, GroupID: entry.GroupID,
			PortionSize: entry.PortionSize, Published: entry.Published, MarketGroupID: entry.MarketGroupID,
			BasePrice: entry.BasePrice}
		return nil
	})
	if err == nil && len(loaded) == 0 {
		err = errors.New("no types found")
	}
	return loaded, err
}

// ParseGroups reads the SDE's groups.jsonl: one line per group,
// {"_key": groupID, "categoryID": ..., "name": {"en": ...}}.
func ParseGroups(r io.Reader) (map[int64]Group, error) {
	loaded := map[int64]Group{}
	err := eachLine(r, func(line []byte) error {
		var entry struct {
			Key        int64             `json:"_key"`
			CategoryID int64             `json:"categoryID"`
			Name       map[string]string `json:"name"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			return err
		}
		loaded[entry.Key] = Group{Name: entry.Name["en"], CategoryID: entry.CategoryID}
		return nil
	})
	if err == nil && len(loaded) == 0 {
		err = errors.New("no groups found")
	}
	return loaded, err
}

// ParseCategories reads the SDE's categories.jsonl: one line per category,
// {"_key": categoryID, "name": {"en": ...}}.
func ParseCategories(r io.Reader) (map[int64]string, error) {
	loaded := map[int64]string{}
	err := eachLine(r, func(line []byte) error {
		var entry struct {
			Key  int64             `json:"_key"`
			Name map[string]string `json:"name"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			return err
		}
		loaded[entry.Key] = entry.Name["en"]
		return nil
	})
	if err == nil && len(loaded) == 0 {
		err = errors.New("no categories found")
	}
	return loaded, err
}

// eachLine calls handle with every non-blank line of a JSON Lines file.
func eachLine(r io.Reader, handle func(line []byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		if err := handle(scanner.Bytes()); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
	return scanner.Err()
}

// ParseBlueprints reads what one run of each blueprint makes from the
// SDE's blueprints.jsonl: one line per blueprint, {"_key": typeID,
// "activities": {"manufacturing": {"products": [{"typeID": ...,
// "quantity": ...}]}, "reaction": {...}}}.
func ParseBlueprints(r io.Reader) (map[int64]Blueprint, error) {
	type activity struct {
		Products []struct {
			TypeID   int64 `json:"typeID"`
			Quantity int64 `json:"quantity"`
		} `json:"products"`
	}
	products := func(a *activity) map[int64]int64 {
		if a == nil || len(a.Products) == 0 {
			return nil
		}
		out := make(map[int64]int64, len(a.Products))
		for _, product := range a.Products {
			out[product.TypeID] = product.Quantity
		}
		return out
	}

	loaded := map[int64]Blueprint{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var entry struct {
			Key        int64 `json:"_key"`
			Activities struct {
				Manufacturing *activity `json:"manufacturing"`
				Reaction      *activity `json:"reaction"`
			} `json:"activities"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		blueprint := Blueprint{Manufacturing: products(entry.Activities.Manufacturing), Reaction: products(entry.Activities.Reaction)}
		if blueprint.Manufacturing != nil || blueprint.Reaction != nil {
			loaded[entry.Key] = blueprint
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, errors.New("no blueprints found")
	}
	return loaded, nil
}

// download fetches the SDE zip and saves just the files named by dests,
// matching each by its base name.
func download(ctx context.Context, url string, dests []string) error {
	if err := os.MkdirAll(filepath.Dir(dests[0]), 0o755); err != nil {
		return err
	}
	zipFile, err := os.CreateTemp(filepath.Dir(dests[0]), "sde-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(zipFile.Name())
	defer zipFile.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", esi.UserAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, resp.Status)
	}
	size, err := io.Copy(zipFile, resp.Body)
	if err != nil {
		return err
	}

	archive, err := zip.NewReader(zipFile, size)
	if err != nil {
		return err
	}
	for _, dest := range dests {
		i := slices.IndexFunc(archive.File, func(entry *zip.File) bool {
			return path.Base(entry.Name) == filepath.Base(dest)
		})
		if i < 0 {
			return fmt.Errorf("the SDE has no %s", filepath.Base(dest))
		}
		if err := extract(archive.File[i], dest); err != nil {
			return err
		}
	}
	return nil
}

func extract(entry *zip.File, dest string) error {
	in, err := entry.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	partial := dest + ".partial"
	out, err := os.Create(partial)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(partial, dest)
}

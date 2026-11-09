package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakePlexWatchlist is plex.tv and Plex's catalogue service in one: PINs
// that are approved at once, and a watchlist the test can change.
type fakePlexWatchlist struct {
	*httptest.Server
	mu    sync.Mutex
	items []map[string]any // MediaContainer.Metadata entries
	ids   map[string][]string
}

func (f *fakePlexWatchlist) put(key, kind, title string, year int, guids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = append([]map[string]any{{"ratingKey": key, "type": kind, "title": title, "year": year}}, f.items...)
	f.ids[key] = guids
}

func newFakePlexWatchlist(t *testing.T) *fakePlexWatchlist {
	t.Helper()
	f := &fakePlexWatchlist{ids: map[string][]string{}}
	token := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("X-Plex-Token") != "account-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id": 42, "code": "abcd", "expiresIn": 1800}`)
	})
	mux.HandleFunc("GET /api/v2/pins/42", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id": 42, "code": "abcd", "authToken": "account-token"}`)
	})
	mux.HandleFunc("GET /api/v2/user", func(w http.ResponseWriter, r *http.Request) {
		if token(w, r) {
			fmt.Fprint(w, `{"username": "plexfan", "title": "Plex Fan"}`)
		}
	})
	mux.HandleFunc("GET /library/sections/watchlist/all", func(w http.ResponseWriter, r *http.Request) {
		if !token(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"totalSize": len(f.items), "Metadata": f.items}})
	})
	mux.HandleFunc("GET /library/metadata/{key}", func(w http.ResponseWriter, r *http.Request) {
		if !token(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		var guids []map[string]string
		for _, g := range f.ids[r.PathValue("key")] {
			guids = append(guids, map[string]string{"id": g})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []map[string]any{{"ratingKey": r.PathValue("key"), "Guid": guids}}}})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func connectWatchlist(t *testing.T, client *http.Client, base string) map[string]any {
	t.Helper()
	pin := postJSON[map[string]any](t, client, base+"/api/plex-watchlist/pin", map[string]any{}, http.StatusOK)
	if pin["code"] != "abcd" {
		t.Fatalf("pin = %v", pin)
	}
	done := getJSON[map[string]any](t, client, base+"/api/plex-watchlist/pin/42")
	if done["done"] != true {
		t.Fatalf("pin poll = %v", done)
	}
	return done["status"].(map[string]any)
}

// secondAdmin adds another administrator account, "ada", and signs it in.
func secondAdmin(t *testing.T, admin *http.Client, base string) (ada *http.Client, id float64) {
	t.Helper()
	created := postJSON[map[string]any](t, admin, base+"/api/users", map[string]any{
		"username": "ada", "password": "ada-long-password", "name": "Ada", "role": "admin",
	}, http.StatusCreated)
	return erLoginAs(t, base, "ada", "ada-long-password"), created["id"].(float64)
}

func TestPlexWatchlistAddsWhatComesNext(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	server.TestSetTMDBBaseURL("fixture-key", arrTMDB(t).URL)
	plex := newFakePlexWatchlist(t)
	server.TestSetPlexTVURL(plex.URL)
	server.TestSetPlexDiscoverURL(plex.URL)

	// Already on the list when it's connected: shown, not added.
	plex.put("old1", "movie", "Night Harbour", 2021, "tmdb://701", "imdb://tt0000002")
	st := connectWatchlist(t, admin, base)
	if st["connected"] != true || st["name"] != "plexfan" || st["autoAdd"] != true {
		t.Fatalf("status = %v", st)
	}
	check := postJSON[map[string]any](t, admin, base+"/api/plex-watchlist/check", map[string]any{}, http.StatusOK)
	if check["added"].(float64) != 0 {
		t.Fatalf("a title already on the list was added: %v", check)
	}
	items := getJSON[[]map[string]any](t, admin, base+"/api/plex-watchlist/items")
	if len(items) != 1 || items[0]["tmdbId"].(float64) != 701 || items[0]["mediaType"] != "movie" {
		t.Fatalf("items = %v", items)
	}

	// Something new: an administrator's watchlist adds it.
	plex.put("new1", "show", "Salt & Static", 2019, "tvdb://777001", "tmdb://9001")
	check = postJSON[map[string]any](t, admin, base+"/api/plex-watchlist/check", map[string]any{}, http.StatusOK)
	if check["added"].(float64) != 1 {
		t.Fatalf("check = %v", check)
	}
	series := getJSON[[]map[string]any](t, admin, base+"/api/series")
	if len(series) != 1 || series[0]["tmdbId"].(float64) != 9001 {
		t.Fatalf("series = %v", series)
	}
	// Seen once is handled once.
	check = postJSON[map[string]any](t, admin, base+"/api/plex-watchlist/check", map[string]any{}, http.StatusOK)
	if check["added"].(float64) != 0 {
		t.Fatalf("added twice: %v", check)
	}

	// Switching it off and disconnecting.
	off := putJSONStatus(t, admin, base+"/api/plex-watchlist", map[string]any{"autoAdd": false}, http.StatusOK)
	if off["autoAdd"] != false {
		t.Fatalf("off = %v", off)
	}
	if code := deleteReq(t, admin, base+"/api/plex-watchlist"); code != http.StatusNoContent {
		t.Fatalf("disconnect: %d", code)
	}
	if st := getJSON[map[string]any](t, admin, base+"/api/plex-watchlist"); st["connected"] != false {
		t.Fatalf("after disconnect: %v", st)
	}
}

// Linked services are for administrators only: the watchlist of an
// administrator who is made a basic account adds nothing any more.
func TestPlexWatchlistOfABasicAccountAddsNothing(t *testing.T) {
	server, base, admin, _, _ := familyServer(t)
	server.TestSetTMDBBaseURL("fixture-key", arrTMDB(t).URL)
	plex := newFakePlexWatchlist(t)
	server.TestSetPlexTVURL(plex.URL)
	server.TestSetPlexDiscoverURL(plex.URL)

	ada, adaID := secondAdmin(t, admin, base)
	connectWatchlist(t, ada, base)
	postJSONMethod[map[string]any](t, admin, http.MethodPut, fmt.Sprintf("%s/api/users/%v", base, adaID), map[string]any{"role": "member"}, http.StatusOK)

	plex.put("new1", "movie", "Night Harbour", 2021, "tmdb://701")
	resp, err := admin.Post(fmt.Sprintf("%s/api/import-lists/plex/%v/run", base, adaID), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("the watchlist of a basic account was checked")
	}
	if movies := getJSON[[]map[string]any](t, admin, base+"/api/movies"); len(movies) != 0 {
		t.Fatalf("a basic account's watchlist added %v", movies)
	}
}

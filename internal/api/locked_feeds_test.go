package api_test

import (
	"net/http"
	"testing"
)

// lockedEReaderFamily is a server with ebooks on and a member "sam" holding an
// OPDS password and a Kobo address (left from before e-readers were for
// administrators only), plus the admin's own pair.
type lockedEReaderFamily struct {
	base, opdsPass, koboPath string
	adminPass, adminKobo     string
	lock                     func(userID int64)
}

func newLockedEReaderFamily(t *testing.T) (f lockedEReaderFamily, memberID, adminID int64) {
	t.Helper()
	server, base, admin, _, id := familyServer(t)
	postJSONMethod[map[string]any](t, admin, http.MethodPut, base+"/api/modules", map[string]any{"ebooks": true}, http.StatusOK)
	opds, err := server.EReader.NewOPDSPassword(int64(id))
	if err != nil {
		t.Fatal(err)
	}
	kobo, err := server.EReader.NewKoboToken(int64(id))
	if err != nil {
		t.Fatal(err)
	}
	adminOPDS := postJSON[ereaderAccess](t, admin, base+"/api/auth/ereader/opds", nil, http.StatusOK)
	adminKobo := postJSON[ereaderAccess](t, admin, base+"/api/auth/ereader/kobo", nil, http.StatusOK)
	f = lockedEReaderFamily{base: base, opdsPass: opds, koboPath: "/kobo/" + kobo, adminPass: adminOPDS.Password, adminKobo: adminKobo.Kobo.Path}
	f.lock = func(userID int64) {
		if err := server.TestExec(`UPDATE users SET subscription_locked = 1 WHERE id = ?`, userID); err != nil {
			t.Fatalf("lock the account: %v", err)
		}
	}
	return f, int64(id), 1
}

func TestLockedAccountIsRefusedOnTheOPDSCatalogue(t *testing.T) {
	f, memberID, adminID := newLockedEReaderFamily(t)
	client := erClient()
	if resp, _ := erOPDSGet(t, client, f.base+"/opds/", "ryan", f.adminPass); resp.StatusCode != http.StatusOK {
		t.Fatalf("before the lock: %d", resp.StatusCode)
	}
	f.lock(memberID)
	f.lock(adminID)
	if resp, _ := erOPDSGet(t, client, f.base+"/opds/", "sam", f.opdsPass); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a locked member reached the catalogue: %d", resp.StatusCode)
	}
	if resp, _ := erOPDSGet(t, client, f.base+"/opds/", "ryan", f.adminPass); resp.StatusCode != http.StatusOK {
		t.Errorf("an administrator with the flag set was refused: %d", resp.StatusCode)
	}
}

func TestLockedAccountIsRefusedOnKoboSync(t *testing.T) {
	f, memberID, adminID := newLockedEReaderFamily(t)
	if resp, _ := erGet(t, f.base+f.adminKobo+"/v1/library/sync"); resp.StatusCode != http.StatusOK {
		t.Fatalf("before the lock: %d", resp.StatusCode)
	}
	f.lock(memberID)
	f.lock(adminID)
	if resp, _ := erGet(t, f.base+f.koboPath+"/v1/library/sync"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a locked member synced: %d", resp.StatusCode)
	}
	if resp, _ := erGet(t, f.base+f.adminKobo+"/v1/library/sync"); resp.StatusCode != http.StatusOK {
		t.Errorf("an administrator with the flag set was refused: %d", resp.StatusCode)
	}
}

func TestLockedAccountIsRefusedOnTheCalendarFeed(t *testing.T) {
	server, base, admin, member, memberID := familyServer(t)
	type feed struct {
		Path string `json:"path"`
	}
	mine := postJSON[feed](t, member, base+"/api/calendar/feed", nil, http.StatusOK)
	theirs := postJSON[feed](t, admin, base+"/api/calendar/feed", nil, http.StatusOK)
	get := func(path string) int {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := get(mine.Path); got != http.StatusOK {
		t.Fatalf("before the lock: %d", got)
	}
	for _, id := range []int64{int64(memberID), 1} {
		if err := server.TestExec(`UPDATE users SET subscription_locked = 1 WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
	}
	if got := get(mine.Path); got != http.StatusNotFound {
		t.Errorf("a locked member's feed answered %d, want 404", got)
	}
	if got := get(theirs.Path); got != http.StatusOK {
		t.Errorf("an administrator's feed with the flag set answered %d", got)
	}
}

// The lock check reads the account. When that read fails the feed says it is
// unavailable, as for any other server trouble, rather than that it is gone.
func TestCalendarFeedIsUnavailableWhenTheAccountCannotBeRead(t *testing.T) {
	server, base, _, member, _ := familyServer(t)
	type feed struct {
		Path string `json:"path"`
	}
	mine := postJSON[feed](t, member, base+"/api/calendar/feed", nil, http.StatusOK)
	// The feed's own lookup still works; reading the account no longer does.
	if err := server.TestExec(`ALTER TABLE users DROP COLUMN subscription_locked`); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(base + mine.Path)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

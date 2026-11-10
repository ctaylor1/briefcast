package service

import (
	"os"
	"testing"

	"github.com/ctaylor1/briefcast/db"
)

// savePodcastItemFixture writes a whole episode row for test setup. Production
// code writes named columns with db.UpdatePodcastItemColumns instead.
func savePodcastItemFixture(item *db.PodcastItem) error {
	return db.DB.Omit("Podcast").Save(item).Error
}

// setStubBaseURL tells a stub helper script which test server to point at.
// Helpers no longer inherit the whole environment, so the URL goes in a file
// next to the script, which the script reads as __file__ + ".base".
func setStubBaseURL(t *testing.T, scriptPath, baseURL string) {
	t.Helper()
	if err := os.WriteFile(scriptPath+".base", []byte(baseURL), 0o644); err != nil {
		t.Fatal(err)
	}
}

package service

import "github.com/ctaylor1/briefcast/db"

// savePodcastItemFixture writes a whole episode row for test setup. Production
// code writes named columns with db.UpdatePodcastItemColumns instead.
func savePodcastItemFixture(item *db.PodcastItem) error {
	return db.DB.Omit("Podcast").Save(item).Error
}

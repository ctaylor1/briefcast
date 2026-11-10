package db

// savePodcastItemFixture writes a whole episode row for test setup. Production
// code writes named columns with UpdatePodcastItemColumns instead.
func savePodcastItemFixture(item *PodcastItem) error {
	return DB.Omit("Podcast").Save(item).Error
}

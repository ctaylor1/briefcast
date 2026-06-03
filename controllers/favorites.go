package controllers

import "github.com/ctaylor1/briefcast/db"

func isPodcastItemFavorited(item db.PodcastItem) bool {
	return item.IsSummaryFavorited || !item.BookmarkDate.IsZero()
}

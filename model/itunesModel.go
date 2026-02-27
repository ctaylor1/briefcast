package model

import "time"

// ItunesResponse represents a public type.
type ItunesResponse struct {
	ResultCount int                  `json:"resultCount"`
	Results     []ItunesSingleResult `json:"results"`
}

// ItunesSingleResult represents a public type.
type ItunesSingleResult struct {
	WrapperType            string    `json:"wrapperType"`
	Kind                   string    `json:"kind"`
	CollectionID           int       `json:"collectionID"`
	TrackID                int       `json:"trackID"`
	ArtistName             string    `json:"artistName"`
	CollectionName         string    `json:"collectionName"`
	TrackName              string    `json:"trackName"`
	CollectionCensoredName string    `json:"collectionCensoredName"`
	TrackCensoredName      string    `json:"trackCensoredName"`
	CollectionViewURL      string    `json:"collectionViewURL"`
	FeedURL                string    `json:"feedURL"`
	TrackViewURL           string    `json:"trackViewURL"`
	ArtworkURL30           string    `json:"artworkUrl30"`
	ArtworkURL60           string    `json:"artworkUrl60"`
	ArtworkURL100          string    `json:"artworkUrl100"`
	CollectionPrice        float64   `json:"collectionPrice"`
	TrackPrice             float64   `json:"trackPrice"`
	TrackRentalPrice       int       `json:"trackRentalPrice"`
	CollectionHdPrice      int       `json:"collectionHdPrice"`
	TrackHdPrice           int       `json:"trackHdPrice"`
	TrackHdRentalPrice     int       `json:"trackHdRentalPrice"`
	ReleaseDate            time.Time `json:"releaseDate"`
	CollectionExplicitness string    `json:"collectionExplicitness"`
	TrackExplicitness      string    `json:"trackExplicitness"`
	TrackCount             int       `json:"trackCount"`
	Country                string    `json:"country"`
	Currency               string    `json:"currency"`
	PrimaryGenreName       string    `json:"primaryGenreName"`
	ContentAdvisoryRating  string    `json:"contentAdvisoryRating,omitempty"`
	ArtworkURL600          string    `json:"artworkUrl600"`
	GenreIds               []string  `json:"genreIds"`
	Genres                 []string  `json:"genres"`
	ArtistID               int       `json:"artistID,omitempty"`
	ArtistViewURL          string    `json:"artistViewURL,omitempty"`
}

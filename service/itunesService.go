package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/TheHippo/podcastindex"
	"github.com/ctaylor1/briefcast/model"
)

// SearchService represents a public type.
type SearchService interface {
	Query(q string) []*model.CommonSearchResultModel
}

// ItunesService represents a public type.
type ItunesService struct {
}

var itunesBaseURL = "https://itunes.apple.com"

// Query handles the corresponding operation.
func (service ItunesService) Query(q string) []*model.CommonSearchResultModel {
	url := fmt.Sprintf("%s/search?term=%s&entity=podcast", itunesBaseURL, url.QueryEscape(q))

	body, err := makeQuery(url)
	if err != nil {
		Logger.Warnw("itunes search failed", "url", url, "error", err)
		return []*model.CommonSearchResultModel{}
	}
	var response model.ItunesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		Logger.Warnw("itunes response decode failed", "url", url, "error", err)
		return []*model.CommonSearchResultModel{}
	}

	var toReturn []*model.CommonSearchResultModel

	for _, obj := range response.Results {
		toReturn = append(toReturn, GetSearchFromItunes(obj))
	}

	return toReturn
}

// PodcastIndexService represents a public type.
type PodcastIndexService struct {
}

const (
	// PodcastIndexKeyEnv is a public constant.
	PodcastIndexKeyEnv = "PODCASTINDEX_KEY"
	// PodcastIndexSecretEnv is a public constant.
	PodcastIndexSecretEnv = "PODCASTINDEX_SECRET" // #nosec G101 -- env var key name only, not a credential value.
)

// Query handles the corresponding operation.
func (service PodcastIndexService) Query(q string) []*model.CommonSearchResultModel {
	var toReturn []*model.CommonSearchResultModel
	key := strings.TrimSpace(os.Getenv(PodcastIndexKeyEnv))
	secret := strings.TrimSpace(os.Getenv(PodcastIndexSecretEnv))
	if key == "" || secret == "" {
		Logger.Warnw("podcastindex credentials missing", "key_env", PodcastIndexKeyEnv, "secret_env", PodcastIndexSecretEnv)
		return toReturn
	}

	c := newPodcastIndexClient(key, secret)
	podcasts, err := c.Search(q)
	if err != nil {
		Logger.Warnw("podcastindex search failed", "error", err)
		return toReturn
	}

	for _, obj := range podcasts {
		toReturn = append(toReturn, GetSearchFromPodcastIndex(obj))
	}

	return toReturn
}

// podcastIndexSearchTimeout bounds a Podcast Index search; the library's
// default client has no timeout, so a stalled upstream hung the request.
var podcastIndexSearchTimeout = 15 * time.Second

// podcastIndexBaseURL is a seam for tests.
var podcastIndexBaseURL = podcastindex.BaseURL

func newPodcastIndexClient(key, secret string) *podcastindex.Client {
	config := *podcastindex.DefaultConfig
	config.BaseURL = podcastIndexBaseURL
	return podcastindex.NewClientWithConfig(key, secret, config, &http.Client{
		Timeout:   podcastIndexSearchTimeout,
		Transport: outboundHTTPTransport(outboundPurposeHTTP),
	})
}

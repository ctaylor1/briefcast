package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/TheHippo/podcastindex"
	"github.com/ctaylor1/briefcast/model"
)

type SearchService interface {
	Query(q string) []*model.CommonSearchResultModel
}

type ItunesService struct {
}

const ITUNES_BASE = "https://itunes.apple.com"

func (service ItunesService) Query(q string) []*model.CommonSearchResultModel {
	url := fmt.Sprintf("%s/search?term=%s&entity=podcast", ITUNES_BASE, url.QueryEscape(q))

	body, err := makeQuery(url)
	if err != nil {
		Logger.Warnw("itunes search failed", "url", url, "error", err)
	}
	var response model.ItunesResponse
	json.Unmarshal(body, &response)

	var toReturn []*model.CommonSearchResultModel

	for _, obj := range response.Results {
		toReturn = append(toReturn, GetSearchFromItunes(obj))
	}

	return toReturn
}

type PodcastIndexService struct {
}

const (
	PodcastIndexKeyEnv    = "PODCASTINDEX_KEY"
	PodcastIndexSecretEnv = "PODCASTINDEX_SECRET"
)

func (service PodcastIndexService) Query(q string) []*model.CommonSearchResultModel {
	var toReturn []*model.CommonSearchResultModel
	key := strings.TrimSpace(os.Getenv(PodcastIndexKeyEnv))
	secret := strings.TrimSpace(os.Getenv(PodcastIndexSecretEnv))
	if key == "" || secret == "" {
		Logger.Warnw("podcastindex credentials missing", "key_env", PodcastIndexKeyEnv, "secret_env", PodcastIndexSecretEnv)
		return toReturn
	}

	c := podcastindex.NewClient(key, secret)
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

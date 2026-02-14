package service

import (
	"encoding/json"
	"fmt"
	"log"
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

	body, _ := makeQuery(url)
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
		log.Printf("podcastindex credentials missing; set %s and %s", PodcastIndexKeyEnv, PodcastIndexSecretEnv)
		return toReturn
	}

	c := podcastindex.NewClient(key, secret)
	podcasts, err := c.Search(q)
	if err != nil {
		log.Printf("podcastindex search failed: %v", err)
		return toReturn
	}

	for _, obj := range podcasts {
		toReturn = append(toReturn, GetSearchFromPodcastIndex(obj))
	}

	return toReturn
}


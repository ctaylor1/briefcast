package service

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/ctaylor1/briefcast/model"
)

// type GoodReadsService struct {
// }

var gpodderBaseURL = "https://gpodder.net"

// Query handles the corresponding operation.
func Query(q string) []*model.CommonSearchResultModel {
	url := fmt.Sprintf("%s/search.json?q=%s", gpodderBaseURL, url.QueryEscape(q))

	body, err := makeQuery(url)
	if err != nil {
		logging.Sugar().Warnw("gpodder query failed", "url", url, "error", err)
		return []*model.CommonSearchResultModel{}
	}
	var response []model.GPodcast
	if err := json.Unmarshal(body, &response); err != nil {
		logging.Sugar().Warnw("failed to decode gpodder query response", "url", url, "error", err)
		return []*model.CommonSearchResultModel{}
	}

	var toReturn []*model.CommonSearchResultModel

	for _, obj := range response {
		toReturn = append(toReturn, GetSearchFromGpodder(obj))
	}

	return toReturn
}

// ByTag handles the corresponding operation.
func ByTag(tag string, count int) []model.GPodcast {
	url := fmt.Sprintf("%s/api/2/tag/%s/%d.json", gpodderBaseURL, url.QueryEscape(tag), count)

	body, err := makeQuery(url)
	if err != nil {
		logging.Sugar().Warnw("gpodder by-tag query failed", "url", url, "error", err)
		return []model.GPodcast{}
	}
	var response []model.GPodcast
	if err := json.Unmarshal(body, &response); err != nil {
		logging.Sugar().Warnw("failed to decode gpodder by-tag response", "url", url, "error", err)
		return []model.GPodcast{}
	}
	return response
}

// Top handles the corresponding operation.
func Top(count int) []model.GPodcast {
	url := fmt.Sprintf("%s/toplist/%d.json", gpodderBaseURL, count)

	body, err := makeQuery(url)
	if err != nil {
		logging.Sugar().Warnw("gpodder toplist query failed", "url", url, "error", err)
		return []model.GPodcast{}
	}
	var response []model.GPodcast
	if err := json.Unmarshal(body, &response); err != nil {
		logging.Sugar().Warnw("failed to decode gpodder toplist response", "url", url, "error", err)
		return []model.GPodcast{}
	}
	return response
}

// Tags handles the corresponding operation.
func Tags(count int) []model.GPodcastTag {
	url := fmt.Sprintf("%s/api/2/tags/%d.json", gpodderBaseURL, count)

	body, err := makeQuery(url)
	if err != nil {
		logging.Sugar().Warnw("gpodder tags query failed", "url", url, "error", err)
		return []model.GPodcastTag{}
	}
	var response []model.GPodcastTag
	if err := json.Unmarshal(body, &response); err != nil {
		logging.Sugar().Warnw("failed to decode gpodder tags response", "url", url, "error", err)
		return []model.GPodcastTag{}
	}
	return response
}

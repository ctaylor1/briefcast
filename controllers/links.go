package controllers

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/internal/feedmeta"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultTopLinksLimit         = 50
	maxTopLinksLimit             = 200
	defaultLinkOccurrencesLimit  = 100
	maxLinkOccurrencesLimit      = 500
	linkDateOnlyLayout           = "2006-01-02"
	maxLinkPlatformFilterLength  = 32
	maxLinkPodcastIDFilterLength = 64
)

var linkKinds = map[string]bool{
	string(feedmeta.LinkKindProfile):   true,
	string(feedmeta.LinkKindPost):      true,
	string(feedmeta.LinkKindShortener): true,
	string(feedmeta.LinkKindWeb):       true,
}

var linkPlatformRe = regexp.MustCompile(`^[a-z0-9]+$`)

// TopLinksQuery holds the GET /links/top query parameters. Fields stay strings
// so each invalid value gets its own error message.
type TopLinksQuery struct {
	Kind               string `form:"kind"`
	Platform           string `form:"platform"`
	PodcastID          string `form:"podcastId"`
	Since              string `form:"since"`
	Until              string `form:"until"`
	IncludeBoilerplate string `form:"includeBoilerplate"`
	Limit              string `form:"limit"`
}

// LinkTargetQuery holds the GET /links/targets/:id query parameters.
type LinkTargetQuery struct {
	Limit string `form:"limit"`
}

// GetTopLinks ranks show note link targets by the number of episodes that
// link to them.
//
// Query parameters (all optional): kind (profile, post, shortener, web),
// platform (x, youtube, web, ...), podcastId, since and until (YYYY-MM-DD or
// RFC 3339, by episode pub date; a date-only until includes that day),
// includeBoilerplate (true to keep links that appear in most of a podcast's
// episodes), and limit (1-200, default 50).
func GetTopLinks(c *gin.Context) {
	var params TopLinksQuery
	if c.ShouldBindQuery(&params) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	query := db.TopLinkTargetsQuery{Limit: defaultTopLinksLimit}

	if kind := strings.TrimSpace(params.Kind); kind != "" {
		if !linkKinds[kind] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kind must be one of profile, post, shortener, web"})
			return
		}
		query.Kind = kind
	}
	if platform := strings.ToLower(strings.TrimSpace(params.Platform)); platform != "" {
		if len(platform) > maxLinkPlatformFilterLength || !linkPlatformRe.MatchString(platform) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "platform must be a lowercase platform name such as x or youtube"})
			return
		}
		query.Platform = platform
	}
	if podcastID := strings.TrimSpace(params.PodcastID); podcastID != "" {
		if len(podcastID) > maxLinkPodcastIDFilterLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "podcastId is too long"})
			return
		}
		query.PodcastID = podcastID
	}

	var err error
	if query.Since, err = parseLinkDateParam(params.Since, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "since must be a date (YYYY-MM-DD) or RFC 3339 timestamp"})
		return
	}
	if query.Until, err = parseLinkDateParam(params.Until, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "until must be a date (YYYY-MM-DD) or RFC 3339 timestamp"})
		return
	}
	if query.Since != nil && query.Until != nil && !query.Since.Before(*query.Until) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "since must be before until"})
		return
	}

	if raw := strings.TrimSpace(params.IncludeBoilerplate); raw != "" {
		include, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "includeBoilerplate must be true or false"})
			return
		}
		query.IncludeBoilerplate = include
	}

	limit, ok := parseBoundedLimit(c, params.Limit, defaultTopLinksLimit, maxTopLinksLimit)
	if !ok {
		return
	}
	query.Limit = limit

	ranks, err := db.TopLinkTargets(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rank links"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"count":              len(ranks),
		"includeBoilerplate": query.IncludeBoilerplate,
		"boilerplateRule": gin.H{
			"minEpisodes": db.DefaultBoilerplateMinEpisodes,
			"percent":     db.DefaultBoilerplatePercent,
		},
		"targets": ranks,
	})
}

// GetLinkTarget returns one link target with per-podcast counts and the
// newest episodes that link to it. Query parameter: limit (occurrences,
// 1-500, default 100).
func GetLinkTarget(c *gin.Context) {
	var searchByIDQuery SearchByIDQuery
	if c.ShouldBindUri(&searchByIDQuery) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	var params LinkTargetQuery
	if c.ShouldBindQuery(&params) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}
	limit, ok := parseBoundedLimit(c, params.Limit, defaultLinkOccurrencesLimit, maxLinkOccurrencesLimit)
	if !ok {
		return
	}

	target, err := db.GetLinkTargetByID(searchByIDQuery.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Link target not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load link target"})
		return
	}

	var parent *db.LinkTarget
	if target.ParentTargetID != nil {
		loaded, parentErr := db.GetLinkTargetByID(*target.ParentTargetID)
		if parentErr != nil && !errors.Is(parentErr, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load link target"})
			return
		}
		if parentErr == nil {
			parent = &loaded
		}
	}

	podcasts, err := db.LinkTargetPodcastCounts(target.ID, db.BoilerplateRule{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count link target references"})
		return
	}
	occurrences, err := db.LinkTargetOccurrences(target.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load link target references"})
		return
	}

	var episodeCount int64
	for _, podcast := range podcasts {
		episodeCount += podcast.EpisodeCount
	}
	response := gin.H{
		"target":       linkTargetJSON(target),
		"episodeCount": episodeCount,
		"podcastCount": len(podcasts),
		"podcasts":     podcasts,
		"occurrences":  occurrences,
	}
	if parent != nil {
		response["parent"] = linkTargetJSON(*parent)
	}
	c.JSON(http.StatusOK, response)
}

func linkTargetJSON(target db.LinkTarget) gin.H {
	payload := gin.H{
		"targetId":      target.ID,
		"normalizedKey": target.NormalizedKey,
		"canonicalUrl":  target.CanonicalURL,
		"kind":          target.Kind,
		"platform":      target.Platform,
		"domain":        target.Domain,
	}
	if target.Handle != "" {
		payload["handle"] = target.Handle
	}
	if target.ParentTargetID != nil {
		payload["parentTargetId"] = *target.ParentTargetID
	}
	return payload
}

// parseLinkDateParam parses YYYY-MM-DD or RFC 3339. With endOfDay, a
// date-only value becomes the start of the next day so that an exclusive
// upper bound includes the named day.
func parseLinkDateParam(raw string, endOfDay bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(linkDateOnlyLayout, raw); err == nil {
		if endOfDay {
			parsed = parsed.AddDate(0, 0, 1)
		}
		return &parsed, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

// parseBoundedLimit parses a limit query value, writing a 400 response and
// returning false when it is invalid.
func parseBoundedLimit(c *gin.Context, raw string, defaultLimit, maxLimit int) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultLimit, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
		return 0, false
	}
	if parsed <= 0 || parsed > maxLimit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and " + strconv.Itoa(maxLimit)})
		return 0, false
	}
	return parsed, true
}

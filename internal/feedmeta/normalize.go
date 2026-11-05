package feedmeta

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// LinkExtractionVersion identifies the show note link extraction and
// normalization rules. Bump it whenever a change would alter stored link data
// (a URL's Key, a target's fields, or an extracted field such as a label), so
// the link backfill re-processes episodes saved under an older version.
const LinkExtractionVersion = 1

// LinkKind classifies what a show note link points at.
type LinkKind string

const (
	// LinkKindProfile is an account on a social platform (x.com/jason).
	LinkKindProfile LinkKind = "profile"
	// LinkKindPost is a single post or video (x.com/jason/status/123).
	LinkKindPost LinkKind = "post"
	// LinkKindShortener is a URL shortener whose destination is unknown
	// until resolved (rb.gy/abc).
	LinkKindShortener LinkKind = "shortener"
	// LinkKindWeb is any other web page (articles, company sites, etc.).
	LinkKindWeb LinkKind = "web"
)

// PlatformWeb is the platform for links that are not on a recognized social
// platform.
const PlatformWeb = "web"

// ErrUnsupportedLink is returned for URLs that cannot be normalized, such as
// non-HTTP schemes or URLs without a host.
var ErrUnsupportedLink = errors.New("unsupported link")

// NormalizedLink is the canonical identity of a show note link.
//
// Key is the stable identity used to count references: two raw URLs that
// point at the same profile, post, or page produce the same Key. CanonicalURL
// is a clean, clickable https form; callers should keep the raw URL too,
// because canonicalization upgrades http to https and drops tracking params.
type NormalizedLink struct {
	Key          string
	CanonicalURL string
	Kind         LinkKind
	Platform     string
	// Handle is the lowercased account handle for profiles and for posts
	// whose author is known from the URL.
	Handle string
	// ParentKey is the author profile's Key for posts whose author is known.
	ParentKey string
	// Domain is the canonical host, without "www." (twitter.com becomes x.com).
	Domain string
}

// NormalizeLink canonicalizes a raw show note URL. It is pure: it performs no
// network requests, so shorteners are classified but not resolved.
func NormalizeLink(rawURL string) (NormalizedLink, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return NormalizedLink{}, ErrUnsupportedLink
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return NormalizedLink{}, ErrUnsupportedLink
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return NormalizedLink{}, ErrUnsupportedLink
	}

	segments := pathSegments(parsed.Path)
	if platform, ok := socialPlatformForHost(host); ok {
		if link, matched := normalizeSocial(platform, host, segments, parsed.Query()); matched {
			return link, nil
		}
		// A recognized platform URL that is not a profile or post (for
		// example x.com/home) is kept as a web page on that platform.
		canonicalHost := platformCanonicalHost[platform]
		return webLink(platform, LinkKindWeb, "web:", canonicalHost, canonicalHost, parsed), nil
	}

	hostPort := host
	if strings.Contains(host, ":") {
		hostPort = "[" + host + "]" // IPv6 literal
	}
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		hostPort += ":" + port
	}
	if shortenerHosts[host] {
		return webLink(PlatformWeb, LinkKindShortener, "short:", host, hostPort, parsed), nil
	}
	return webLink(PlatformWeb, LinkKindWeb, "web:", host, hostPort, parsed), nil
}

// ParentProfile returns the normalized author profile of a post whose author
// is known from its URL (ParentKey is set). It reports false otherwise.
func ParentProfile(link NormalizedLink) (NormalizedLink, bool) {
	if link.ParentKey == "" || link.Handle == "" {
		return NormalizedLink{}, false
	}
	var profileURL string
	switch link.Platform {
	case platformX:
		profileURL = "https://x.com/" + link.Handle
	case platformInstagram:
		profileURL = "https://instagram.com/" + link.Handle
	case platformTikTok:
		profileURL = "https://tiktok.com/@" + link.Handle
	case platformThreads:
		profileURL = "https://threads.net/@" + link.Handle
	case platformBluesky:
		profileURL = "https://bsky.app/profile/" + link.Handle
	default:
		return NormalizedLink{}, false
	}
	parent, err := NormalizeLink(profileURL)
	if err != nil || parent.Key != link.ParentKey || parent.Kind != LinkKindProfile {
		return NormalizedLink{}, false
	}
	return parent, true
}

// trackingParams are query parameters that identify a share or campaign, not
// the page itself. Keys are compared lowercased; any "utm_" key is also
// dropped.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "gbraid": true, "wbraid": true,
	"msclkid": true, "yclid": true, "mc_cid": true, "mc_eid": true,
	"_hsenc": true, "_hsmi": true, "mkt_tok": true,
	"igshid": true, "igsh": true, "ref_src": true, "ref_url": true,
}

var shortenerHosts = map[string]bool{
	"rb.gy": true, "bit.ly": true, "t.co": true, "tinyurl.com": true,
	"ow.ly": true, "buff.ly": true, "lnkd.in": true, "goo.gl": true,
	"is.gd": true, "cutt.ly": true, "shorturl.at": true, "rebrand.ly": true,
	"tiny.cc": true, "dub.sh": true, "geni.us": true, "amzn.to": true,
	"apple.co": true, "spoti.fi": true, "vm.tiktok.com": true, "vt.tiktok.com": true,
}

const (
	platformX         = "x"
	platformInstagram = "instagram"
	platformTikTok    = "tiktok"
	platformLinkedIn  = "linkedin"
	platformYouTube   = "youtube"
	platformThreads   = "threads"
	platformBluesky   = "bluesky"
)

var platformCanonicalHost = map[string]string{
	platformX:         "x.com",
	platformInstagram: "instagram.com",
	platformTikTok:    "tiktok.com",
	platformLinkedIn:  "linkedin.com",
	platformYouTube:   "youtube.com",
	platformThreads:   "threads.net",
	platformBluesky:   "bsky.app",
}

var socialHosts = map[string]string{
	"x.com": platformX, "mobile.x.com": platformX,
	"twitter.com": platformX, "mobile.twitter.com": platformX, "m.twitter.com": platformX,
	"instagram.com": platformInstagram, "m.instagram.com": platformInstagram,
	"tiktok.com": platformTikTok, "m.tiktok.com": platformTikTok,
	"linkedin.com": platformLinkedIn, "m.linkedin.com": platformLinkedIn,
	"youtube.com": platformYouTube, "m.youtube.com": platformYouTube, "youtu.be": platformYouTube,
	"threads.net": platformThreads, "threads.com": platformThreads,
	"bsky.app": platformBluesky,
}

func socialPlatformForHost(host string) (string, bool) {
	if platform, ok := socialHosts[host]; ok {
		return platform, true
	}
	// Country subdomains such as uk.linkedin.com.
	if strings.HasSuffix(host, ".linkedin.com") {
		return platformLinkedIn, true
	}
	return "", false
}

func normalizeSocial(platform, host string, seg []string, query url.Values) (NormalizedLink, bool) {
	var link NormalizedLink
	var ok bool
	switch platform {
	case platformX:
		link, ok = normalizeX(seg)
	case platformInstagram:
		link, ok = normalizeInstagram(seg)
	case platformTikTok:
		link, ok = normalizeTikTok(seg)
	case platformLinkedIn:
		link, ok = normalizeLinkedIn(seg)
	case platformYouTube:
		link, ok = normalizeYouTube(host, seg, query)
	case platformThreads:
		link, ok = normalizeThreads(seg)
	case platformBluesky:
		link, ok = normalizeBluesky(seg)
	}
	if ok {
		link.Platform = platform
		link.Domain = platformCanonicalHost[platform]
	}
	return link, ok
}

var (
	xHandleRe         = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	instagramHandleRe = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)
	tiktokHandleRe    = regexp.MustCompile(`^@[A-Za-z0-9._]{1,24}$`)
	threadsHandleRe   = regexp.MustCompile(`^@[A-Za-z0-9._]{1,30}$`)
	youtubeHandleRe   = regexp.MustCompile(`^@[A-Za-z0-9._-]{3,30}$`)
	youtubeVideoIDRe  = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	digitsRe          = regexp.MustCompile(`^[0-9]+$`)
	// Shortcodes and record keys are case-sensitive; keep them as-is.
	opaqueIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

var xReservedPaths = toSet(
	"home", "explore", "search", "i", "intent", "hashtag", "settings", "messages",
	"notifications", "share", "compose", "login", "logout", "signup", "tos",
	"privacy", "about", "download", "jobs",
)

var xProfileTabs = toSet(
	"with_replies", "media", "likes", "highlights", "articles",
	"followers", "following", "verified_followers",
)

func normalizeX(seg []string) (NormalizedLink, bool) {
	// x.com/i/web/status/<id> and x.com/i/status/<id> have no author.
	if len(seg) >= 4 && seg[0] == "i" && seg[1] == "web" && seg[2] == "status" && digitsRe.MatchString(seg[3]) {
		return xPost("", seg[3]), true
	}
	if len(seg) >= 3 && seg[0] == "i" && seg[1] == "status" && digitsRe.MatchString(seg[2]) {
		return xPost("", seg[2]), true
	}
	if len(seg) == 0 || !xHandleRe.MatchString(seg[0]) || xReservedPaths[strings.ToLower(seg[0])] {
		return NormalizedLink{}, false
	}
	handle := strings.ToLower(seg[0])
	if len(seg) == 1 || (len(seg) == 2 && xProfileTabs[strings.ToLower(seg[1])]) {
		return profile("x:"+handle, "https://x.com/"+handle, handle), true
	}
	if len(seg) >= 3 && (seg[1] == "status" || seg[1] == "statuses") && digitsRe.MatchString(seg[2]) {
		return xPost(handle, seg[2]), true
	}
	return NormalizedLink{}, false
}

func xPost(handle, id string) NormalizedLink {
	link := NormalizedLink{Key: "x:status:" + id, Kind: LinkKindPost}
	if handle == "" {
		link.CanonicalURL = "https://x.com/i/status/" + id
		return link
	}
	link.CanonicalURL = "https://x.com/" + handle + "/status/" + id
	link.Handle = handle
	link.ParentKey = "x:" + handle
	return link
}

var instagramReservedPaths = toSet(
	"p", "reel", "reels", "tv", "explore", "stories", "accounts", "direct",
	"about", "legal", "developer", "web",
)

var instagramPostPrefixes = toSet("p", "reel", "reels", "tv")

func normalizeInstagram(seg []string) (NormalizedLink, bool) {
	if len(seg) >= 2 && instagramPostPrefixes[seg[0]] && opaqueIDRe.MatchString(seg[1]) {
		return post("instagram:post:"+seg[1], "https://instagram.com/p/"+seg[1], "", ""), true
	}
	if len(seg) == 0 || !instagramHandleRe.MatchString(seg[0]) || instagramReservedPaths[strings.ToLower(seg[0])] {
		return NormalizedLink{}, false
	}
	handle := strings.ToLower(seg[0])
	if len(seg) == 1 {
		return profile("instagram:"+handle, "https://instagram.com/"+handle, handle), true
	}
	// Newer share format: instagram.com/<handle>/p/<code>.
	if len(seg) >= 3 && instagramPostPrefixes[seg[1]] && opaqueIDRe.MatchString(seg[2]) {
		return post("instagram:post:"+seg[2], "https://instagram.com/p/"+seg[2], handle, "instagram:"+handle), true
	}
	return NormalizedLink{}, false
}

func normalizeTikTok(seg []string) (NormalizedLink, bool) {
	if len(seg) == 0 || !tiktokHandleRe.MatchString(seg[0]) {
		return NormalizedLink{}, false
	}
	handle := strings.ToLower(strings.TrimPrefix(seg[0], "@"))
	parent := "tiktok:" + handle
	if len(seg) == 1 {
		return profile(parent, "https://tiktok.com/@"+handle, handle), true
	}
	if len(seg) >= 3 && (seg[1] == "video" || seg[1] == "photo") && digitsRe.MatchString(seg[2]) {
		return post("tiktok:video:"+seg[2], "https://tiktok.com/@"+handle+"/video/"+seg[2], handle, parent), true
	}
	return NormalizedLink{}, false
}

var linkedInProfileTypes = toSet("in", "company", "school", "showcase")

func normalizeLinkedIn(seg []string) (NormalizedLink, bool) {
	if len(seg) >= 2 && linkedInProfileTypes[strings.ToLower(seg[0])] {
		kind := strings.ToLower(seg[0])
		slug := strings.ToLower(seg[1])
		return profile("linkedin:"+kind+"/"+slug, "https://linkedin.com/"+kind+"/"+url.PathEscape(slug), slug), true
	}
	if len(seg) >= 2 && seg[0] == "posts" {
		return post("linkedin:post:"+seg[1], "https://linkedin.com/posts/"+url.PathEscape(seg[1]), "", ""), true
	}
	if len(seg) >= 3 && seg[0] == "feed" && seg[1] == "update" {
		return post("linkedin:post:"+seg[2], "https://linkedin.com/feed/update/"+url.PathEscape(seg[2]), "", ""), true
	}
	return NormalizedLink{}, false
}

func normalizeYouTube(host string, seg []string, query url.Values) (NormalizedLink, bool) {
	if host == "youtu.be" {
		if len(seg) >= 1 && youtubeVideoIDRe.MatchString(seg[0]) {
			return youtubeVideo(seg[0]), true
		}
		return NormalizedLink{}, false
	}
	if len(seg) == 1 && seg[0] == "watch" {
		if id := query.Get("v"); youtubeVideoIDRe.MatchString(id) {
			return youtubeVideo(id), true
		}
		return NormalizedLink{}, false
	}
	if len(seg) >= 2 && (seg[0] == "shorts" || seg[0] == "live" || seg[0] == "embed") && youtubeVideoIDRe.MatchString(seg[1]) {
		return youtubeVideo(seg[1]), true
	}
	if len(seg) >= 1 && youtubeHandleRe.MatchString(seg[0]) {
		handle := strings.ToLower(seg[0])
		return profile("youtube:"+handle, "https://youtube.com/"+handle, strings.TrimPrefix(handle, "@")), true
	}
	if len(seg) >= 2 && seg[0] == "channel" && opaqueIDRe.MatchString(seg[1]) {
		return profile("youtube:channel:"+seg[1], "https://youtube.com/channel/"+seg[1], ""), true
	}
	if len(seg) >= 2 && (seg[0] == "c" || seg[0] == "user") && opaqueIDRe.MatchString(seg[1]) {
		name := strings.ToLower(seg[1])
		return profile("youtube:"+seg[0]+":"+name, "https://youtube.com/"+seg[0]+"/"+name, name), true
	}
	return NormalizedLink{}, false
}

func youtubeVideo(id string) NormalizedLink {
	return post("youtube:video:"+id, "https://youtube.com/watch?v="+id, "", "")
}

func normalizeThreads(seg []string) (NormalizedLink, bool) {
	if len(seg) == 0 || !threadsHandleRe.MatchString(seg[0]) {
		return NormalizedLink{}, false
	}
	handle := strings.ToLower(strings.TrimPrefix(seg[0], "@"))
	parent := "threads:" + handle
	if len(seg) == 1 {
		return profile(parent, "https://threads.net/@"+handle, handle), true
	}
	if len(seg) >= 3 && seg[1] == "post" && opaqueIDRe.MatchString(seg[2]) {
		return post("threads:post:"+seg[2], "https://threads.net/@"+handle+"/post/"+seg[2], handle, parent), true
	}
	return NormalizedLink{}, false
}

func normalizeBluesky(seg []string) (NormalizedLink, bool) {
	if len(seg) < 2 || seg[0] != "profile" || seg[1] == "" {
		return NormalizedLink{}, false
	}
	handle := strings.ToLower(seg[1])
	parent := "bluesky:" + handle
	if len(seg) == 2 {
		return profile(parent, "https://bsky.app/profile/"+handle, handle), true
	}
	// Post record keys are only unique per account, so the key includes it.
	if len(seg) >= 4 && seg[2] == "post" && opaqueIDRe.MatchString(seg[3]) {
		return post("bluesky:post:"+handle+"/"+seg[3], "https://bsky.app/profile/"+handle+"/post/"+seg[3], handle, parent), true
	}
	return NormalizedLink{}, false
}

func profile(key, canonicalURL, handle string) NormalizedLink {
	return NormalizedLink{Key: key, CanonicalURL: canonicalURL, Kind: LinkKindProfile, Handle: handle}
}

func post(key, canonicalURL, handle, parentKey string) NormalizedLink {
	return NormalizedLink{Key: key, CanonicalURL: canonicalURL, Kind: LinkKindPost, Handle: handle, ParentKey: parentKey}
}

// webLink builds a Key and CanonicalURL from the host, the path with trailing
// slashes removed, and the query without tracking params (sorted by key). The
// path keeps its case because web paths are case-sensitive.
func webLink(platform string, kind LinkKind, keyPrefix, domain, hostPort string, parsed *url.URL) NormalizedLink {
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	target := hostPort + path
	if query := cleanQuery(parsed.Query()); query != "" {
		target += "?" + query
	}
	return NormalizedLink{
		Key:          keyPrefix + target,
		CanonicalURL: "https://" + target,
		Kind:         kind,
		Platform:     platform,
		Domain:       domain,
	}
}

func cleanQuery(values url.Values) string {
	for key := range values {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || trackingParams[lower] {
			values.Del(key)
		}
	}
	// Encode sorts by key, which keeps Keys stable regardless of param order.
	return values.Encode()
}

func pathSegments(path string) []string {
	parts := strings.Split(path, "/")
	segments := parts[:0]
	for _, part := range parts {
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func toSet(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

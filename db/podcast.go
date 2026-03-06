package db

import (
	"time"
)

// Podcast is
type Podcast struct {
	Base
	Title string

	Summary     string `gorm:"type:text"`
	SummaryHTML string `gorm:"type:text"`

	Author string

	Image string

	URL string

	FeedMetadata string `gorm:"type:text" json:"-"`

	LastEpisode *time.Time

	PodcastItems []PodcastItem

	Tags []*Tag `gorm:"many2many:podcast_tags;"`

	DownloadedEpisodesCount  int `gorm:"-"`
	DownloadingEpisodesCount int `gorm:"-"`
	AllEpisodesCount         int `gorm:"-"`

	DownloadedEpisodesSize  int64 `gorm:"-"`
	DownloadingEpisodesSize int64 `gorm:"-"`
	AllEpisodesSize         int64 `gorm:"-"`

	IsPaused bool `gorm:"default:false"`

	RetentionKeepAll bool `gorm:"default:false"`

	AutoSkipSponsorChapters bool `gorm:"default:false"`
}

// PodcastItem is
type PodcastItem struct {
	Base
	PodcastID   string
	Podcast     Podcast
	Title       string
	Summary     string `gorm:"type:text"`
	SummaryHTML string `gorm:"type:text"`

	EpisodeType string

	Duration int

	PubDate time.Time

	FileURL string

	GUID  string
	Image string

	ChaptersURL     string
	ChaptersType    string
	ChaptersJSON    string `gorm:"type:text" json:"-"`
	ID3TagsJSON     string `gorm:"type:text" json:"-"`
	ID3ChaptersJSON string `gorm:"type:text" json:"-"`

	DownloadDate   time.Time
	DownloadPath   string
	DownloadStatus DownloadStatus `gorm:"default:0"`

	IsPlayed bool `gorm:"default:false"`

	BookmarkDate time.Time

	LocalImage string

	FileSize int64

	DownloadedBytes    int64
	DownloadTotalBytes int64

	HasChapters   bool `gorm:"-"`
	HasTranscript bool `gorm:"-"`

	ItemMetadata               string `gorm:"type:text" json:"-"`
	TranscriptJSON             string `gorm:"type:text" json:"-"`
	CanonicalTranscript        string `gorm:"type:text" json:"-"`
	CanonicalTranscriptVersion int
	CanonicalUpdatedAt         *time.Time
	TranscriptStatus           string `gorm:"type:text"`
	// Progress fields are persisted so long-running transcriptions can resume after restarts.
	TranscriptProgressPct    int    `gorm:"default:0"`
	TranscriptProgressStage  string `gorm:"type:text"`
	TranscriptCheckpointJSON string `gorm:"type:text" json:"-"`
	// WhisperX retry bookkeeping to avoid tight failure loops and enable delayed retries.
	TranscriptRetryCount  int `gorm:"default:0"`
	TranscriptNextAttempt *time.Time
	TranscriptLastError   string `gorm:"type:text" json:"-"`
	// Lineage: which WhisperX model produced the transcript.
	TranscriptModel string `gorm:"type:text"`

	// LLM-generated summary fields (populated after successful transcription).
	LLMSummary       string     `gorm:"type:text" json:"-"`
	LLMSummaryStatus string     `gorm:"type:text"`
	LLMSummaryError  string     `gorm:"type:text" json:"-"`
	LLMSummaryDate   *time.Time
	// Lineage: which LLM model and prompt produced the summary.
	LLMSummaryModel  string `gorm:"type:text"`
	LLMSummaryPrompt string `gorm:"type:text" json:"-"`

	HasSummary bool `gorm:"-"`
}

// DownloadStatus represents a public type.
type DownloadStatus int

const (
	// NotDownloaded is a public constant.
	NotDownloaded DownloadStatus = iota
	// Downloading is a public constant.
	Downloading
	// Downloaded is a public constant.
	Downloaded
	// Deleted is a public constant.
	Deleted
	// Paused is a public constant.
	Paused
)

// Setting represents a public type.
type Setting struct {
	Base
	DownloadOnAdd                 bool `gorm:"default:true"`
	InitialDownloadCount          int  `gorm:"default:5"`
	AutoDownload                  bool `gorm:"default:true"`
	AppendDateToFileName          bool `gorm:"default:false"`
	AppendEpisodeNumberToFileName bool `gorm:"default:false"`
	DarkMode                      bool `gorm:"default:false"`
	DownloadEpisodeImages         bool `gorm:"default:false"`
	GenerateNFOFile               bool `gorm:"default:false"`
	DontDownloadDeletedFromDisk   bool `gorm:"default:false"`
	BaseURL                       string
	MaxDownloadConcurrency        int `gorm:"default:5"`
	UserAgent                     string

	RetentionKeepAll          bool `gorm:"default:true"`
	RetentionKeepLatest       int  `gorm:"default:0"`
	RetentionDeleteAfterDays  int  `gorm:"default:0"`
	RetentionDeleteOnlyPlayed bool `gorm:"default:true"`

	// LLM summarization settings (user-configurable from the UI).
	SummarizationEnabled bool   `gorm:"default:false"`
	SummarizationPrompt  string `gorm:"type:text"`
}

// Migration represents a public type.
type Migration struct {
	Base
	Date time.Time
	Name string
}

// JobLock represents a public type.
type JobLock struct {
	Base
	Date     time.Time
	Name     string
	Duration int
}

// Tag represents a public type.
type Tag struct {
	Base
	Label       string
	Description string     `gorm:"type:text"`
	Podcasts    []*Podcast `gorm:"many2many:podcast_tags;"`
}

// IsLocked handles the corresponding operation.
func (lock *JobLock) IsLocked() bool {
	return lock.IsLockedAt(time.Now().UTC())
}

// IsLockedAt handles the corresponding operation.
func (lock *JobLock) IsLockedAt(now time.Time) bool {
	if lock == nil {
		return false
	}
	if lock.Duration <= 0 || lock.Date.IsZero() {
		return false
	}
	return lock.Date.UTC().Add(time.Duration(lock.Duration) * time.Minute).After(now.UTC())
}

// PodcastItemStatsModel represents a public type.
type PodcastItemStatsModel struct {
	PodcastID      string
	DownloadStatus DownloadStatus
	Count          int
	Size           int64
}

// PodcastItemDiskStatsModel represents a public type.
type PodcastItemDiskStatsModel struct {
	DownloadStatus DownloadStatus
	Count          int
	Size           int64
}

// PodcastItemConsolidateDiskStatsModel represents a public type.
type PodcastItemConsolidateDiskStatsModel struct {
	Downloaded      int64
	Downloading     int64
	NotDownloaded   int64
	Deleted         int64
	PendingDownload int64
}

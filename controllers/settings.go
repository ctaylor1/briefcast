package controllers

import (
	"net/http"

	"github.com/ctaylor1/briefcast/db"
	"github.com/ctaylor1/briefcast/service"
	"github.com/gin-gonic/gin"
)

// SettingsResponse is the full settings payload returned by GET /settings.
type SettingsResponse struct {
	// Retention
	KeepAllEpisodes    bool `json:"keepAllEpisodes"`
	KeepLatestEpisodes int  `json:"keepLatestEpisodes"`
	DeleteAfterDays    int  `json:"deleteAfterDays"`
	DeleteOnlyPlayed   bool `json:"deleteOnlyPlayed"`
	// Initial back catalog download policy
	DownloadOnAdd         bool   `json:"downloadOnAdd"`
	InitialDownloadCount  int    `json:"initialDownloadCount"`
	InitialDownloadMode   string `json:"initialDownloadMode"`
	InitialDownloadMonths int    `json:"initialDownloadMonths"`
	AutoDownload          bool   `json:"autoDownload"`
	// Summarization
	SummarizationEnabled    bool   `json:"summarizationEnabled"`
	SummarizationModel      string `json:"summarizationModel"`
	SummarizationPrompt     string `json:"summarizationPrompt"`
	SummarizationUserPrompt string `json:"summarizationUserPrompt"`
	// Model settings
	LLMConcurrency int `json:"llmConcurrency"`
	// Read-only defaults so the UI can show placeholders.
	DefaultModel        string `json:"defaultModel"`
	DefaultSystemPrompt string `json:"defaultSystemPrompt"`
	DefaultUserPrompt   string `json:"defaultUserPrompt"`
	// Appearance
	ThemeMode      string `json:"themeMode"`
	Timezone       string `json:"timezone"`
	LightStartHour int    `json:"lightStartHour"`
	DarkStartHour  int    `json:"darkStartHour"`
	// Briefpoint
	BriefpointEnabled   bool   `json:"briefpointEnabled"`
	BriefpointServerURL string `json:"briefpointServerURL"`
	BriefpointAPIKey    string `json:"briefpointAPIKey"`
}

// SettingsPatch is the partial update payload accepted by PATCH /settings.
type SettingsPatch struct {
	// Retention
	KeepAllEpisodes    *bool `json:"keepAllEpisodes"`
	KeepLatestEpisodes *int  `json:"keepLatestEpisodes"`
	DeleteAfterDays    *int  `json:"deleteAfterDays"`
	DeleteOnlyPlayed   *bool `json:"deleteOnlyPlayed"`
	// Initial back catalog download policy
	DownloadOnAdd         *bool   `json:"downloadOnAdd"`
	InitialDownloadCount  *int    `json:"initialDownloadCount"`
	InitialDownloadMode   *string `json:"initialDownloadMode"`
	InitialDownloadMonths *int    `json:"initialDownloadMonths"`
	AutoDownload          *bool   `json:"autoDownload"`
	// Summarization
	SummarizationEnabled    *bool   `json:"summarizationEnabled"`
	SummarizationModel      *string `json:"summarizationModel"`
	SummarizationPrompt     *string `json:"summarizationPrompt"`
	SummarizationUserPrompt *string `json:"summarizationUserPrompt"`
	// Model settings
	LLMConcurrency *int `json:"llmConcurrency"`
	// Appearance
	ThemeMode      *string `json:"themeMode"`
	Timezone       *string `json:"timezone"`
	LightStartHour *int    `json:"lightStartHour"`
	DarkStartHour  *int    `json:"darkStartHour"`
	// Briefpoint
	BriefpointEnabled   *bool   `json:"briefpointEnabled"`
	BriefpointServerURL *string `json:"briefpointServerURL"`
	BriefpointAPIKey    *string `json:"briefpointAPIKey"`
}

// GetSettings handles the corresponding operation.
func GetSettings(c *gin.Context) {
	setting := db.GetOrCreateSetting()
	c.JSON(http.StatusOK, settingsFromModel(setting))
}

// PatchSettings handles the corresponding operation.
func PatchSettings(c *gin.Context) {
	var patch SettingsPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	setting := db.GetOrCreateSetting()
	if patch.KeepAllEpisodes != nil {
		setting.RetentionKeepAll = *patch.KeepAllEpisodes
	}
	if patch.KeepLatestEpisodes != nil {
		if *patch.KeepLatestEpisodes < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "keepLatestEpisodes must be 0 or greater"})
			return
		}
		setting.RetentionKeepLatest = *patch.KeepLatestEpisodes
	}
	if patch.DeleteAfterDays != nil {
		if *patch.DeleteAfterDays < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deleteAfterDays must be 0 or greater"})
			return
		}
		setting.RetentionDeleteAfterDays = *patch.DeleteAfterDays
	}
	if patch.DeleteOnlyPlayed != nil {
		setting.RetentionDeleteOnlyPlayed = *patch.DeleteOnlyPlayed
	}
	if patch.DownloadOnAdd != nil {
		setting.DownloadOnAdd = *patch.DownloadOnAdd
	}
	if patch.InitialDownloadCount != nil {
		if *patch.InitialDownloadCount < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "initialDownloadCount must be 0 or greater"})
			return
		}
		setting.InitialDownloadCount = *patch.InitialDownloadCount
	}
	if patch.InitialDownloadMode != nil {
		mode := service.NormalizeInitialDownloadMode(*patch.InitialDownloadMode)
		if mode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "initialDownloadMode must be count, months, or all"})
			return
		}
		setting.InitialDownloadMode = mode
	}
	if patch.InitialDownloadMonths != nil {
		if *patch.InitialDownloadMonths < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "initialDownloadMonths must be 0 or greater"})
			return
		}
		setting.InitialDownloadMonths = *patch.InitialDownloadMonths
	}
	if patch.AutoDownload != nil {
		setting.AutoDownload = *patch.AutoDownload
	}
	if patch.SummarizationEnabled != nil {
		setting.SummarizationEnabled = *patch.SummarizationEnabled
	}
	if patch.SummarizationModel != nil {
		setting.SummarizationModel = *patch.SummarizationModel
	}
	if patch.SummarizationPrompt != nil {
		setting.SummarizationPrompt = *patch.SummarizationPrompt
	}
	if patch.SummarizationUserPrompt != nil {
		setting.SummarizationUserPrompt = *patch.SummarizationUserPrompt
	}
	if patch.LLMConcurrency != nil {
		v := *patch.LLMConcurrency
		if v < 1 || v > 10 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "llmConcurrency must be between 1 and 10"})
			return
		}
		setting.LLMConcurrency = v
	}
	if patch.ThemeMode != nil {
		setting.ThemeMode = *patch.ThemeMode
	}
	if patch.Timezone != nil {
		setting.Timezone = *patch.Timezone
	}
	if patch.LightStartHour != nil {
		setting.LightStartHour = *patch.LightStartHour
	}
	if patch.DarkStartHour != nil {
		setting.DarkStartHour = *patch.DarkStartHour
	}
	if patch.BriefpointEnabled != nil {
		setting.BriefpointEnabled = *patch.BriefpointEnabled
	}
	if patch.BriefpointServerURL != nil {
		setting.BriefpointServerURL = *patch.BriefpointServerURL
	}
	if patch.BriefpointAPIKey != nil {
		setting.BriefpointAPIKey = *patch.BriefpointAPIKey
	}

	if err := db.UpdateSettings(setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settingsFromModel(setting))
}

// BackfillSummaries kicks off background summarization of all episodes that
// have a transcript but no summary yet. Returns 409 if already running.
func BackfillSummaries(c *gin.Context) {
	if service.GetSummaryBackfillRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "backfill is already running"})
		return
	}

	go func() {
		service.BackfillSummaries(nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "summary backfill started"})
}

// GetBackfillSummariesStatus returns whether a backfill is currently running.
func GetBackfillSummariesStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"running": service.GetSummaryBackfillRunning()})
}

// GetSummaryModels returns the distinct LLM models that have been used to generate summaries.
func GetSummaryModels(c *gin.Context) {
	models, err := db.GetDistinctSummaryModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load summary models"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": models})
}

// ResummarizeSummaries regenerates summaries for episodes matching filter criteria.
func ResummarizeSummaries(c *gin.Context) {
	if service.GetSummaryBackfillRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "a summary job is already running"})
		return
	}

	var filter service.ResummarizeFilter
	if err := c.ShouldBindJSON(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if filter.DryRun {
		result, err := service.ResummarizeSummaries(filter, nil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}

	go func() {
		service.ResummarizeSummaries(filter, nil)
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "re-summarize started"})
}

// BackfillLinks kicks off background extraction of show note links for all
// episodes that have show notes but no extracted links yet.
func BackfillLinks(c *gin.Context) {
	if service.GetLinkBackfillRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "link backfill is already running"})
		return
	}

	go func() {
		service.BackfillShowNoteLinks()
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "link backfill started"})
}

// GetBackfillLinksStatus returns whether a link backfill is currently running.
func GetBackfillLinksStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"running": service.GetLinkBackfillRunning()})
}

// ExportAll exports all existing transcripts and summaries to text and markdown files.
func ExportAll(c *gin.Context) {
	if service.GetExportAllRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "export is already running"})
		return
	}

	go func() {
		service.ExportAll()
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "export started"})
}

// GetExportAllStatus returns whether an export is currently running.
func GetExportAllStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"running": service.GetExportAllRunning()})
}

func settingsFromModel(setting *db.Setting) SettingsResponse {
	cfg := service.LoadLLMConfig()
	return SettingsResponse{
		KeepAllEpisodes:         setting.RetentionKeepAll,
		KeepLatestEpisodes:      setting.RetentionKeepLatest,
		DeleteAfterDays:         setting.RetentionDeleteAfterDays,
		DeleteOnlyPlayed:        setting.RetentionDeleteOnlyPlayed,
		DownloadOnAdd:           setting.DownloadOnAdd,
		InitialDownloadCount:    setting.InitialDownloadCount,
		InitialDownloadMode:     service.NormalizeInitialDownloadModeWithDefault(setting.InitialDownloadMode),
		InitialDownloadMonths:   setting.InitialDownloadMonths,
		AutoDownload:            setting.AutoDownload,
		SummarizationEnabled:    setting.SummarizationEnabled,
		SummarizationModel:      setting.SummarizationModel,
		SummarizationPrompt:     setting.SummarizationPrompt,
		SummarizationUserPrompt: setting.SummarizationUserPrompt,
		LLMConcurrency:          setting.LLMConcurrency,
		DefaultModel:            cfg.Model,
		DefaultSystemPrompt:     cfg.DefaultPrompt,
		DefaultUserPrompt:       cfg.DefaultUserPrompt,
		ThemeMode:               setting.ThemeMode,
		Timezone:                setting.Timezone,
		LightStartHour:          setting.LightStartHour,
		DarkStartHour:           setting.DarkStartHour,
		BriefpointEnabled:       setting.BriefpointEnabled,
		BriefpointServerURL:     setting.BriefpointServerURL,
		BriefpointAPIKey:        setting.BriefpointAPIKey,
	}
}

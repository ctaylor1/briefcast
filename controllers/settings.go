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
	// Read-only defaults so the UI can show placeholders.
	DefaultModel        string `json:"defaultModel"`
	DefaultSystemPrompt string `json:"defaultSystemPrompt"`
	DefaultUserPrompt   string `json:"defaultUserPrompt"`
	// Appearance
	ThemeMode      string `json:"themeMode"`
	Timezone       string `json:"timezone"`
	LightStartHour int    `json:"lightStartHour"`
	DarkStartHour  int    `json:"darkStartHour"`
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
	// Appearance
	ThemeMode      *string `json:"themeMode"`
	Timezone       *string `json:"timezone"`
	LightStartHour *int    `json:"lightStartHour"`
	DarkStartHour  *int    `json:"darkStartHour"`
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
		DefaultModel:            cfg.Model,
		DefaultSystemPrompt:     cfg.DefaultPrompt,
		DefaultUserPrompt:       cfg.DefaultUserPrompt,
		ThemeMode:               setting.ThemeMode,
		Timezone:                setting.Timezone,
		LightStartHour:          setting.LightStartHour,
		DarkStartHour:           setting.DarkStartHour,
	}
}

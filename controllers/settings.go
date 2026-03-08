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
	// Summarization
	SummarizationEnabled    bool   `json:"summarizationEnabled"`
	SummarizationPrompt     string `json:"summarizationPrompt"`
	SummarizationUserPrompt string `json:"summarizationUserPrompt"`
	// Read-only defaults so the UI can show placeholders.
	DefaultSystemPrompt string `json:"defaultSystemPrompt"`
	DefaultUserPrompt   string `json:"defaultUserPrompt"`
}

// SettingsPatch is the partial update payload accepted by PATCH /settings.
type SettingsPatch struct {
	// Retention
	KeepAllEpisodes    *bool `json:"keepAllEpisodes"`
	KeepLatestEpisodes *int  `json:"keepLatestEpisodes"`
	DeleteAfterDays    *int  `json:"deleteAfterDays"`
	DeleteOnlyPlayed   *bool `json:"deleteOnlyPlayed"`
	// Summarization
	SummarizationEnabled    *bool   `json:"summarizationEnabled"`
	SummarizationPrompt     *string `json:"summarizationPrompt"`
	SummarizationUserPrompt *string `json:"summarizationUserPrompt"`
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
	if patch.SummarizationEnabled != nil {
		setting.SummarizationEnabled = *patch.SummarizationEnabled
	}
	if patch.SummarizationPrompt != nil {
		setting.SummarizationPrompt = *patch.SummarizationPrompt
	}
	if patch.SummarizationUserPrompt != nil {
		setting.SummarizationUserPrompt = *patch.SummarizationUserPrompt
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

func settingsFromModel(setting *db.Setting) SettingsResponse {
	cfg := service.LoadLLMConfig()
	return SettingsResponse{
		KeepAllEpisodes:         setting.RetentionKeepAll,
		KeepLatestEpisodes:      setting.RetentionKeepLatest,
		DeleteAfterDays:         setting.RetentionDeleteAfterDays,
		DeleteOnlyPlayed:        setting.RetentionDeleteOnlyPlayed,
		SummarizationEnabled:    setting.SummarizationEnabled,
		SummarizationPrompt:     setting.SummarizationPrompt,
		SummarizationUserPrompt: setting.SummarizationUserPrompt,
		DefaultSystemPrompt:     cfg.DefaultPrompt,
		DefaultUserPrompt:       cfg.DefaultUserPrompt,
	}
}

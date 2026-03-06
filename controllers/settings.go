package controllers

import (
	"net/http"

	"github.com/ctaylor1/briefcast/db"
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
	SummarizationEnabled bool   `json:"summarizationEnabled"`
	SummarizationPrompt  string `json:"summarizationPrompt"`
}

// SettingsPatch is the partial update payload accepted by PATCH /settings.
type SettingsPatch struct {
	// Retention
	KeepAllEpisodes    *bool `json:"keepAllEpisodes"`
	KeepLatestEpisodes *int  `json:"keepLatestEpisodes"`
	DeleteAfterDays    *int  `json:"deleteAfterDays"`
	DeleteOnlyPlayed   *bool `json:"deleteOnlyPlayed"`
	// Summarization
	SummarizationEnabled *bool   `json:"summarizationEnabled"`
	SummarizationPrompt  *string `json:"summarizationPrompt"`
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

	if err := db.UpdateSettings(setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settingsFromModel(setting))
}

func settingsFromModel(setting *db.Setting) SettingsResponse {
	return SettingsResponse{
		KeepAllEpisodes:      setting.RetentionKeepAll,
		KeepLatestEpisodes:   setting.RetentionKeepLatest,
		DeleteAfterDays:      setting.RetentionDeleteAfterDays,
		DeleteOnlyPlayed:     setting.RetentionDeleteOnlyPlayed,
		SummarizationEnabled: setting.SummarizationEnabled,
		SummarizationPrompt:  setting.SummarizationPrompt,
	}
}

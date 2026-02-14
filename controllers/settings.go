package controllers

import (
	"net/http"

	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

type RetentionSettingsResponse struct {
	KeepAllEpisodes    bool `json:"keepAllEpisodes"`
	KeepLatestEpisodes int  `json:"keepLatestEpisodes"`
	DeleteAfterDays    int  `json:"deleteAfterDays"`
	DeleteOnlyPlayed   bool `json:"deleteOnlyPlayed"`
}

type RetentionSettingsPatch struct {
	KeepAllEpisodes    *bool `json:"keepAllEpisodes"`
	KeepLatestEpisodes *int  `json:"keepLatestEpisodes"`
	DeleteAfterDays    *int  `json:"deleteAfterDays"`
	DeleteOnlyPlayed   *bool `json:"deleteOnlyPlayed"`
}

func GetSettings(c *gin.Context) {
	setting := db.GetOrCreateSetting()
	c.JSON(http.StatusOK, retentionSettingsFromSetting(setting))
}

func PatchSettings(c *gin.Context) {
	var patch RetentionSettingsPatch
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

	if err := db.UpdateSettings(setting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, retentionSettingsFromSetting(setting))
}

func retentionSettingsFromSetting(setting *db.Setting) RetentionSettingsResponse {
	return RetentionSettingsResponse{
		KeepAllEpisodes:    setting.RetentionKeepAll,
		KeepLatestEpisodes: setting.RetentionKeepLatest,
		DeleteAfterDays:    setting.RetentionDeleteAfterDays,
		DeleteOnlyPlayed:   setting.RetentionDeleteOnlyPlayed,
	}
}

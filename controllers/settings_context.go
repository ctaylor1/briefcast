package controllers

import (
	"github.com/ctaylor1/briefcast/db"
	"github.com/gin-gonic/gin"
)

// settingFromContext returns the settings a middleware stored on the request,
// or reads them now. Only the few handlers that need settings pay for the read.
func settingFromContext(c *gin.Context) *db.Setting {
	if value, ok := c.Get("setting"); ok {
		if setting, ok := value.(*db.Setting); ok && setting != nil {
			return setting
		}
	}
	setting := db.GetOrCreateSetting()
	c.Set("setting", setting)
	return setting
}

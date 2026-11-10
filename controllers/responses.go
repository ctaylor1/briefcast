package controllers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// respondLookupError answers a failed read or write of a record: 404 when it
// does not exist, 500 otherwise. The cause is logged, not sent to the client.
func respondLookupError(c *gin.Context, err error, what string, keysAndValues ...interface{}) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": what + " not found"})
		return
	}
	controllerLogger.Warnw("request failed", append([]interface{}{"what", what, "error", err}, keysAndValues...)...)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
}

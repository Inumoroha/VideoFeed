package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health 返回服务当前的健康状态。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListSkills(c *gin.Context) {
	if h.skillProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "skill plugin not configured"})
		return
	}

	skills, err := h.skillProvider.ListSkills()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"skills": skills})
}

func (h *Handler) GetSkill(c *gin.Context) {
	if h.skillProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "skill plugin not configured"})
		return
	}

	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "skill name is required"})
		return
	}

	skill, err := h.skillProvider.GetSkill(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if c.Query("include_content") != "true" {
		skill.Instructions = ""
	}

	c.JSON(http.StatusOK, skill)
}

func (h *Handler) EnableSkill(c *gin.Context) {
	if h.skillProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "skill plugin not configured"})
		return
	}

	name := c.Param("name")
	if err := h.skillProvider.SetSkillEnabled(name, true); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": name, "enabled": true})
}

func (h *Handler) DisableSkill(c *gin.Context) {
	if h.skillProvider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "skill plugin not configured"})
		return
	}

	name := c.Param("name")
	if err := h.skillProvider.SetSkillEnabled(name, false); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": name, "enabled": false})
}

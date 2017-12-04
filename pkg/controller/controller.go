package controller

import (
	"fmt"
	"net/http"

	"github.com/codefresh-io/cf-triggers/pkg/model"
	"github.com/gin-gonic/gin"
)

// Controller trigger controller
type Controller struct {
	svc model.TriggerService
}

// NewController new trigger controller
func NewController(svc model.TriggerService) *Controller {
	return &Controller{svc}
}

// List triggers
func (c *Controller) List(ctx *gin.Context) {
	var triggers []model.Trigger
	var err error
	if triggers, err = c.svc.List(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "message": "Failed to get list of triggers!"})
		return
	}
	if len(triggers) <= 0 {
		ctx.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": "No triggers found!"})
		return
	}
	ctx.JSON(http.StatusOK, triggers)
}

// Get trigger
func (c *Controller) Get(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	var trigger model.Trigger
	var err error
	if trigger, err = c.svc.Get(id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "message": "Failed to get trigger!"})
		return
	}
	if trigger.IsEmpty() {
		ctx.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "message": fmt.Sprintf("No trigger %s found!", id)})
		return
	}
	ctx.JSON(http.StatusOK, trigger)
}

// Add trigger
func (c *Controller) Add(ctx *gin.Context) {
	var trigger model.Trigger
	ctx.Bind(&trigger)

	if trigger.Event != "" && len(trigger.Pipelines) != 0 {
		// add trigger
		if err := c.svc.Add(trigger); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "message": "Failed to add trigger!"})
			return
		}
		// report OK
		ctx.Status(http.StatusOK)
	} else {
		// Display error
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"status": http.StatusUnprocessableEntity, "message": "Required fields are empty!"})
	}
}

// Update trigger
func (c *Controller) Update(ctx *gin.Context) {

}

// Delete trigger
func (c *Controller) Delete(ctx *gin.Context) {
	id := ctx.Params.ByName("id")
	if err := c.svc.Delete(id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "message": "Failed to delete trigger!"})
		return
	}
	ctx.Status(http.StatusOK)
}
package router

import (
	"github.com/gin-gonic/gin"
	events "github.com/hngprojects/telex_be/pkg/controller/events"
	"github.com/hngprojects/telex_be/pkg/middleware"
)

func Events(r *gin.Engine, options events.Options) {
	controller := events.New(options)
	group := r.Group("/api/v1/organisations/:org_id/events", middleware.Authorize(options.DB.Postgresql))
	group.GET("/capabilities", controller.Capabilities)
	group.GET("", controller.List)
	group.POST("", controller.Create)
	group.PUT("/featured", controller.Featured)
	group.GET("/:event_id", controller.Get)
	group.PATCH("/:event_id", controller.Save)
	group.POST("/:event_id/publish", controller.Publish)
	group.POST("/:event_id/cancel", controller.Cancel)
	group.PUT("/:event_id/rsvp", controller.Rsvp)
	group.POST("/:event_id/join", controller.Join)
}

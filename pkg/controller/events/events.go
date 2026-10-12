package events

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt"
	"github.com/hngprojects/telex_be/internal/models"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	service "github.com/hngprojects/telex_be/services/events"
	"github.com/hngprojects/telex_be/utility"
)

type Options struct {
	DB        *storage.Database
	Validator *validator.Validate
	Logger    *utility.Logger
}
type Controller struct {
	Service   *service.Service
	Validator *validator.Validate
	Logger    *utility.Logger
}

func New(options Options) *Controller {
	return &Controller{Service: service.NewService(options.DB.Postgresql, nil), Validator: options.Validator, Logger: options.Logger}
}
func (base *Controller) respond(c *gin.Context, result any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, utility.BuildSuccessResponse(http.StatusOK, "Events request completed", result))
		return
	}
	code := service.Status(err)
	message := err.Error()
	var fields any
	var eventErr *service.Error
	if errors.As(err, &eventErr) {
		fields = eventErr.Fields
	}
	if code >= 500 {
		message = "Events are temporarily unavailable"
		if base.Logger != nil {
			base.Logger.Error("events request failed: %v", err)
		}
	}
	c.JSON(code, utility.BuildErrorResponse(code, "error", message, fields, nil))
}
func (base *Controller) actor(c *gin.Context) (service.Actor, bool) {
	raw, ok := c.Get("userClaims")
	if !ok {
		base.respond(c, nil, &service.Error{Code: 401, Message: "Authentication required"})
		return service.Actor{}, false
	}
	claims, ok := raw.(jwt.MapClaims)
	if !ok {
		base.respond(c, nil, &service.Error{Code: 401, Message: "Authentication required"})
		return service.Actor{}, false
	}
	user, ok := claims["user_id"].(string)
	if !ok || user == "" {
		base.respond(c, nil, &service.Error{Code: 401, Message: "Authentication required"})
		return service.Actor{}, false
	}
	return service.Actor{UserID: user, OrganisationID: c.Param("org_id")}, true
}
func (base *Controller) bind(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 96*1024)
	if err := c.ShouldBindJSON(value); err != nil {
		base.respond(c, nil, &service.Error{Code: 422, Message: "Invalid request body"})
		return false
	}
	if err := base.Validator.Struct(value); err != nil {
		fields := map[string]string{}
		var validation validator.ValidationErrors
		if errors.As(err, &validation) {
			for _, f := range validation {
				name := f.Field()
				switch name {
				case "OperationID":
					name = "operation_id"
				case "ExpectedRevision":
					name = "expected_revision"
				case "HostID":
					name = "host_id"
				case "EventID":
					name = "event_id"
				case "Going":
					name = "going"
				}
				fields[name] = "Invalid or missing value"
			}
		}
		base.respond(c, nil, &service.Error{Code: 422, Message: "Please check the request", Fields: fields})
		return false
	}
	return true
}
func (base *Controller) Capabilities(c *gin.Context) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	r, e := base.Service.Capabilities(c.Request.Context(), a)
	base.respond(c, r, e)
}
func (base *Controller) List(c *gin.Context) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	page, limit := 1, 20
	for _, v := range []struct {
		name   string
		target *int
	}{{"page", &page}, {"limit", &limit}} {
		if raw := c.Query(v.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				base.respond(c, nil, &service.Error{Code: 422, Message: "Invalid pagination"})
				return
			}
			*v.target = n
		}
	}
	r, e := base.Service.List(c.Request.Context(), service.ListInput{Actor: a, Tab: c.Query("tab"), Date: c.Query("date"), TimeZone: c.Query("time_zone"), Search: c.Query("q"), Page: page, Limit: limit})
	base.respond(c, r, e)
}
func (base *Controller) Get(c *gin.Context) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	r, e := base.Service.Get(c.Request.Context(), service.EventAccessInput{Actor: a, EventID: c.Param("event_id")})
	base.respond(c, r, e)
}
func (base *Controller) write(c *gin.Context, create bool) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	var body models.EventWriteRequest
	if !base.bind(c, &body) {
		return
	}
	input := service.WriteInput{Actor: a, EventID: c.Param("event_id"), Request: body}
	var result models.EventResponse
	var err error
	if create {
		result, err = base.Service.Create(c.Request.Context(), input)
	} else {
		result, err = base.Service.Save(c.Request.Context(), input)
	}
	base.respond(c, result, err)
}
func (base *Controller) Create(c *gin.Context) { base.write(c, true) }
func (base *Controller) Save(c *gin.Context)   { base.write(c, false) }
func (base *Controller) action(c *gin.Context, publish bool) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	var body models.EventActionRequest
	if !base.bind(c, &body) {
		return
	}
	input := service.ActionInput{Actor: a, EventID: c.Param("event_id"), ExpectedRevision: body.ExpectedRevision}
	var result models.EventResponse
	var err error
	if publish {
		result, err = base.Service.Publish(c.Request.Context(), input)
	} else {
		result, err = base.Service.Cancel(c.Request.Context(), input)
	}
	base.respond(c, result, err)
}
func (base *Controller) Publish(c *gin.Context) { base.action(c, true) }
func (base *Controller) Cancel(c *gin.Context)  { base.action(c, false) }
func (base *Controller) Rsvp(c *gin.Context) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	var body models.EventRsvpRequest
	if !base.bind(c, &body) {
		return
	}
	r, e := base.Service.SetRsvp(c.Request.Context(), service.RsvpInput{Actor: a, EventID: c.Param("event_id"), Going: *body.Going})
	base.respond(c, r, e)
}
func (base *Controller) Join(c *gin.Context) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	r, e := base.Service.ResolveJoin(c.Request.Context(), service.EventAccessInput{Actor: a, EventID: c.Param("event_id")})
	base.respond(c, r, e)
}
func (base *Controller) Featured(c *gin.Context) {
	a, ok := base.actor(c)
	if !ok {
		return
	}
	var body models.EventFeaturedRequest
	if !base.bind(c, &body) {
		return
	}
	r, e := base.Service.SetFeatured(c.Request.Context(), service.FeaturedInput{Actor: a, EventID: body.EventID, ExpectedRevision: body.ExpectedRevision})
	base.respond(c, r, e)
}

package test_events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt"
	controller "github.com/hngprojects/telex_be/pkg/controller/events"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
)

func TestHTTPValidationAndMemberCannotManage(t *testing.T) {
	f := setup(t)
	enable(t, f)
	gin.SetMode(gin.TestMode)
	c := controller.New(controller.Options{DB: &storage.Database{Postgresql: f.db}, Validator: validator.New()})
	c.Service = f.s
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userClaims", jwt.MapClaims{"user_id": f.member.UserID}); c.Next() })
	r.GET("/organisations/:org_id/events/capabilities", c.Capabilities)
	r.POST("/organisations/:org_id/events", c.Create)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/organisations/"+f.member.OrganisationID+"/events/capabilities", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("capabilities: %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["data"].(map[string]any)["can_manage"] != false {
		t.Fatal("member management leaked")
	}
	req := httptest.NewRequest("POST", "/organisations/"+f.member.OrganisationID+"/events", strings.NewReader(`{"operation_id":"11111111-1111-4111-8111-111111111111","title":"Forbidden"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("member mutation: %d %s", w.Code, w.Body.String())
	}
}

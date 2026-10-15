package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"
)

func TestUnsupportedCapabilitiesReturnActionableConflict(t *testing.T) {
	failure := protocol.Require(nil, protocol.CapabilityUpgrade)
	response := httptest.NewRecorder()
	writeError(response, errors.Join(service.ErrDeploymentUnavailable, failure))

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), protocol.CapabilityUpgrade) {
		t.Fatal("capability error lost behind transport wrapper", response.Code, response.Body)
	}

	response = httptest.NewRecorder()
	writeFileError(response, &model.FileError{Code: "unsupported_capability", Message: "Spark needs backup deletion support"})

	if response.Code != http.StatusConflict {
		t.Fatal("file capability error status", response.Code)
	}
}

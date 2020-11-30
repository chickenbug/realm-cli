package realm_test

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/10gen/realm-cli/internal/cloud/realm"
	"github.com/10gen/realm-cli/internal/utils/test/assert"
)

func TestServerError(t *testing.T) {
	t.Run("Should unmarshal a non-json response successfully", func(t *testing.T) {
		err := realm.UnmarshalServerError(&http.Response{
			Body: ioutil.NopCloser(strings.NewReader("something bad happened")),
		})
		assert.Equal(t, realm.ServerError{Message: "something bad happened"}, err)
	})

	t.Run("Should unmarshal an empty response with its status", func(t *testing.T) {
		err := realm.UnmarshalServerError(&http.Response{
			Status: "something bad happened",
			Body:   ioutil.NopCloser(strings.NewReader("")),
		})
		assert.Equal(t, realm.ServerError{Message: "something bad happened"}, err)
	})

	t.Run("Should unmarshal a server error payload without an error code successfully", func(t *testing.T) {
		err := realm.UnmarshalServerError(&http.Response{
			Body: ioutil.NopCloser(strings.NewReader(`{"error": "something bad happened"}`)),
		})
		assert.Equal(t, realm.ServerError{Message: "something bad happened"}, err)
	})

	t.Run("Should unmarshal a server error payload with an error code successfully", func(t *testing.T) {
		err := realm.UnmarshalServerError(&http.Response{
			Body: ioutil.NopCloser(strings.NewReader(`{"error": "something bad happened","error_code": "AnErrorCode"}`)),
		})
		assert.Equal(t, realm.ServerError{Code: "AnErrorCode", Message: "something bad happened"}, err)
	})
}

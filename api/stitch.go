package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/10gen/stitch-cli/auth"
	"github.com/10gen/stitch-cli/models"
)

const (
	authProviderCloudLoginRoute = adminBaseURL + "/auth/providers/mongodb-cloud/login"
	appExportRoute              = adminBaseURL + "/groups/%s/apps/%s/export"
	appImportRoute              = adminBaseURL + "/groups/%s/apps/%s/import"
	appsByGroupIDRoute          = adminBaseURL + "/groups/%s/apps"
	userProfileRoute            = "/api/client/v2.0/auth/profile"
)

var (
	errExportMissingFilename = errors.New("the app export response did not specify a filename")
)

// ErrStitchResponse represents a response from a Stitch API call
type ErrStitchResponse struct {
	data errStitchResponseData
}

// Error returns a stringified error message
func (esr ErrStitchResponse) Error() string {
	return fmt.Sprintf("error: %s", esr.data.Error)
}

// UnmarshalJSON unmarshals JSON data into an ErrStitchResponse
func (esr *ErrStitchResponse) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &esr.data)
}

type errStitchResponseData struct {
	Error string `json:"error"`
}

// UnmarshalReader unmarshals an io.Reader into an ErrStitchResponse
func UnmarshalReader(r io.Reader) error {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return err
	}

	str := buf.String()

	var stitchResponse ErrStitchResponse
	if err := json.NewDecoder(&buf).Decode(&stitchResponse); err != nil {
		stitchResponse.data.Error = str
	}

	return stitchResponse
}

// StitchClient represents a Client that can be used to call the Stitch Admin API
type StitchClient interface {
	Authenticate(apiKey, username string) (*auth.Response, error)
	Export(groupID, appID string) (string, io.ReadCloser, error)
	Import(groupID, appID string, appData []byte) error
	FetchAppByClientAppID(clientAppID string) (*models.App, error)
}

// NewStitchClient returns a new StitchClient to be used for making calls to the Stitch Admin API
func NewStitchClient(client Client) StitchClient {
	return &basicStitchClient{
		Client: client,
	}
}

type basicStitchClient struct {
	Client
}

// Authenticate will authenticate a user given an api key and username
func (sc *basicStitchClient) Authenticate(apiKey, username string) (*auth.Response, error) {
	body, err := json.Marshal(map[string]string{
		"apiKey":   apiKey,
		"username": username,
	})
	if err != nil {
		return nil, err
	}

	res, err := sc.Client.ExecuteRequest(http.MethodPost, authProviderCloudLoginRoute, RequestOptions{
		Body: bytes.NewReader(body),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: failed to authenticate: %s", res.Status, UnmarshalReader(res.Body))
	}

	decoder := json.NewDecoder(res.Body)

	var authResponse auth.Response
	if err := decoder.Decode(&authResponse); err != nil {
		return nil, err
	}

	return &authResponse, nil
}

// Export will download a Stitch app as a .zip
func (sc *basicStitchClient) Export(groupID, appID string) (string, io.ReadCloser, error) {
	res, err := sc.ExecuteRequest(http.MethodGet, fmt.Sprintf(appExportRoute, groupID, appID), RequestOptions{})
	if err != nil {
		return "", nil, err
	}

	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		return "", nil, UnmarshalReader(res.Body)
	}

	_, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition"))
	if err != nil {
		res.Body.Close()
		return "", nil, err
	}

	filename := params["filename"]
	if len(filename) == 0 {
		res.Body.Close()
		return "", nil, errExportMissingFilename
	}

	return filename, res.Body, nil
}

// Import will push a local Stitch app to the server
func (sc *basicStitchClient) Import(groupID, appID string, appData []byte) error {
	res, err := sc.ExecuteRequest(http.MethodPost, fmt.Sprintf(appImportRoute, groupID, appID), RequestOptions{
		Body: bytes.NewReader(appData),
	})
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return UnmarshalReader(res.Body)
	}

	return nil
}

func (sc *basicStitchClient) fetchAppsByGroupID(groupID string) ([]*models.App, error) {
	res, err := sc.ExecuteRequest(http.MethodGet, fmt.Sprintf(appsByGroupIDRoute, groupID), RequestOptions{})
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, UnmarshalReader(res.Body)
	}

	dec := json.NewDecoder(res.Body)
	var apps []*models.App
	if err := dec.Decode(&apps); err != nil {
		return nil, err
	}

	return apps, nil
}

// FetchAppByClientAppID fetches a Stitch app given a clientAppID
func (sc *basicStitchClient) FetchAppByClientAppID(clientAppID string) (*models.App, error) {
	res, err := sc.ExecuteRequest(http.MethodGet, userProfileRoute, RequestOptions{})
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, UnmarshalReader(res.Body)
	}

	dec := json.NewDecoder(res.Body)
	var profileData models.UserProfile
	if err := dec.Decode(&profileData); err != nil {
		return nil, err
	}

	for _, groupID := range profileData.AllGroupIDs() {
		apps, err := sc.fetchAppsByGroupID(groupID)
		if err != nil {
			return nil, err
		}

		if app := findAppByClientAppID(apps, clientAppID); app != nil {
			return app, nil
		}
	}

	return nil, fmt.Errorf("unable to find app with ID: %s", clientAppID)
}

func findAppByClientAppID(apps []*models.App, clientAppID string) *models.App {
	for _, app := range apps {
		if app.ClientAppID == clientAppID {
			return app
		}
	}

	return nil
}

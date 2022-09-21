package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/wesovilabs/koazee"
	"go.uber.org/zap"
	"io"
	"net/http"
)

var logger, _ = zap.NewProduction()

type ClientConfig struct {
	Address  string
	Port     int
	Password string
	Client   *http.Client
}

type NexusError struct {
	message    string
	statuscode int
}

func (m NexusError) Error() string {
	return fmt.Sprintf("Message: %s. StatusCode %d", m.message, m.statuscode)
}

func (r *ClientConfig) baseUrl() string {
	return fmt.Sprintf("https://%s:%d/service/rest/v1/", r.Address, r.Port)
}

func (r *ClientConfig) ChangeAdmin123Password() error {
	if len(r.Password) > 0 && r.Password != "admin123" {
		url := fmt.Sprintf(r.baseUrl() + "security/users/admin/change-password")
		request, err := http.NewRequest("PUT", url, bytes.NewBuffer([]byte(r.Password)))
		//request, err := http.Post(url, "text/plain", bytes.NewBuffer([]byte(r.Password)))
		if err != nil {
			return err
		}
		request.Header.Set("accept", "application/json")
		request.Header.Set("Content-Type", "text/plain")
		request.SetBasicAuth("admin", "admin123")

		response, err := r.Client.Do(request)
		if err != nil {
			return err
		}
		switch status := response.StatusCode; status {
		case http.StatusUnauthorized:
			logger.Info("Password already changed")
		case http.StatusNoContent:
			{
				logger.Info("Password changed")
				return nil
			}
		default:
			return NexusError{
				message:    "Unknown error",
				statuscode: status,
			}
		}

	}

	return nil
}

func (r *ClientConfig) AddBlobStore(name string, spaceUsedQuotaMb int) error {

	url := fmt.Sprintf(r.baseUrl() + fmt.Sprintf("blobstores/%s/quota-status", name))
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("accept", "application/json")
	request.SetBasicAuth("admin", r.Password)
	getResponse, err := r.Client.Do(request)
	if err != nil {
		return err
	}

	switch status := getResponse.StatusCode; status {
	case http.StatusOK:
		logger.Info(fmt.Sprintf("Blobstore %s already defined", name))
	case http.StatusNotFound:
		{
			logger.Info(fmt.Sprintf("Creating blobstore %s ", name))
			return r.createBlobStore(name, spaceUsedQuotaMb)
		}
	default:
		return NexusError{
			message:    "Unknown error",
			statuscode: status,
		}
	}

	return nil
}

func (r *ClientConfig) createBlobStore(name string, spaceUsedQuotaMb int) error {
	url := fmt.Sprintf(r.baseUrl() + "blobstores/file")
	storeRequest := newBlobStoreRequest(name, spaceUsedQuotaMb*1000)
	b, err := json.Marshal(storeRequest)
	if err != nil {
		panic(err)
	}
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("accept", "application/json")
	request.SetBasicAuth("admin", r.Password)
	response, err := r.Client.Do(request)
	if err != nil {
		return err
	}

	switch status := response.StatusCode; status {
	case http.StatusNoContent:
		logger.Info(fmt.Sprintf("Blobstore %s created", name))

	default:
		return NexusError{
			message:    "Unknown error",
			statuscode: status,
		}
	}
	return nil
}

func (r *ClientConfig) ActivateRealm(realmsRequest []string) error {
	var realmsToActivate []string
	{ // Determine the active realms
		url := fmt.Sprintf(r.baseUrl() + "security/realms/active")
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("accept", "application/json")
		request.SetBasicAuth("admin", r.Password)

		resp, err := r.Client.Do(request)
		if err != nil {
			return err
		}

		// Close request body anyway
		defer func() {
			_ = resp.Body.Close()
		}()
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		var activeRealms []string
		err = json.Unmarshal(content, &activeRealms)
		if err != nil {
			return err
		}
		stream := koazee.StreamOf(activeRealms)

		for _, realmToActivate := range realmsRequest {
			contains, _ := stream.Contains(realmToActivate)
			// We have a not activated realm
			// Merge the actives and the request
			// Otherwise nexus will remove the current active realms
			if !contains {
				realmsToActivate = append(activeRealms, realmsRequest...)
				break
			}
		}
	}

	if len(realmsToActivate) > 0 {

		url := fmt.Sprintf(r.baseUrl() + "security/realms/active")
		//realm := fmt.Sprintf("[\"%s\"]", name)
		b, err := json.Marshal(realmsToActivate)
		if err != nil {
			panic(err)
		}
		request, err := http.NewRequest("PUT", url, bytes.NewBuffer(b))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("accept", "application/json")
		request.SetBasicAuth("admin", r.Password)
		response, err := r.Client.Do(request)
		if err != nil {
			return err
		}

		switch status := response.StatusCode; status {
		case http.StatusNoContent:
			logger.Info(fmt.Sprintf("Realms %s added", realmsToActivate))

		default:
			return NexusError{
				message:    "Unknown error",
				statuscode: status,
			}
		}
	}
	return nil
}

type softQuota struct {
	Type  string `json:"type"`
	Limit int    `json:"limit"`
}
type blobStoreRequest struct {
	Quota *softQuota `json:"softQuota,omitempty"`
	Path  string     `json:"path"`
	Name  string     `json:"name"`
}

func newBlobStoreRequest(name string, space int) blobStoreRequest {

	if space > 0 {
		return blobStoreRequest{
			Quota: &softQuota{
				Type:  "spaceUsedQuota",
				Limit: space,
			},
			Path: fmt.Sprintf("%s/blobs", name),
			Name: name,
		}
	}
	return blobStoreRequest{
		Path: fmt.Sprintf("%s/blobs", name),
		Name: name,
	}
}

type repoDockerProxy struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Storage *struct {
		BlobStoreName               string `json:"blobStoreName"`
		StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
	} `json:"storage"`
	Cleanup *struct {
		PolicyNames []string `json:"policyNames"`
	} `json:"cleanup"`
	Proxy struct {
		RemoteUrl      string `json:"remoteUrl"`
		ContentMaxAge  int    `json:"contentMaxAge"`
		MetadataMaxAge int    `json:"metadataMaxAge"`
	} `json:"proxy"`
	NegativeCache *struct {
		Enabled    bool `json:"enabled"`
		TimeToLive int  `json:"timeToLive"`
	} `json:"negativeCache"`
	HttpClient struct {
		Blocked    bool `json:"blocked"`
		AutoBlock  bool `json:"autoBlock"`
		Connection struct {
			Retries                 int    `json:"retries"`
			UserAgentSuffix         string `json:"userAgentSuffix"`
			Timeout                 int    `json:"timeout"`
			EnableCircularRedirects bool   `json:"enableCircularRedirects"`
			EnableCookies           bool   `json:"enableCookies"`
			UseTrustStore           bool   `json:"useTrustStore"`
		} `json:"connection"`
		Authentication struct {
			Type       string `json:"type"`
			Username   string `json:"username"`
			Password   string `json:"password"`
			NtlmHost   string `json:"ntlmHost"`
			NtlmDomain string `json:"ntlmDomain"`
		} `json:"authentication"`
	} `json:"httpClient"`
	RoutingRule string `json:"routingRule"`
	Replication struct {
	} `json:"replication"`
	Docker struct {
		V1Enabled      bool   `json:"v1Enabled"`
		ForceBasicAuth bool   `json:"forceBasicAuth"`
		HttpPort       int    `json:"httpPort"`
		HttpsPort      int    `json:"httpsPort"`
		Subdomain      string `json:"subdomain"`
	} `json:"docker"`
	DockerProxy struct {
		IndexType string `json:"indexType"`
		IndexUrl  string `json:"indexUrl"`
	} `json:"dockerProxy"`
}

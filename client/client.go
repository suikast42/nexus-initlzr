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

func (r *ClientConfig) AddDockerRepos(realmsRequest []string) error {
	pushRepo, err := r.getOrCreateDockerLocalRepo(false)
	if err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Repo docker pushRepo is there %s", pushRepo.Name))
	pullRepo, err := r.getOrCreateDockerGroupRepo(false)
	if err != nil {
		return err
	}
	logger.Info(fmt.Sprintf("Repo docker pullRepo is there %s", pullRepo.Name))
	return nil
}

func (r *ClientConfig) getOrCreateDockerLocalRepo(secondCall bool) (*dockerLocalRepo, error) {
	var dockerLocalRepo *dockerLocalRepo
	{ // Determine the active realms
		url := fmt.Sprintf(r.baseUrl() + "repositories/docker/hosted/dockerLocal")
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("accept", "application/json")
		request.SetBasicAuth("admin", r.Password)
		response, err := r.Client.Do(request)
		if err != nil {
			return nil, err
		}
		// Close request body anyway
		defer func() {
			_ = response.Body.Close()
		}()

		switch status := response.StatusCode; status {
		case http.StatusOK:
			{
				content, err := io.ReadAll(response.Body)
				if err != nil {
					return nil, err
				}
				err = json.Unmarshal(content, &dockerLocalRepo)
				if err != nil {
					return nil, err
				}
			}
		case http.StatusNotFound:
			{
				if secondCall {
					return nil, NexusError{
						message:    "´Can't create dockerLocal repo ",
						statuscode: status,
					}
				}
				url := fmt.Sprintf(r.baseUrl() + "repositories/docker/hosted")
				dockerLocalRepo := newDockerLocalRepo()
				b, err := json.Marshal(dockerLocalRepo)
				if err != nil {
					return nil, err
				}
				request, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
				if err != nil {
					return nil, err
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("accept", "application/json")
				request.SetBasicAuth("admin", r.Password)
				response, err := r.Client.Do(request)
				if err != nil {
					return nil, err
				}
				defer func() {
					_ = response.Body.Close()
				}()
				switch status := response.StatusCode; status {
				case http.StatusCreated:
					return r.getOrCreateDockerLocalRepo(true)
				default:
					return nil, NexusError{
						message:    "Unknown error",
						statuscode: status,
					}
				}
			}
		default:
			return nil, NexusError{
				message:    "Unknown error",
				statuscode: status,
			}
		}

	}

	return dockerLocalRepo, nil
}
func (r *ClientConfig) getOrCreateDockerGroupRepo(secondCall bool) (*dockerGroupRepo, error) {
	var dockerGroup *dockerGroupRepo
	{ // Determine the active realms
		url := fmt.Sprintf(r.baseUrl() + "repositories/docker/group/dockerGroup")
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("accept", "application/json")
		request.SetBasicAuth("admin", r.Password)

		response, err := r.Client.Do(request)
		if err != nil {
			return nil, err
		}
		// Close request body anyway
		defer func() {
			_ = response.Body.Close()
		}()
		switch status := response.StatusCode; status {
		case http.StatusOK:
			{ // Docker group repo found
				content, err := io.ReadAll(response.Body)
				if err != nil {
					return nil, err
				}
				err = json.Unmarshal(content, &dockerGroup)
				if err != nil {
					return nil, err
				}
			}
			// Create docker repo
		case http.StatusNotFound:
			{
				if secondCall {
					return nil, NexusError{
						message:    "´Can't create dockerGroup repo ",
						statuscode: status,
					}
				}
				url := fmt.Sprintf(r.baseUrl() + "repositories/docker/group")
				dockerGroupRepo := newDockerGroupRepo()
				b, err := json.Marshal(dockerGroupRepo)
				if err != nil {
					return nil, err
				}
				request, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
				if err != nil {
					return nil, err
				}
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("accept", "application/json")
				request.SetBasicAuth("admin", r.Password)
				response, err := r.Client.Do(request)
				if err != nil {
					return nil, err
				}
				defer func() {
					_ = response.Body.Close()
				}()
				switch status := response.StatusCode; status {
				case http.StatusCreated:
					return r.getOrCreateDockerGroupRepo(true)
				default:
					return nil, NexusError{
						message:    "Unknown error",
						statuscode: status,
					}
				}

			}
		default:
			return nil, NexusError{
				message:    "Unknown error",
				statuscode: status,
			}
		}

	}

	return dockerGroup, nil
}
func newDockerLocalRepo() dockerLocalRepo {
	return dockerLocalRepo{
		Name:   "dockerLocal",
		Online: true,
		Storage: struct {
			BlobStoreName               string `json:"blobStoreName"`
			StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
			WritePolicy                 string `json:"writePolicy"`
		}{
			BlobStoreName:               "docker",
			StrictContentTypeValidation: false,
			WritePolicy:                 "allow",
		},
		Docker: struct {
			V1Enabled      bool   `json:"v1Enabled"`
			ForceBasicAuth bool   `json:"forceBasicAuth"`
			HttpPort       int    `json:"httpPort"`
			HttpsPort      int    `json:"httpsPort,omitempty"`
			Subdomain      string `json:"subdomain,omitempty"`
		}{
			V1Enabled:      false,
			ForceBasicAuth: false,
			HttpPort:       5001,
		},
	}
}

type dockerLocalRepo struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Storage struct {
		BlobStoreName               string `json:"blobStoreName"`
		StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
		WritePolicy                 string `json:"writePolicy"`
	} `json:"storage"`
	Cleanup *struct {
		PolicyNames []string `json:"policyNames"`
	} `json:"cleanup"`
	Component struct {
		ProprietaryComponents bool `json:"proprietaryComponents"`
	} `json:"component,omitempty"`
	Docker struct {
		V1Enabled      bool   `json:"v1Enabled"`
		ForceBasicAuth bool   `json:"forceBasicAuth"`
		HttpPort       int    `json:"httpPort"`
		HttpsPort      int    `json:"httpsPort,omitempty"`
		Subdomain      string `json:"subdomain,omitempty"`
	} `json:"docker"`
}

func newDockerGroupRepo() dockerGroupRepo {
	return dockerGroupRepo{
		Name:   "dockerGroup",
		Online: true,
		Storage: struct {
			BlobStoreName               string `json:"blobStoreName"`
			StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
		}{
			BlobStoreName:               "docker",
			StrictContentTypeValidation: true,
		},
		Group: struct {
			MemberNames    []string `json:"memberNames"`
			WritableMember string   `json:"writableMember"`
		}{
			MemberNames: []string{"dockerLocal"},
		},
		Docker: struct {
			V1Enabled      bool   `json:"v1Enabled"`
			ForceBasicAuth bool   `json:"forceBasicAuth"`
			HttpPort       int    `json:"httpPort"`
			HttpsPort      int    `json:"httpsPort,omitempty"`
			Subdomain      string `json:"subdomain,omitempty"`
		}{
			V1Enabled:      false,
			ForceBasicAuth: false,
			HttpPort:       5000,
			HttpsPort:      0,
			Subdomain:      "",
		},
	}
}

type dockerGroupRepo struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Storage struct {
		BlobStoreName               string `json:"blobStoreName"`
		StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
	} `json:"storage"`
	Group struct {
		MemberNames    []string `json:"memberNames"`
		WritableMember string   `json:"writableMember"`
	} `json:"group"`
	Docker struct {
		V1Enabled      bool   `json:"v1Enabled"`
		ForceBasicAuth bool   `json:"forceBasicAuth"`
		HttpPort       int    `json:"httpPort"`
		HttpsPort      int    `json:"httpsPort,omitempty"`
		Subdomain      string `json:"subdomain,omitempty"`
	} `json:"docker"`
}

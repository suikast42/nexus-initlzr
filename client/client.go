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

func (r *ClientConfig) AddDockerRepos(repos []DockerGroup) error {
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

	for _, repoReq := range repos {
		repo, err := r.getOrCreateProxyRepo(false, repoReq)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Repo docker pullRepo is there %s", repo.Name))

		members := koazee.StreamOf(pullRepo.Group.MemberNames)
		contains, _ := members.Contains(repoReq.Name)
		if !contains {
			logger.Error("Pull repo does not contain repo . Implement this")
		}

	}

	logger.Info(fmt.Sprintf("Repo docker pushRepo is there %s", pushRepo.Name))
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
	{
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
func (r *ClientConfig) getOrCreateProxyRepo(secondCall bool, repo DockerGroup) (*dockerLocalRepo, error) {
	var dockerLocalRepo *dockerLocalRepo
	{
		url := fmt.Sprintf(r.baseUrl() + fmt.Sprintf("repositories/docker/proxy/%s", repo.Name))
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
				err = json.Unmarshal(content, &dockerLocalRepo)
				if err != nil {
					return nil, err
				}
			}
			// Create docker repo
		case http.StatusNotFound:
			{
				if secondCall {
					return nil, NexusError{
						message:    fmt.Sprintf("Can't create repo %s", repo.Name),
						statuscode: status,
					}
				}
				url := fmt.Sprintf(r.baseUrl() + "repositories/docker/proxy")
				dockerLocalRepo := newDockerProxyRepos(repo.Name, repo.Url, repo.Username, repo.Password)
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
					return r.getOrCreateProxyRepo(true, repo)
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

func newDockerProxyRepos(name string, url string, username string, password string) dockerProxyRepos {
	repo := dockerProxyRepos{
		Name:   name,
		Online: true,
		Storage: struct {
			BlobStoreName               string `json:"blobStoreName"`
			StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
		}{
			BlobStoreName:               "docker",
			StrictContentTypeValidation: false,
		},
		Proxy: struct {
			RemoteUrl      string `json:"remoteUrl"`
			ContentMaxAge  int    `json:"contentMaxAge"`
			MetadataMaxAge int    `json:"metadataMaxAge"`
		}{
			RemoteUrl:      url,
			ContentMaxAge:  1440,
			MetadataMaxAge: 1440,
		},
		NegativeCache: struct {
			Enabled    bool `json:"enabled"`
			TimeToLive int  `json:"timeToLive"`
		}{Enabled: true, TimeToLive: 1440},
		HttpClient: struct {
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
		}{
			Blocked:   false,
			AutoBlock: false,
			Connection: struct {
				Retries                 int    `json:"retries"`
				UserAgentSuffix         string `json:"userAgentSuffix"`
				Timeout                 int    `json:"timeout"`
				EnableCircularRedirects bool   `json:"enableCircularRedirects"`
				EnableCookies           bool   `json:"enableCookies"`
				UseTrustStore           bool   `json:"useTrustStore"`
			}{Timeout: 20},
		},

		DockerProxy: struct {
			IndexType string `json:"indexType"`
			IndexUrl  string `json:"indexUrl"`
		}{},
		Docker: struct {
			V1Enabled      bool   `json:"v1Enabled"`
			ForceBasicAuth bool   `json:"forceBasicAuth"`
			HttpPort       int    `json:"httpPort,omitempty"`
			HttpsPort      int    `json:"httpsPort,omitempty"`
			Subdomain      string `json:"subdomain,omitempty"`
		}{V1Enabled: false, ForceBasicAuth: false},

		//{V1Enabled: false, ForceBasicAuth: false, HttpPort: 8500, HttpsPort: 8501},
	}

	if "dockerHub" == name {
		repo.DockerProxy.IndexType = "HUB"
		repo.DockerProxy.IndexUrl = "https://index.docker.io"
	} else {
		repo.DockerProxy.IndexType = "REGISTRY"
	}
	if len(username) > 0 {
		repo.HttpClient.Authentication.Username = username
		repo.HttpClient.Authentication.Type = "username"
	}
	if len(password) > 0 {
		repo.HttpClient.Authentication.Password = password
	}
	marshal, _ := json.Marshal(repo)
	fmt.Printf("%+v\n", string(marshal))
	return repo
}

type dockerProxyRepos struct {
	Name    string `json:"name"`
	Online  bool   `json:"online"`
	Storage struct {
		BlobStoreName               string `json:"blobStoreName"`
		StrictContentTypeValidation bool   `json:"strictContentTypeValidation"`
	} `json:"storage"`
	Cleanup *struct {
		PolicyNames []string `json:"policyNames"`
	} `json:"cleanup,omitempty"`
	Proxy struct {
		RemoteUrl      string `json:"remoteUrl"`
		ContentMaxAge  int    `json:"contentMaxAge"`
		MetadataMaxAge int    `json:"metadataMaxAge"`
	} `json:"proxy"`
	NegativeCache struct {
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
			//username, ntlm
			Type       string `json:"type"`
			Username   string `json:"username"`
			Password   string `json:"password"`
			NtlmHost   string `json:"ntlmHost"`
			NtlmDomain string `json:"ntlmDomain"`
		} `json:"authentication"`
	} `json:"httpClient"`
	RoutingRuleName *string `json:"routingRuleName,omitempty"`
	Replication     *struct {
		PreemptivePullEnabled bool   `json:"preemptivePullEnabled"`
		AssetPathRegex        string `json:"assetPathRegex"`
	} `json:"replication,omitempty"`
	Docker struct {
		V1Enabled      bool   `json:"v1Enabled"`
		ForceBasicAuth bool   `json:"forceBasicAuth"`
		HttpPort       int    `json:"httpPort,omitempty"`
		HttpsPort      int    `json:"httpsPort,omitempty"`
		Subdomain      string `json:"subdomain,omitempty"`
	} `json:"docker,omitempty"`
	DockerProxy struct {
		//[ HUB, REGISTRY, CUSTOM ]
		IndexType string `json:"indexType"`
		IndexUrl  string `json:"indexUrl"`
	} `json:"dockerProxy"`
}

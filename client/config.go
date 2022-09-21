package client

type NexusConfig struct {
	Address    string `json:"address"`
	Port       int    `json:"port"`
	Password   string `json:"password"`
	BlobStores []struct {
		Name     string `json:"name"`
		Capacity int    `json:"capacity"`
	} `json:"blobStores"`
	DockerGroup []DockerGroup
}

type DockerGroup struct {
	Name     string `json:"name"`
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

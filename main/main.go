package main

import (
	"crypto/tls"
	"fmt"
	"github.com/spf13/viper"
	"github.com/suikast42/nexus-initlzr/client"
	"go.uber.org/zap"
	"net/http"
	"os"
)

var logger, _ = zap.NewProduction()

func main() {
	err := readConfig()
	if err != nil {
		panic(err)
	}

	var nexusConfig client.NexusConfig
	err = viper.Unmarshal(&nexusConfig)
	if err != nil {
		panic(err)
	}

	nexusClient := client.ClientConfig{
		Address:  nexusConfig.Address,
		Port:     nexusConfig.Port,
		Password: nexusConfig.Password,
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
	logger.Info(fmt.Sprintf("nexus.address: %s", nexusClient.Address))
	logger.Info(fmt.Sprintf("nexus.port: %d", nexusClient.Port))
	err = nexusClient.WaitForUp()
	if err != nil {
		panic(err)
	}
	err = nexusClient.ChangeAdmin123Password()
	if err != nil {
		panic(err)
	}

	for _, v := range nexusConfig.BlobStores {
		err := nexusClient.AddBlobStore(v.Name, v.Capacity)
		if err != nil {
			panic(err)
		}
	}
	realms := []string{"DockerToken"}
	err = nexusClient.ActivateRealm(realms)
	if err != nil {
		panic(err)
	}

	err = nexusClient.AddDockerRepos(nexusConfig.DockerGroup)
	if err != nil {
		panic(err)
	}
}

func readConfig() error {
	viper.SetConfigType("json") // Look for specific type
	{ //initialize local cfg
		viper.AddConfigPath("./")
		viper.SetConfigName("config") // Register config file name (no extension)
		err := viper.ReadInConfig()
		if err != nil {
			return err
		}
	}
	{
		cfg, present := os.LookupEnv("NEXUS_INIT_CONFIG_PATH")
		if present {
			viper.AddConfigPath(cfg)
		}
	}
	{
		cfg, present := os.LookupEnv("NEXUS_INIT_CONFIG_FILE")
		if present {
			viper.SetConfigName(cfg) // Register config file name (no extension)
			err := viper.MergeInConfig()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

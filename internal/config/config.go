package config

import (
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
)

var (
	loadOnce sync.Once
	config   Config
)

type Config struct {
	SyncPeriod             time.Duration `envconfig:"sync_period"`
	PollInterval           time.Duration `envconfig:"poll_interval"`
	LongPollInterval       time.Duration `envconfig:"long_poll_interval"`
	ManagementNamespace    string        `envconfig:"management_namespace"`
	EnableNamespaceSecrets bool          `envconfig:"enable_namespace_secrets"`
	Suspend                bool          `envconfig:"suspend"`
	ClusterID              string        `envconfig:"cluster_id"`
}

func Get() Config {
	loadOnce.Do(func() {
		config = Config{ // default values
			SyncPeriod:             60 * time.Second,
			PollInterval:           10 * time.Second,
			LongPollInterval:       5 * time.Minute,
			EnableNamespaceSecrets: true,
			Suspend:                false,
		}
		envconfig.MustProcess("", &config)
	})
	return config
}

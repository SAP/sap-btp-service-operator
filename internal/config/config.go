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
	SyncPeriod                time.Duration `envconfig:"sync_period"`
	PollInterval              time.Duration `envconfig:"poll_interval"`
	LongPollInterval          time.Duration `envconfig:"long_poll_interval"`
	ManagementNamespace       string        `envconfig:"management_namespace"`
	ReleaseNamespace          string        `envconfig:"release_namespace"`
	AllowClusterAccess        bool          `envconfig:"allow_cluster_access"`
	AllowedNamespaces         []string      `envconfig:"allowed_namespaces"`
	EnableNamespaceSecrets    bool          `envconfig:"enable_namespace_secrets"`
	ClusterID                 string        `envconfig:"cluster_id"`
	RetryBaseDelay            time.Duration `envconfig:"retry_base_delay"`
	RetryMaxDelay             time.Duration `envconfig:"retry_max_delay"`
	IgnoreNonTransientTimeout time.Duration
}

func Get() Config {
	loadOnce.Do(func() {
		config = Config{ // default values
			SyncPeriod:                60 * time.Second,
			PollInterval:              10 * time.Second,
			LongPollInterval:          5 * time.Minute,
			EnableNamespaceSecrets:    true,
			AllowedNamespaces:         []string{},
			AllowClusterAccess:        true,
			RetryBaseDelay:            10 * time.Second,
			RetryMaxDelay:             time.Hour,
			IgnoreNonTransientTimeout: 2 * time.Hour,
		}
		envconfig.MustProcess("", &config)
	})
	return config
}

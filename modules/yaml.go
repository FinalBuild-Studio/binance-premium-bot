package modules

import (
	"io/ioutil"
	"path/filepath"
	"sync"

	"github.com/CapsLock-Studio/binance-premium-bot/models"
	"go.uber.org/ratelimit"
	"gopkg.in/yaml.v2"
)

type Yaml struct {
	Path        string
	RateLimiter ratelimit.Limiter
}

func NewYaml(path string, ratelimiter ratelimit.Limiter) *Yaml {
	return &Yaml{
		Path:        path,
		RateLimiter: ratelimiter,
	}
}

func (y *Yaml) Run() {
	filename, _ := filepath.Abs(y.Path)
	file, err := ioutil.ReadFile(filename)

	if err != nil {
		panic(err)
	}

	config := models.Config{}
	yaml.Unmarshal(file, &config)

	wg := &sync.WaitGroup{}

	for _, setting := range config.Settings {
		wg.Add(1)

		go func(setting models.ConfigSetting) {
			defer wg.Done()
			if setting.ApiKey == "" {
				setting.ApiKey = config.ApiKey
			}

			if setting.ApiSecret == "" {
				setting.ApiSecret = config.ApiSecret
			}

			if setting.Leverage == 0 {
				setting.Leverage = config.Leverage
			}

			if setting.Difference == 0 {
				setting.Difference = config.Difference
			}

			NewCore(&setting, nil, nil, y.RateLimiter).Run()
		}(setting)
	}

	wg.Wait()
}

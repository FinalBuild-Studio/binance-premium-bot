package models

type BaseConfig struct {
	ApiKey     string  `yaml:"apiKey" json:"apiKey"`
	ApiSecret  string  `yaml:"apiSecret" json:"apiSecret"`
	Leverage   int     `yaml:"leverage" json:"leverage"`
	Difference float64 `yaml:"difference" json:"difference"`
	Before     float64 `yaml:"before" json:"before"`
	Webhook    string  `yaml:"webhook" json:"webhook"`
	Threshold  float64 `yaml:"threshold" json:"threshold"`
}

type ConfigSetting struct {
	BaseConfig
	Symbol    string  `yaml:"symbol" json:"symbol"`
	Quantity  float64 `yaml:"quantity" json:"quantity"`
	Total     float64 `yaml:"total" json:"total"`
	Reduce    bool    `yaml:"reduce" json:"reduce"`
	Arbitrage bool    `yaml:"arbitrage" json:"arbitrage"`
	UserID    string  `json:"-"`
}

type Config struct {
	BaseConfig
	Settings []ConfigSetting `yaml:"settings"`
}

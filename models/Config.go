package models

type ConfigSetting struct {
	Symbol     string  `yaml:"symbol" json:"symbol"`
	ApiKey     string  `yaml:"apiKey" json:"apiKey"`
	ApiSecret  string  `yaml:"apiSecret" json:"apiSecret"`
	Quantity   float64 `yaml:"quantity" json:"quantity"`
	Total      float64 `yaml:"total" json:"total"`
	Reduce     bool    `yaml:"reduce" json:"reduce"`
	Arbitrage  bool    `yaml:"arbitrage" json:"arbitrage"`
	Difference float64 `yaml:"difference" json:"difference"`
	Leverage   int     `yaml:"leverage" json:"leverage"`
	BidSide    string  `yaml:"bidSide" json:"bidSide"`
	Monitor    bool    `yaml:"monitor" json:"monitor"`
}

type Config struct {
	ApiKey     string          `yaml:"apiKey"`
	ApiSecret  string          `yaml:"apiSecret"`
	Leverage   int             `yaml:"leverage"`
	Difference float64         `yaml:"difference"`
	Settings   []ConfigSetting `yaml:"settings"`
}

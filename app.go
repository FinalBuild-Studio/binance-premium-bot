package main

import (
	"flag"

	"github.com/CapsLock-Studio/binance-premium-bot/models"
	"go.uber.org/ratelimit"

	m "github.com/CapsLock-Studio/binance-premium-bot/modules"
)

func main() {
	apiKey := flag.String("apiKey", "", "binance api key")
	apiSecret := flag.String("apiSecret", "", "binance api secret")
	symbol := flag.String("symbol", "", "binance future symbol")
	quantity := flag.Float64("quantity", 0, "quantity per order")
	total := flag.Float64("total", 0, "total quantity")
	reduce := flag.Bool("reduce", false, "use reduce mode")
	arbitrage := flag.Bool("arbitrage", false, "use arbitrage mode")
	difference := flag.Float64("difference", m.DEFAULT_DIFFERENCE, "BUSD & USDT difference")
	leverage := flag.Int("leverage", 10, "futures leverage")
	config := flag.String("config", "", "yaml config for multi-assets")
	serve := flag.Bool("serve", false, "serve in http mode")
	threshold := flag.Float64("threshold", 0, "minimum threshold")
	before := flag.Float64("before", m.DEFAULT_MINUTES, "change direction before n minutes")
	flag.Parse()

	ratelimiter := ratelimit.New(1)

	if *serve {
		m.NewHttp(ratelimiter).Serve()
	} else if *config != "" {
		m.NewYaml(*config, ratelimiter).Run()
	} else {
		m.NewCore(&models.ConfigSetting{
			ApiKey:     *apiKey,
			ApiSecret:  *apiSecret,
			Symbol:     *symbol,
			Quantity:   *quantity,
			Total:      *total,
			Reduce:     *reduce,
			Arbitrage:  *arbitrage,
			Difference: *difference,
			Leverage:   *leverage,
			Threshold:  *threshold,
			Before:     *before,
		}, nil, nil, ratelimiter).Run()
	}
}

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
	difference := flag.Float64("difference", .05, "BUSD & USDT difference")
	leverage := flag.Int("leverage", 10, "futures leverage")
	bidSide := flag.String("bidSide", "", "determine bid side in reduce mode")
	config := flag.String("config", "", "yaml config for multi-assets")
	monitor := flag.Bool("monitor", false, "assume you have positions on binance")
	serve := flag.Bool("serve", false, "serve in http mode")
	flag.Parse()

	ratelimiter := ratelimit.New(1)

	if *serve {
		m.NewHttp(ratelimiter).Serve()
	} else if *config != "" {
		m.NewYaml(*config, ratelimiter)
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
			BidSide:    *bidSide,
			Monitor:    *monitor,
		}, nil, nil, ratelimiter).Run()
	}
}

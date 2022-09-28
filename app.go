package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/parnurzeal/gorequest"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"

	binance "github.com/CapsLock-Studio/binance-premium-index/models"
)

// {"type":"MARKET","symbol":"BTCUSDT","side":"BUY","quantity":"0.001"}
type BinancePlaceOrder struct {
	Type       string `json:"type"`
	Symbol     string `json:"symbol"`
	Side       string `json:"side"`
	Quantity   string `json:"quantity"`
	ReduceOnly string `json:"reduceOnly"`
}

type ConfigSetting struct {
	Symbol     string  `yaml:"symbol"`
	ApiKey     string  `yaml:"apiKey"`
	ApiSecret  string  `yaml:"apiSecret"`
	Quantity   float64 `yaml:"quantity"`
	Total      float64 `yaml:"total"`
	Reduce     bool    `yaml:"reduce"`
	Arbitrage  bool    `yaml:"arbitrage"`
	Difference float64 `yaml:"difference"`
	Leverage   int     `yaml:"leverage"`
	BidSide    string  `yaml:"bidSide"`
}

type Config struct {
	ApiKey     string          `yaml:"apiKey"`
	ApiSecret  string          `yaml:"apiSecret"`
	Leverage   int             `yaml:"leverage"`
	Difference float64         `yaml:"difference"`
	Settings   []ConfigSetting `yaml:"settings"`
}

func getDepth(
	symbol string,
	currency string,
) (
	bid,
	bidSize,
	ask,
	askSize float64,
) {
	bidAndAsk := struct {
		Asks [][]string `json:"asks"`
		Bids [][]string `json:"bids"`
	}{}

	fapi(
		"/depth",
		gorequest.GET,
		"",
		"",
		map[string]string{
			"limit":  "5",
			"symbol": symbol + currency,
		},
	).EndStruct(&bidAndAsk)

	bid, _ = strconv.ParseFloat(bidAndAsk.Bids[len(bidAndAsk.Bids)-1][0], 64)
	ask, _ = strconv.ParseFloat(bidAndAsk.Asks[len(bidAndAsk.Asks)-1][0], 64)
	bidSize, _ = strconv.ParseFloat(bidAndAsk.Bids[len(bidAndAsk.Bids)-1][1], 64)
	askSize, _ = strconv.ParseFloat(bidAndAsk.Asks[len(bidAndAsk.Asks)-1][1], 64)

	return
}

func fapi(
	path,
	method,
	apiKey,
	apiSecret string,
	body map[string]string,
) *gorequest.SuperAgent {
	if body != nil {
		params := url.Values{}

		params.Add("timestamp", decimal.NewFromInt(time.Now().UnixMilli()).String())

		for key, value := range body {
			params.Add(key, value)
		}

		mac := hmac.New(sha256.New, []byte(apiSecret))
		mac.Write([]byte(params.Encode()))
		signingKey := fmt.Sprintf("%x", mac.Sum(nil))

		params.Add("signature", signingKey)

		path += "?" + params.Encode()
	}

	req := gorequest.
		New().
		CustomMethod(method, "https://fapi.binance.com/fapi/v1"+path)

	req.Header.Set("X-MBX-APIKEY", apiKey)

	return req
}

func binanceIndexDirection(index []binance.BinancePremium) bool {
	var busd float64
	var usdt float64

	for _, v := range index {
		if strings.HasSuffix(v.Symbol, "BUSD") {
			busd, _ = strconv.ParseFloat(v.MarkPrice, 64)
		}

		if strings.HasSuffix(v.Symbol, "USDT") {
			usdt, _ = strconv.ParseFloat(v.MarkPrice, 64)
		}
	}

	return busd > usdt
}

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
	flag.Parse()

	if *config != "" {
		filename, _ := filepath.Abs(*config)
		file, err := ioutil.ReadFile(filename)

		if err != nil {
			panic(err)
		}

		config := Config{}
		yaml.Unmarshal(file, &config)

		wg := &sync.WaitGroup{}

		for _, setting := range config.Settings {
			wg.Add(1)

			go func(setting ConfigSetting) {
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

				run(
					setting.ApiKey,
					setting.ApiSecret,
					setting.Symbol,
					setting.Quantity,
					setting.Total,
					setting.Reduce,
					setting.Arbitrage,
					setting.Difference,
					setting.Leverage,
					setting.BidSide,
				)
			}(setting)
		}

		wg.Wait()
	} else {
		run(
			*apiKey,
			*apiSecret,
			*symbol,
			*quantity,
			*total,
			*reduce,
			*arbitrage,
			*difference,
			*leverage,
			*bidSide,
		)
	}
}

func run(
	apiKey,
	apiSecret,
	symbol string,
	quantity,
	total float64,
	reduce,
	arbitrage bool,
	difference float64,
	leverage int,
	bidSide string,
) {
	logger := logrus.New().WithField("symbol", symbol)
	currentProgressBarTotal := 0
	totalQuantity := total
	quantityPerOrder := quantity
	progressBarTotal := int(totalQuantity / quantityPerOrder)

	if int(math.Mod(totalQuantity, quantityPerOrder)) > 0 {
		progressBarTotal += 1
	}

	maxProgressBar := progressBarTotal

	// initialize flag
	var direction *bool
	var fundingRateReverseMode bool
	var arbitrageDirection *bool
	var arbitrageTriggered bool

	// force set arbitrage=OFF
	if reduce {
		arbitrage = false
	}

	// initialize step
	step := 1

	if arbitrage {
		logger.Info("You're in arbitrage mode.")
		logger.Info("I'll help you place some orders.")
		logger.Info("Use reverse mode when differece +-0.08%.")
		logger.Info("Total quantity has been reset.")

		total = quantity
		difference = .08
		totalQuantity = total
	} else {
		logger.Info("I'm trying to place some orders...")
		logger.Info("Please be patient and keep waiting...")
	}

	for {
		if totalQuantity <= 0 && reduce && !arbitrage {
			break
		}

		// enable arbitrage mode
		if arbitrage && totalQuantity <= 0 {
			reduce = true
			totalQuantity = total
			arbitrageTriggered = true
		}

		if totalQuantity >= total {
			totalQuantity = total

			// create new bar
			if !fundingRateReverseMode {
				currentProgressBarTotal = 0
			}

			if currentProgressBarTotal >= maxProgressBar {
				fundingRateReverseMode = false
				step = 1

				maxProgressBar = progressBarTotal
			}
		}

		// update quantity per order
		if quantityPerOrder > totalQuantity {
			quantityPerOrder = totalQuantity
		} else {
			quantityPerOrder = quantity
		}

		hedge := make([]binance.BinanceHedge, 0)

		req := gorequest.New().Get("https://wiwisorich.capslock.tw")
		req.EndStruct(&hedge)

		for _, v := range hedge {
			if v.Symbol == symbol {
				markPriceDirection := binanceIndexDirection(v.Index)

				if arbitrage && difference > v.MarkPriceGap {
					break
				}

				if !arbitrage && v.MarkPriceGap > difference {
					break
				}

				if arbitrageDirection != nil && ((!arbitrageTriggered && *arbitrageDirection != markPriceDirection) || (arbitrageTriggered && *arbitrageDirection == markPriceDirection)) {
					break
				}

				// record arbitrage direction
				if arbitrage {
					if arbitrageDirection == nil {
						if markPriceDirection == v.Direction {
							break
						}

						arbitrageDirection = &markPriceDirection
					}

					v.Direction = *arbitrageDirection
				}

				if fundingRateReverseMode {
					v.Direction = !v.Direction
				}

				var usdtBid float64
				var usdtAsk float64
				var busdBid float64
				var busdAsk float64
				var usdtBidSize float64
				var usdtAskSize float64
				var busdBidSize float64
				var busdAskSize float64

				useLeverage := map[string]string{
					"leverage": fmt.Sprint(leverage),
				}

				wg := &sync.WaitGroup{}

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "USDT"
					fapi("/leverage", gorequest.POST, apiKey, apiSecret, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "BUSD"
					fapi("/leverage", gorequest.POST, apiKey, apiSecret, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					usdtBid, usdtBidSize, usdtAsk, usdtAskSize = getDepth(v.Symbol, "USDT")
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					busdBid, busdBidSize, busdAsk, busdAskSize = getDepth(v.Symbol, "BUSD")
				}()

				// wait sync group
				wg.Wait()

				rules := []bool{
					usdtBidSize > quantityPerOrder,
					busdBidSize > quantityPerOrder,
					usdtAskSize > quantityPerOrder,
					busdAskSize > quantityPerOrder,
				}

				if slices.Contains(rules, false) {
					break
				}

				// update var
				currentProgressBarTotal += 1

				// handle order
				// X-MBX-APIKEY
				if reduce {
					v.Direction = !v.Direction

					if bidSide == "BUSD" {
						v.Direction = false
					} else if bidSide == "USDT" {
						v.Direction = true
					}
				} else if direction == nil {
					direction = &v.Direction
				} else if *direction != v.Direction {
					if totalQuantity >= total {
						step = 1
					} else {
						step = -1
					}

					direction = &v.Direction

					// reset quantity
					quantityPerOrder = quantity

					logger.Info("direction changed, close orders...")

					// change max
					if currentProgressBarTotal > progressBarTotal {
						maxProgressBar = progressBarTotal
					} else {
						maxProgressBar = currentProgressBarTotal
					}

					currentProgressBarTotal = 0
				}

				binanceOrderBUSD := BinancePlaceOrder{
					Type:     "MARKET",
					Symbol:   v.Symbol + "BUSD",
					Quantity: decimal.NewFromFloat(quantityPerOrder).String(),
				}
				binanceOrderUSDT := BinancePlaceOrder{
					Type:     "MARKET",
					Symbol:   v.Symbol + "USDT",
					Quantity: decimal.NewFromFloat(quantityPerOrder).String(),
				}

				logger.Info("USDT BID=", usdtBid)
				logger.Info("USDT ASK=", usdtAsk)
				logger.Info("BUSD BID=", busdBid)
				logger.Info("BUSD ASK=", busdAsk)

				if arbitrageDirection == nil {
					if v.Direction {
						binanceOrderBUSD.Side = "BUY"
						binanceOrderUSDT.Side = "SELL"
					} else {
						binanceOrderBUSD.Side = "SELL"
						binanceOrderUSDT.Side = "BUY"
					}
				} else {
					if v.Direction {
						binanceOrderBUSD.Side = "SELL"
						binanceOrderUSDT.Side = "BUY"
					} else {
						binanceOrderBUSD.Side = "BUY"
						binanceOrderUSDT.Side = "SELL"
					}
				}

				if reduce || fundingRateReverseMode {
					binanceOrderBUSD.ReduceOnly = "true"
					binanceOrderUSDT.ReduceOnly = "true"
				}

				orders := make([]BinancePlaceOrder, 0)
				orders = append(orders, binanceOrderBUSD)
				orders = append(orders, binanceOrderUSDT)

				// place binance order
				if totalQuantity > 0 {
					batchOrders, _ := json.Marshal(orders)

					logger.Info(string(batchOrders))
					fapi(
						"/batchOrders",
						gorequest.POST,
						apiKey,
						apiSecret,
						map[string]string{
							"batchOrders": string(batchOrders),
						},
					).End()
				}

				// update total
				value := decimal.
					NewFromFloat(quantityPerOrder).
					Mul(decimal.NewFromInt(int64(step)))

				// calculate totalQuantity
				totalQuantity, _ = decimal.
					NewFromFloat(totalQuantity).
					Sub(value).
					Float64()

				// exit loop
				break
			}
		}

		time.Sleep(1 * time.Second)
	}
}

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/parnurzeal/gorequest"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"

	binance "github.com/CapsLock-Studio/binance-premium-index/models"
)

const (
	BINANCE_FAPI_ENDPOINT     string = "https://fapi.binance.com/fapi/v1"
	BINANCE_FAPI_LEVERAGE     string = "/leverage"
	BINANCE_FAPI_BATCH_ORDERS string = "/batchOrders"
	BINANCE_FAPI_DEPTH        string = "/depth"
	BINANCE_FAPI_OPEN_ORDERS  string = "/positionRisk"

	FUNDING_RATE_ENDPOINT string = "https://wiwisorich.capslock.tw"
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

type BinanceOrder struct {
	Symbol       string `json:"symbol"`
	PositionSide string `json:"positionSide"`
	PositionAmt  string `json:"positionAmt"`
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
		BINANCE_FAPI_DEPTH,
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

		for key, value := range body {
			params.Add(key, value)
		}

		params.Add("timestamp", decimal.NewFromInt(time.Now().UnixMilli()).String())

		mac := hmac.New(sha256.New, []byte(apiSecret))
		mac.Write([]byte(params.Encode()))
		signingKey := fmt.Sprintf("%x", mac.Sum(nil))

		params.Add("signature", signingKey)

		path += "?" + params.Encode()
	}

	req := gorequest.
		New().
		CustomMethod(method, BINANCE_FAPI_ENDPOINT+path)

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

func httpServer() {
	ch := make(chan string, 10)
	route := gin.Default()

	route.POST("/", func(ctx *gin.Context) {
		var r ConfigSetting
		if ctx.Bind(&r) != nil {
			return
		}

		ID := uuid.New().String()

		go func(r ConfigSetting) {
			run(
				r.ApiKey,
				r.ApiSecret,
				r.Symbol,
				r.Quantity,
				r.Total,
				r.Reduce,
				r.Arbitrage,
				r.Difference,
				r.Leverage,
				r.BidSide,
				r.Monitor,
				ch,
				&ID,
			)
		}(r)

		ctx.Data(http.StatusOK, "text/plain", []byte(ID))
	})

	route.DELETE("/:id", func(ctx *gin.Context) {
		ID := ctx.Param("id")

		ch <- ID
		ctx.Data(http.StatusOK, "text/plain", []byte("DONE"))
	})

	route.Run()
}

func readConfig(path string) {
	filename, _ := filepath.Abs(path)
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
				setting.Monitor,
				nil,
				nil,
			)
		}(setting)
	}

	wg.Wait()
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
	monitor := flag.Bool("monitor", false, "assume you have positions on binance")
	serve := flag.Bool("serve", false, "serve in http mode")
	flag.Parse()

	if *serve {
		httpServer()
	} else if *config != "" {
		readConfig(*config)
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
			*monitor,
			nil,
			nil,
		)
	}
}

func getMaxProgressBar(totalQuantity, quantityPerOrder float64) int {
	progressBarTotal := int(totalQuantity / quantityPerOrder)

	if int(math.Mod(totalQuantity, quantityPerOrder)) > 0 {
		progressBarTotal += 1
	}

	return progressBarTotal
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
	monitor bool,
	ch chan string,
	ID *string,
) {
	logger := logrus.New().WithField("symbol", symbol)
	currentProgressBarTotal := 0
	totalQuantity := total
	quantityPerOrder := quantity
	progressBarTotal := getMaxProgressBar(totalQuantity, quantityPerOrder)

	maxProgressBar := progressBarTotal

	// initialize flag
	var currentDirection *bool
	var fundingRateReverseMode bool
	var arbitrageDirection *bool
	var arbitrageTriggered bool

	// force set arbitrage=OFF
	if reduce {
		arbitrage = false
	} else if monitor {
		openPositionForUSDT := make([]BinanceOrder, 0)
		openPositionForBUSD := make([]BinanceOrder, 0)
		wg := &sync.WaitGroup{}

		wg.Add(1)
		go func() {
			defer wg.Done()
			fapi(
				BINANCE_FAPI_OPEN_ORDERS,
				gorequest.GET,
				apiKey,
				apiSecret,
				map[string]string{
					"symbol":     symbol + "USDT",
					"recvWindow": "5000",
				},
			).EndStruct(&openPositionForUSDT)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			fapi(
				BINANCE_FAPI_OPEN_ORDERS,
				gorequest.GET,
				apiKey,
				apiSecret,
				map[string]string{
					"symbol":     symbol + "BUSD",
					"recvWindow": "5000",
				},
			).EndStruct(&openPositionForBUSD)
		}()

		wg.Wait()

		if len(openPositionForBUSD) > 0 && len(openPositionForUSDT) > 0 {
			openQtyForBUSD, _ := decimal.NewFromString(openPositionForBUSD[0].PositionAmt)
			openQtyForUSDT, _ := decimal.NewFromString(openPositionForUSDT[0].PositionAmt)

			openQty, _ := decimal.Min(openQtyForBUSD.Abs(), openQtyForUSDT.Abs()).Float64()

			direction := openQtyForBUSD.GreaterThan(decimal.NewFromInt(0))

			currentDirection = &direction
			currentProgressBarTotal = maxProgressBar - int(openQty/quantityPerOrder)

			if currentProgressBarTotal < 0 {
				currentProgressBarTotal = 0
			}

			totalQuantity, _ = decimal.
				NewFromFloat(totalQuantity).
				Sub(decimal.NewFromFloat(openQty)).
				Float64()
		}
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
		if ch != nil && len(ch) > 0 {
			logrus.Info("Check channel...")
			buffered := <-ch

			if buffered == *ID {
				logger.Info("Receive close signal...")
				break
			}

			logger.Info("Send to channel again")
			ch <- buffered
		}

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

		// fetch hedge information
		gorequest.
			New().
			Get(FUNDING_RATE_ENDPOINT).
			EndStruct(&hedge)

		for _, v := range hedge {
			if v.Symbol == symbol {
				markPriceDirection := binanceIndexDirection(v.Index)

				logger.Info("MarkPriceGap=", v.MarkPriceGap)

				if arbitrage && difference > v.MarkPriceGap {
					break
				}

				if !arbitrage && v.MarkPriceGap > difference {
					break
				}

				if arbitrageDirection != nil && ((!arbitrageTriggered && *arbitrageDirection != markPriceDirection) || (arbitrageTriggered && *arbitrageDirection == markPriceDirection)) {
					break
				}

				if currentDirection != nil && v.Direction != *currentDirection {
					fundingRateReverseMode = true
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
					fapi(BINANCE_FAPI_LEVERAGE, gorequest.POST, apiKey, apiSecret, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "BUSD"
					fapi(BINANCE_FAPI_LEVERAGE, gorequest.POST, apiKey, apiSecret, useLeverage).End()
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
				if reduce {
					v.Direction = !v.Direction

					if bidSide == "BUSD" {
						v.Direction = false
					} else if bidSide == "USDT" {
						v.Direction = true
					}
				} else if currentDirection == nil {
					currentDirection = &v.Direction
				} else if *currentDirection != v.Direction {
					if totalQuantity >= total {
						step = 1
					} else {
						step = -1
					}

					currentDirection = &v.Direction

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

				perQuantity := decimal.NewFromFloat(quantityPerOrder).String()

				binanceOrderBUSD := BinancePlaceOrder{
					Type:     "MARKET",
					Symbol:   v.Symbol + "BUSD",
					Quantity: perQuantity,
				}
				binanceOrderUSDT := BinancePlaceOrder{
					Type:     "MARKET",
					Symbol:   v.Symbol + "USDT",
					Quantity: perQuantity,
				}

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
					logger.Info("USDT BID=", usdtBid)
					logger.Info("USDT ASK=", usdtAsk)
					logger.Info("BUSD BID=", busdBid)
					logger.Info("BUSD ASK=", busdAsk)

					batchOrders, _ := json.Marshal(orders)

					logger.Info(string(batchOrders))
					fapi(
						BINANCE_FAPI_BATCH_ORDERS,
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

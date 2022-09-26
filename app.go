package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/parnurzeal/gorequest"
	"github.com/schollz/progressbar/v3"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/thoas/go-funk"

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

func getDepth(
	symbol string,
	currency string,
) (
	bidSize float64,
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
	flag.Parse()

	totalQuantity := *total
	quantityPerOrder := *quantity
	progressBarTotal := int(totalQuantity / quantityPerOrder)

	if int(math.Mod(totalQuantity, quantityPerOrder)) > 0 {
		progressBarTotal += 1
	}

	currentProgressBarTotal := 0

	// initialize bar
	bar := progressbar.NewOptions(progressBarTotal, progressbar.OptionSetWidth(30))
	manualFundingRateReverseMode := *reduce
	arbitrageFundingRateDifference := *difference
	arbitrageAutoMode := *arbitrage

	// initialize flag
	var direction *bool
	var fundingRateReverseMode bool
	var arbitrageDirection *bool
	var arbitrageTriggered bool

	// force set arbitrage=OFF
	if manualFundingRateReverseMode {
		arbitrageAutoMode = false
	}

	// initialize step
	step := 1

	if arbitrageAutoMode {
		logrus.Info("You're in arbitrage mode.")
		logrus.Info("I'll help you place some orders and use reverse mode when differece +0.08%.")
	} else {
		logrus.Info("I'm trying to place some orders...")
		logrus.Info("Please be patient and keep waiting...")
	}

	for {
		if totalQuantity <= 0 && manualFundingRateReverseMode {
			break
		}

		// enable arbitrage mode
		if arbitrageAutoMode && totalQuantity <= 0 {
			arbitrageFundingRateDifference, _ = decimal.NewFromFloat(*difference).Add(decimal.NewFromFloat(.08)).Float64()

			manualFundingRateReverseMode = true
			totalQuantity = *total
			arbitrageTriggered = true
		}

		if totalQuantity >= *total {
			totalQuantity = *total

			// create new bar
			if !fundingRateReverseMode {
				bar.Reset()

				currentProgressBarTotal = 0
			}

			if bar.IsFinished() {
				fundingRateReverseMode = false
				step = 1

				bar.ChangeMax(progressBarTotal)
			}
		}

		// update quantity per order
		if quantityPerOrder > totalQuantity {
			quantityPerOrder = totalQuantity
		} else {
			quantityPerOrder = *quantity
		}

		hedge := make([]binance.BinanceHedge, 0)

		req := gorequest.New().Get("https://wiwisorich.capslock.tw")
		req.EndStruct(&hedge)

		for _, v := range hedge {
			if v.Symbol == *symbol {
				markPriceDirection := binanceIndexDirection(v.Index)

				conditions := []bool{
					v.MarkPriceGap > arbitrageFundingRateDifference,
					// not triggered, skip when direction is not as same as old one
					// triggered, skip when direction is as same as old one
					arbitrageDirection != nil && ((!arbitrageTriggered && *arbitrageDirection != markPriceDirection) || (arbitrageTriggered && *arbitrageDirection == markPriceDirection)),
				}

				if funk.Every(conditions, true) {
					break
				}

				// record arbitrage direction
				if arbitrageAutoMode && arbitrageDirection == nil {
					arbitrageDirection = &markPriceDirection
				}

				if fundingRateReverseMode {
					v.Direction = !v.Direction
				}

				var usdtBidSize float64
				var usdtAskSize float64
				var busdBidSize float64
				var busdAskSize float64

				useLeverage := map[string]string{
					"leverage": fmt.Sprint(*leverage),
				}

				wg := &sync.WaitGroup{}

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "USDT"
					fapi("/leverage", gorequest.POST, *apiKey, *apiSecret, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "BUSD"
					fapi("/leverage", gorequest.POST, *apiKey, *apiSecret, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					usdtBidSize, usdtAskSize = getDepth(v.Symbol, "USDT")
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					busdBidSize, busdAskSize = getDepth(v.Symbol, "BUSD")
				}()

				// wait sync group
				wg.Wait()

				rules := []bool{
					usdtBidSize > quantityPerOrder,
					busdBidSize > quantityPerOrder,
					usdtAskSize > quantityPerOrder,
					busdAskSize > quantityPerOrder,
				}

				if funk.Contains(rules, false) {
					break
				}

				// add bar status
				bar.Add(1)

				// handle order
				// X-MBX-APIKEY
				if manualFundingRateReverseMode {
					v.Direction = !v.Direction
				} else if direction == nil {
					direction = &v.Direction
				} else if *direction != v.Direction {
					if totalQuantity >= *total {
						step = 1
					} else {
						step = -1
					}

					direction = &v.Direction

					// reset quantity
					quantityPerOrder = *quantity

					fmt.Println()
					logrus.Info("direction changed, close orders...")
					bar.Reset()

					// change max
					if currentProgressBarTotal > progressBarTotal {
						bar.ChangeMax(progressBarTotal)
					} else {
						bar.ChangeMax(currentProgressBarTotal)
					}

					currentProgressBarTotal = 0
				}

				// update var
				currentProgressBarTotal += 1

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

				if v.Direction {
					binanceOrderBUSD.Side = "BUY"
					binanceOrderUSDT.Side = "SELL"
				} else {
					binanceOrderBUSD.Side = "SELL"
					binanceOrderUSDT.Side = "BUY"
				}

				if manualFundingRateReverseMode || fundingRateReverseMode {
					binanceOrderBUSD.ReduceOnly = "true"
					binanceOrderUSDT.ReduceOnly = "true"
				}

				orders := make([]BinancePlaceOrder, 0)
				orders = append(orders, binanceOrderBUSD)
				orders = append(orders, binanceOrderUSDT)

				// place binance order
				if totalQuantity > 0 {
					batchOrders, _ := json.Marshal(orders)

					fapi(
						"/batchOrders",
						gorequest.POST,
						*apiKey,
						*apiSecret,
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

	fmt.Println()
}

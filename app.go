package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/url"
	"strconv"
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
	Type     string `json:"type"`
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	Quantity string `json:"quantity"`
}

func getDepth(
	symbol string,
	currency string,
) (
	bidSize float64,
	askSize float64,
) {
	// check depth
	params := url.Values{}
	params.Add("limit", "5")
	params.Add("symbol", symbol+currency)

	bidAndAsk := struct {
		Asks [][]string `json:"asks"`
		Bids [][]string `json:"bids"`
	}{}

	fapi(
		"/depth?"+params.Encode(),
		gorequest.GET,
		"",
		nil,
	).EndStruct(&bidAndAsk)

	bidSize, _ = strconv.ParseFloat(bidAndAsk.Bids[len(bidAndAsk.Bids)-1][1], 64)
	askSize, _ = strconv.ParseFloat(bidAndAsk.Asks[len(bidAndAsk.Asks)-1][1], 64)

	return
}

func fapi(
	path,
	method,
	apiKey string,
	body interface{},
) *gorequest.SuperAgent {
	req := gorequest.
		New().
		CustomMethod(method, "https://fapi.binance.com/fapi/v1"+path)

	req.Header.Set("X-MBX-APIKEY", apiKey)

	if body != nil {
		data, _ := json.Marshal(body)
		req.Send(string(data))
	}

	return req
}

func main() {
	apiKey := flag.String("apiKey", "", "binance api key")
	symbol := flag.String("symbol", "", "binance future symbol")
	quantity := flag.Float64("quantity", 0, "quantity per order")
	total := flag.Float64("total", 0, "total quantity")
	reduce := flag.Bool("reduce", false, "use reduce mode")
	difference := flag.Float64("difference", .05, "BUSD & USDT difference")
	leverage := flag.Int("leverage", 10, "futures leverage")
	flag.Parse()

	totalQuantity := *total
	quantityPerOrder := *quantity
	progressBarTotal := int(totalQuantity / quantityPerOrder)

	if int(math.Mod(totalQuantity, quantityPerOrder)) > 0 {
		progressBarTotal += 1
	}

	// initialize bar
	bar := &progressbar.ProgressBar{}

	// initialize flag
	var direction *bool

	var fundingRateReverseMode bool

	// initialize step
	step := 1

	logrus.Info("I'm trying to place some orders...")
	logrus.Info("Please be patient and keep waiting...")

	for {
		if totalQuantity <= 0 && *reduce {
			break
		}

		if totalQuantity >= *total {
			totalQuantity = *total

			// create new bar
			if !fundingRateReverseMode {
				bar = progressbar.NewOptions(progressBarTotal, progressbar.OptionSetWidth(30))
			}

			fundingRateReverseMode = false
			step = 1
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
				if v.MarkPriceGap > *difference {
					break
				}

				if fundingRateReverseMode {
					v.Direction = !v.Direction
				}

				var usdtBidSize float64
				var usdtAskSize float64
				var busdBidSize float64
				var busdAskSize float64

				useLeverage := map[string]interface{}{
					"leverage": *leverage,
				}

				wg := sync.WaitGroup{}

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "USDT"
					fapi("/leverage", gorequest.POST, *apiKey, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "BUSD"
					fapi("/leverage", gorequest.POST, *apiKey, useLeverage).End()
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

				// handle order
				// X-MBX-APIKEY
				// TODO
				if *reduce {
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
					bar = progressbar.NewOptions(progressBarTotal, progressbar.OptionSetWidth(30))
				}

				binanceOrderBUSD := BinancePlaceOrder{
					Type:   "MARKET",
					Symbol: v.Symbol + "BUSD",
				}
				binanceOrderUSDT := BinancePlaceOrder{
					Type:   "MARKET",
					Symbol: v.Symbol + "USDT",
				}

				if v.Direction {
					binanceOrderBUSD.Side = "BUY"
					binanceOrderUSDT.Side = "SELL"
				} else {
					binanceOrderBUSD.Side = "SELL"
					binanceOrderUSDT.Side = "BUY"
				}

				binanceOrderUSDT.Quantity = decimal.NewFromFloat(quantityPerOrder).String()
				binanceOrderBUSD.Quantity = decimal.NewFromFloat(quantityPerOrder).String()

				orders := make([]BinancePlaceOrder, 0)
				orders = append(orders, binanceOrderBUSD)
				orders = append(orders, binanceOrderUSDT)

				// place binance order
				if totalQuantity > 0 {
					fapi(
						"/batchOrders",
						gorequest.POST,
						*apiKey,
						orders,
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

				// add bar status
				bar.Add(1)

				// exit loop
				break
			}
		}

		time.Sleep(1 * time.Second)
	}
}

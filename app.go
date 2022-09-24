package main

import (
	"encoding/json"
	"flag"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/thoas/go-funk"

	binance "github.com/CapsLock-Studio/binance-premium-index/models"
)

func getDepth(
	symbol string,
	currency string,
) (
	bidSize float64,
	askSize float64,
) {
	depthURL, err := url.Parse("https://fapi.binance.com/fapi/v1/depth")
	if err != nil {
		return
	}

	// check depth
	params := url.Values{}
	params.Add("limit", "5")
	params.Add("symbol", symbol+currency)
	depthURL.RawQuery = params.Encode()

	res, err := http.Get(depthURL.String())
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	bidAndAsk := struct {
		Asks [][]string `json:"asks"`
		Bids [][]string `json:"bids"`
	}{}
	decoder := json.NewDecoder(res.Body)

	decoder.Decode(&bidAndAsk)

	bidSize, _ = strconv.ParseFloat(bidAndAsk.Bids[len(bidAndAsk.Bids)-1][1], 64)
	askSize, _ = strconv.ParseFloat(bidAndAsk.Asks[len(bidAndAsk.Asks)-1][1], 64)

	return
}

func main() {
	// key := flag.String("key", "", "binance api key")
	symbol := flag.String("symbol", "", "binance future symbol")
	quantity := flag.Float64("quantity", 0, "quantity per order")
	total := flag.Float64("total", 0, "total quantity")
	difference := flag.Float64("difference", .05, "BUSD & USDT difference")
	flag.Parse()

	totalQuantity := *total
	quantityPerOrder := *quantity
	progressBarTotal := int64(totalQuantity / quantityPerOrder)

	if math.Mod(totalQuantity, quantityPerOrder) > 0 {
		progressBarTotal += 1
	}

	// initialize bar
	bar := progressbar.Default(progressBarTotal)

	for {
		if totalQuantity <= 0 {
			break
		}

		// update quantity per order
		if quantityPerOrder > float64(totalQuantity) {
			quantityPerOrder = float64(totalQuantity)
		}

		res, err := http.Get("https://wiwisorich.capslock.tw")
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()

		hedge := make([]binance.BinanceHedge, 0)
		decoder := json.NewDecoder(res.Body)

		decoder.Decode(&hedge)

		for _, v := range hedge {
			if v.Symbol == *symbol {
				if v.MarkPriceGap > *difference {
					break
				}

				var usdtBidSize float64
				var usdtAskSize float64
				var busdBidSize float64
				var busdAskSize float64

				wg := sync.WaitGroup{}

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

				// update total
				totalQuantity -= quantityPerOrder

				// add bar status
				bar.Add(1)

				// exit loop
				break
			}
		}

		time.Sleep(1 * time.Second)
	}

	// done
}

package modules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CapsLock-Studio/binance-premium-bot/models"
	"github.com/parnurzeal/gorequest"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"
	"golang.org/x/exp/slices"

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

type Core struct {
	ApiKey      string
	ApiSecret   string
	Symbol      string
	Quantity    float64
	Total       float64
	Reduce      bool
	Arbitrage   bool
	Difference  float64
	Leverage    int
	BidSide     string
	Monitor     bool
	Channel     chan string
	ID          *string
	RateLimiter ratelimit.Limiter
}

func NewCore(setting models.ConfigSetting, channel chan string, ID *string, rl ratelimit.Limiter) *Core {
	return &Core{
		ApiKey:      setting.ApiKey,
		ApiSecret:   setting.ApiSecret,
		Symbol:      setting.Symbol,
		Quantity:    setting.Quantity,
		Total:       setting.Total,
		Reduce:      setting.Reduce,
		Arbitrage:   setting.Arbitrage,
		Difference:  setting.Difference,
		Leverage:    setting.Leverage,
		BidSide:     setting.BidSide,
		Monitor:     setting.Monitor,
		Channel:     channel,
		ID:          ID,
		RateLimiter: rl,
	}
}

func (c *Core) GetDepth(
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

	c.MakeRequest(
		BINANCE_FAPI_DEPTH,
		gorequest.GET,
		map[string]string{
			"limit":  "5",
			"symbol": c.Symbol + currency,
		},
	).EndStruct(&bidAndAsk)

	if len(bidAndAsk.Bids) > 0 && len(bidAndAsk.Asks) > 0 {
		bid, _ = strconv.ParseFloat(bidAndAsk.Bids[len(bidAndAsk.Bids)-1][0], 64)
		ask, _ = strconv.ParseFloat(bidAndAsk.Asks[len(bidAndAsk.Asks)-1][0], 64)
		bidSize, _ = strconv.ParseFloat(bidAndAsk.Bids[len(bidAndAsk.Bids)-1][1], 64)
		askSize, _ = strconv.ParseFloat(bidAndAsk.Asks[len(bidAndAsk.Asks)-1][1], 64)
	}

	return
}

func (c *Core) MakeRequest(
	path,
	method string,
	body map[string]string,
) *gorequest.SuperAgent {
	if body != nil {
		params := url.Values{}

		for key, value := range body {
			params.Add(key, value)
		}

		params.Add("timestamp", decimal.NewFromInt(time.Now().UnixMilli()).String())

		mac := hmac.New(sha256.New, []byte(c.ApiSecret))
		mac.Write([]byte(params.Encode()))
		signingKey := fmt.Sprintf("%x", mac.Sum(nil))

		params.Add("signature", signingKey)

		path += "?" + params.Encode()
	}

	req := gorequest.
		New().
		CustomMethod(method, BINANCE_FAPI_ENDPOINT+path)

	req.Header.Set("X-MBX-APIKEY", c.ApiKey)

	return req
}

func (c *Core) Run() {
	logger := logrus.
		New().
		WithField("symbol", c.Symbol).
		WithField("leverage", c.Leverage)

	// add key information
	if len(c.ApiKey) > 5 {
		logger = logger.WithField("key", c.ApiKey[0:5])
	}

	currentProgressBarTotal := 0
	totalQuantity := c.Total
	quantityPerOrder := c.Quantity
	progressBarTotal := getMaxProgressBar(totalQuantity, quantityPerOrder)

	maxProgressBar := progressBarTotal

	// initialize flag
	var currentDirection *bool
	var fundingRateReverseMode bool
	var arbitrageDirection *bool
	var arbitrageTriggered bool

	// force set arbitrage=OFF
	if c.Reduce {
		c.Arbitrage = false
	} else if c.Monitor {
		openPositionForUSDT := make([]models.BinanceOrder, 0)
		openPositionForBUSD := make([]models.BinanceOrder, 0)
		wg := &sync.WaitGroup{}

		wg.Add(1)
		go func() {
			defer wg.Done()
			c.MakeRequest(
				BINANCE_FAPI_OPEN_ORDERS,
				gorequest.GET,
				map[string]string{
					"symbol":     c.Symbol + "USDT",
					"recvWindow": "5000",
				},
			).EndStruct(&openPositionForUSDT)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			c.MakeRequest(
				BINANCE_FAPI_OPEN_ORDERS,
				gorequest.GET,
				map[string]string{
					"symbol":     c.Symbol + "BUSD",
					"recvWindow": "5000",
				},
			).EndStruct(&openPositionForBUSD)
		}()

		wg.Wait()

		if len(openPositionForBUSD) > 0 && len(openPositionForUSDT) > 0 {
			openQtyForBUSD, _ := decimal.NewFromString(openPositionForBUSD[0].PositionAmt)
			openQtyForUSDT, _ := decimal.NewFromString(openPositionForUSDT[0].PositionAmt)

			openQty, _ := decimal.Min(openQtyForBUSD.Abs(), openQtyForUSDT.Abs()).Float64()

			if openQty > 0 {
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
	}

	// initialize step
	step := 1

	if c.Arbitrage {
		logger.Info("You're in arbitrage mode.")
		logger.Info("I'll help you place some orders.")
		logger.Info("Use reverse mode when differece +-0.08%.")
		logger.Info("Total quantity has been reset.")

		c.Total = c.Quantity
		c.Difference = .08
		totalQuantity = c.Total
	} else {
		logger.Info("I'm trying to place some orders...")
		logger.Info("Please be patient and keep waiting...")
	}

	for {
		// wait 1 seconds
		c.RateLimiter.Take()

		if c.Channel != nil && len(c.Channel) > 0 {
			logrus.Info("Check channel...")
			buffered := <-c.Channel

			if buffered == *c.ID {
				logger.Info("Receive close signal...")
				break
			}

			logger.Info("Send to channel again")
			c.Channel <- buffered
		}

		if totalQuantity <= 0 && c.Reduce && !c.Arbitrage {
			break
		}

		// enable arbitrage mode
		if c.Arbitrage && totalQuantity <= 0 {
			c.Reduce = true
			totalQuantity = c.Total
			arbitrageTriggered = true
		}

		if totalQuantity >= c.Total {
			totalQuantity = c.Total

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
			quantityPerOrder = c.Quantity
		}

		hedge := make([]binance.BinanceHedge, 0)

		// fetch hedge information
		gorequest.
			New().
			Get(FUNDING_RATE_ENDPOINT).
			EndStruct(&hedge)

		for _, v := range hedge {
			if v.Symbol == c.Symbol {
				markPriceDirection := binanceIndexDirection(v.Index)

				logger.Info("MarkPriceGap=", v.MarkPriceGap)

				if c.Arbitrage && c.Difference > v.MarkPriceGap {
					break
				}

				if !c.Arbitrage {
					if v.FundingRateGap == 0 {
						continue
					}

					if v.MarkPriceGap > c.Difference {
						break
					}
				}

				if arbitrageDirection != nil && ((!arbitrageTriggered && *arbitrageDirection != markPriceDirection) || (arbitrageTriggered && *arbitrageDirection == markPriceDirection)) {
					break
				}

				if currentDirection != nil && v.Direction != *currentDirection {
					fundingRateReverseMode = true
				}

				// record arbitrage direction
				if c.Arbitrage {
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
					"leverage": fmt.Sprint(c.Leverage),
				}

				wg := &sync.WaitGroup{}

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "USDT"
					c.MakeRequest(BINANCE_FAPI_LEVERAGE, gorequest.POST, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()

					useLeverage["symbol"] = v.Symbol + "BUSD"
					c.MakeRequest(BINANCE_FAPI_LEVERAGE, gorequest.POST, useLeverage).End()
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					usdtBid, usdtBidSize, usdtAsk, usdtAskSize = c.GetDepth("USDT")
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					busdBid, busdBidSize, busdAsk, busdAskSize = c.GetDepth("BUSD")
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
				if c.Reduce {
					v.Direction = !v.Direction

					if c.BidSide == "BUSD" {
						v.Direction = false
					} else if c.BidSide == "USDT" {
						v.Direction = true
					}
				} else if currentDirection == nil {
					currentDirection = &v.Direction
				} else if *currentDirection != v.Direction {
					if totalQuantity >= c.Total {
						step = 1
					} else {
						step = -1
					}

					currentDirection = &v.Direction

					// reset quantity
					quantityPerOrder = c.Quantity

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

				binanceOrderBUSD := models.BinancePlaceOrder{
					Type:     "MARKET",
					Symbol:   v.Symbol + "BUSD",
					Quantity: perQuantity,
				}
				binanceOrderUSDT := models.BinancePlaceOrder{
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

				if c.Reduce || fundingRateReverseMode {
					binanceOrderBUSD.ReduceOnly = "true"
					binanceOrderUSDT.ReduceOnly = "true"
				}

				orders := make([]models.BinancePlaceOrder, 0)
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
					c.MakeRequest(
						BINANCE_FAPI_BATCH_ORDERS,
						gorequest.POST,
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

func getMaxProgressBar(totalQuantity, quantityPerOrder float64) int {
	progressBarTotal := int(totalQuantity / quantityPerOrder)

	if int(math.Mod(totalQuantity, quantityPerOrder)) > 0 {
		progressBarTotal += 1
	}

	return progressBarTotal
}

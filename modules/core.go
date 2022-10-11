package modules

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
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

	DEFAULT_LEVERAGE   int     = 10
	DEFAULT_DIFFERENCE float64 = 0.05
	DEFAULT_MINUTES    float64 = 480
)

type Core struct {
	Setting        *models.ConfigSetting
	EventReceiver  chan string
	ID             *string
	RateLimiter    ratelimit.Limiter
	EventPublisher chan models.EventMessage
}

func NewCore(
	setting *models.ConfigSetting,
	eventReceiver chan string,
	ID *string,
	ratelimiter ratelimit.Limiter,
) *Core {
	return &Core{
		Setting:        setting,
		EventReceiver:  eventReceiver,
		ID:             ID,
		RateLimiter:    ratelimiter,
		EventPublisher: make(chan models.EventMessage),
	}
}

func (c *Core) GetPublisher() <-chan models.EventMessage {
	return c.EventPublisher
}

func (c *Core) GetDepth(
	currency string,
) (
	bid,
	bidSize,
	ask,
	askSize float64,
) {
	depth := struct {
		Asks [][]string `json:"asks"`
		Bids [][]string `json:"bids"`
	}{}

	c.MakeRequest(
		BINANCE_FAPI_DEPTH,
		gorequest.GET,
		map[string]string{
			"limit":  "5",
			"symbol": c.Setting.Symbol + currency,
		},
	).EndStruct(&depth)

	if len(depth.Bids) > 0 && len(depth.Asks) > 0 {
		bid, _ = strconv.ParseFloat(depth.Bids[len(depth.Bids)-1][0], 64)
		ask, _ = strconv.ParseFloat(depth.Asks[len(depth.Asks)-1][0], 64)
		bidSize, _ = strconv.ParseFloat(depth.Bids[len(depth.Bids)-1][1], 64)
		askSize, _ = strconv.ParseFloat(depth.Asks[len(depth.Asks)-1][1], 64)
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

		mac := hmac.New(sha256.New, []byte(c.Setting.ApiSecret))
		mac.Write([]byte(params.Encode()))
		signingKey := fmt.Sprintf("%x", mac.Sum(nil))

		params.Add("signature", signingKey)

		path += "?" + params.Encode()
	}

	req := gorequest.
		New().
		CustomMethod(method, BINANCE_FAPI_ENDPOINT+path)

	req.Header.Set("X-MBX-APIKEY", c.Setting.ApiKey)

	return req
}

func (c *Core) Run() {
	go func() {
		for v := range c.GetPublisher() {
			if v.Setting.Webhook != "" {
				var ID string

				if c.ID != nil {
					ID = *c.ID
				}

				gorequest.
					New().
					Post(v.Setting.Webhook).
					Send(map[string]any{
						"type":    v.Type,
						"id":      ID,
						"symbol":  v.Setting.Symbol,
						"message": v.Message,
					})
			}
		}
	}()

	logger := logrus.
		New().
		WithField("symbol", c.Setting.Symbol).
		WithField("leverage", c.Setting.Leverage)

	// add key information
	if len(c.Setting.ApiKey) > 5 {
		logger = logger.WithField("key", c.Setting.ApiKey[0:5])
	}

	if c.ID != nil {
		logger = logger.WithField("id", *c.ID)
	}

	c.EventPublisher <- models.EventMessage{Type: "create", Setting: c.Setting}

	currentProgressBarTotal := 0
	totalQuantity := c.Setting.Total
	quantityPerOrder := c.Setting.Quantity
	progressBarTotal := int(totalQuantity / quantityPerOrder)

	if int(math.Mod(totalQuantity, quantityPerOrder)) > 0 {
		progressBarTotal += 1
	}

	maxProgressBar := progressBarTotal

	// initialize flag
	var currentDirection *bool
	var fundingRateReverseMode bool
	var arbitrageDirection *bool
	var arbitrageTriggered bool

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
				"symbol":     c.Setting.Symbol + "USDT",
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
				"symbol":     c.Setting.Symbol + "BUSD",
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

			if !c.Setting.Reduce {
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

	// force set arbitrage=OFF
	if c.Setting.Reduce {
		c.Setting.Arbitrage = false
	}

	// initialize step
	step := 1

	if c.Setting.Arbitrage {
		logger.Info("You're in arbitrage mode.")
		logger.Info("I'll help you place some orders.")
		logger.Info("Use reverse mode when differece +-0.08%.")
		logger.Info("Total quantity has been reset.")

		c.Setting.Total = c.Setting.Quantity
		c.Setting.Difference = .08
		totalQuantity = c.Setting.Total
	} else {
		logger.Info("I'm trying to place some orders...")
		logger.Info("Please be patient and keep waiting...")
	}

	for {
		// wait 1 seconds
		c.RateLimiter.Take()

		if c.EventReceiver != nil && len(c.EventReceiver) > 0 {
			logrus.Info("Check channel...")
			buffered := <-c.EventReceiver

			if buffered == *c.ID {
				logger.Info("Receive close signal...")
				break
			}

			logger.Info("Send to channel again")
			c.EventReceiver <- buffered
		}

		if totalQuantity < 0 {
			totalQuantity = 0
		}

		if totalQuantity <= 0 && c.Setting.Reduce && !c.Setting.Arbitrage {
			break
		}

		// enable arbitrage mode
		if c.Setting.Arbitrage && totalQuantity <= 0 {
			c.Setting.Reduce = true
			totalQuantity = c.Setting.Total
			arbitrageTriggered = true
		}

		if totalQuantity >= c.Setting.Total {
			totalQuantity = c.Setting.Total

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
			quantityPerOrder = c.Setting.Quantity
		}

		hedge := make([]binance.BinanceHedge, 0)

		// fetch hedge information
		gorequest.
			New().
			Get(FUNDING_RATE_ENDPOINT).
			EndStruct(&hedge)

		for _, v := range hedge {
			if v.Symbol == c.Setting.Symbol {
				markPriceDirection := v.GetPrice("USDT") > v.GetPrice("BUSD")

				logger.Info("MarkPriceGap=", v.MarkPriceGap)

				if c.Setting.Arbitrage && c.Setting.Difference > v.MarkPriceGap {
					break
				}

				if !c.Setting.Arbitrage {
					if v.FundingRateGap == 0 && !c.Setting.Reduce {
						continue
					}

					if v.MarkPriceGap > c.Setting.Difference {
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
				if c.Setting.Arbitrage {
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

				logger.Info("ask bid & ask depth...")

				useLeverage := map[string]string{
					"leverage": fmt.Sprint(c.Setting.Leverage),
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
					quantityPerOrder > 0,
				}

				logger.
					WithField("USDT BID SIZE", usdtBidSize).
					WithField("USDT ASK SIZE", usdtAskSize).
					WithField("BUSD BID SIZE", busdBidSize).
					WithField("BUSD ASK SIZE", busdAskSize).
					WithField("quantity", quantityPerOrder).
					WithField("total", totalQuantity).
					Info("check size and order quantity")

				if slices.Contains(rules, false) {
					break
				}

				// update var
				currentProgressBarTotal += 1

				// handle order
				if c.Setting.Reduce {
					v.Direction = !v.Direction

					if currentDirection == nil {
						logger.Info("can't find current direction")
						break
					}

					if *currentDirection {
						v.Direction = false
					} else {
						v.Direction = true
					}
				} else if currentDirection == nil {
					currentDirection = &v.Direction
				} else if *currentDirection != v.Direction {
					yield := (v.FundingRateGap * 365 * 3 * float64(c.Setting.Leverage)) / 2

					if !c.Setting.Reduce {
						if c.Setting.Threshold > yield {
							logger.Info("not reduce mode and threshold is greater then yield=", yield)
							continue
						}

						minutes := v.GetLeftMinutes("BUSD")
						if minutes >= c.Setting.Before {
							logger.
								WithField("before", c.Setting.Before).
								WithField("minutes", minutes).
								Info("left minutes is greater than config")
							continue
						}
					}

					c.EventPublisher <- models.EventMessage{Type: "reverse", Setting: c.Setting}

					if totalQuantity >= c.Setting.Total {
						step = 1
					} else {
						step = -1
					}

					currentDirection = &v.Direction

					// reset quantity
					quantityPerOrder = c.Setting.Quantity

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

				if v.Direction {
					binanceOrderBUSD.Side = "BUY"
					binanceOrderUSDT.Side = "SELL"
				} else {
					binanceOrderBUSD.Side = "SELL"
					binanceOrderUSDT.Side = "BUY"
				}

				if c.Setting.Reduce || fundingRateReverseMode {
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

					c.EventPublisher <- models.EventMessage{
						Type:    "place",
						Setting: c.Setting,
						Message: map[string]float64{
							"USDT_ASK_PRICE": usdtAsk,
							"BUSD_ASK_PRICE": busdAsk,
							"BUSD_BID_PRICE": busdBid,
							"USDT_BID_PRICE": usdtBid,
							"USDT_BID_SIZE":  usdtBidSize,
							"USDT_ASK_SIZE":  usdtAskSize,
							"BUSD_BID_SIZE":  busdBidSize,
							"BUSD_ASK_SIZE":  busdAskSize,
						},
					}

					batchOrders, _ := json.Marshal(orders)

					logger.Info(string(batchOrders))

					_, body, _ := c.MakeRequest(
						BINANCE_FAPI_BATCH_ORDERS,
						gorequest.POST,
						map[string]string{
							"batchOrders": string(batchOrders),
						},
					).End()
					logrus.Info(string(body))

					// update total
					value := decimal.
						NewFromFloat(quantityPerOrder).
						Mul(decimal.NewFromInt(int64(step)))

					// calculate totalQuantity
					totalQuantity, _ = decimal.
						NewFromFloat(totalQuantity).
						Sub(value).
						Float64()
				}

				// exit loop
				break
			}
		}

		time.Sleep(1 * time.Second)
	}
}

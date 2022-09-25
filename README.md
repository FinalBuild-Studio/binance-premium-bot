# binance-premium-bot

:warning: **This bot is for test only!** :warning:

It helps you create double-side contract order to generate funding fee without loss. It's possible to make incredible APR(>500%) on crypto market.

## How to build

```bash
go build
```

## Usage

```
Usage of ./binance-premium-bot:
  -apiKey string
    	binance api key
  -apiSecret string
    	binance api secret
  -difference float
    	BUSD & USDT difference (default 0.05)
  -leverage int
    	futures leverage (default 10)
  -quantity float
    	quantity per order
  -reduce
    	use reduce mode
  -symbol string
    	binance future symbol
  -total float
    	total quantity
```

## Example

```bash
go app.go -total 0.002 -quantity 0.001 -symbol BTC -apiKey XXX -apiSecret XXX
```

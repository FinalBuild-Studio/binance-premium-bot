# binance-premium-bot

:warning: **This bot is for test only!** :warning:

It helps you create double-side contract order to generate funding fee without loss. It's possible to make incredible APR(>500%) on crypto market.

## What you should prepare first

1. Binance API Key & Secret
2. golang 1.19 | docker

## How to build and use

### For executable binary

```bash
go build

./binance-premium-bot --help
```

### For docker

```bash
docker build . -t binance-premium-bot

docker run -it --rm binance-premium-bot --help
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

## Example (docker for example)

```bash
docker run -it --rm binance-premium-bot -total 0.002 -quantity 0.001 -symbol BTC -apiKey XXX -apiSecret XXX
```

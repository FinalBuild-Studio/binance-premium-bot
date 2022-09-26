# binance-premium-bot

:warning: **This bot is for test only!** :warning:

It helps you create double-side contract order to generate funding fee without loss(delta neutral strategy).
It's possible to make incredible APR(**>500%**) in crypto even in :bear: market.

<span color="red">Use it at your own risk!!!</span>

## What you should prepare first

1. Binance API Key & Secret, you can learn how to create [here](https://www.binance.com/en/amp/support/faq/360002502072)
2. golang 1.19 or [docker](https://www.docker.com/get-started/)

## How to build and use

### For executable binary

```bash
go build

./binance-premium-bot --help
```

### Build your own docker image

```bash
docker build . -t binance-premium-bot

docker run -it --rm binance-premium-bot --help
```

### Run docker image from github registry

```bash
docker run -ti --rm ghcr.io/capslock-studio/binance-premium-bot:main --help
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
docker run -it --rm ghcr.io/capslock-studio/binance-premium-bot:main -total 0.002 -quantity 0.001 -symbol BTC -apiKey XXX -apiSecret XXX
```

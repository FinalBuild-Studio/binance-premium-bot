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
docker run --pull always -ti --rm ghcr.io/capslock-studio/binance-premium-bot:main --help
```

### If you think building your own binary and docker image is too hard for you
Just check [this](https://github.com/CapsLock-Studio/binance-premium-bot/actions/workflows/go.yml) and download the latest binary

## Usage

```
Usage of ./binance-premium-bot:
  -apiKey string
    	binance api key
  -apiSecret string
    	binance api secret
  -arbitrage
    	use arbitrage mode
  -bidSide string
    	determine bid side in reduce mode
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
docker run --pull always -it --rm ghcr.io/capslock-studio/binance-premium-bot:main -total 0.002 -quantity 0.001 -symbol BTC -apiKey XXX -apiSecret XXX
```

## Auto arbitrage mode

Arbitrage mode is the mode can find chance to arbitrage between USDT & BUSD perpetual.
You have to set `-arbitrage` flag and let bot run automatically. :smile:

Here are some backtest result(real data)

|#|CURRENCY|POSITION|BUY|SELL|PROFIT|
|-|-|-|-|-|-|
|1|LDOBUSD|LONG|1.60700000|1.60500000|-0.12445550715619177%|
|1|LDOUSDT|SHORT|1.60300000|1.60900000|0.37290242386575545%|
|2|1000SHIBUSDT|SHORT|0.01114409|0.01119110|0.4200659452600737%|
|2|1000SHIBBUSD|LONG|0.01118100|0.01115740|-0.2110723548877592%|
|3|DOTBUSD|LONG|6.55900000|6.55468499|-0.06578762006403427%|
|3|DOTUSDT|SHORT|6.54700000|6.56429594|0.2634850737701558%|
|4|CVXBUSD|LONG|4.78300000|4.76850647|-0.3030217436755216%|
|4|CVXUSDT|SHORT|4.76271841|4.78700000|0.5072402339669978%|
|5|LDOBUSD|SHORT|1.62000000|1.61500000|-0.30959752321982137%|
|5|LDOUSDT|LONG|1.61356730|1.62200000|0.5226122269582547%|

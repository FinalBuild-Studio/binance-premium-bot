package models

type BinancePlaceOrder struct {
	Type       string `json:"type"`
	Symbol     string `json:"symbol"`
	Side       string `json:"side"`
	Quantity   string `json:"quantity"`
	ReduceOnly string `json:"reduceOnly"`
}

type BinanceOrder struct {
	Symbol       string `json:"symbol"`
	PositionSide string `json:"positionSide"`
	PositionAmt  string `json:"positionAmt"`
}

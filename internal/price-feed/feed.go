package pricefeed

import (
	"cryptosim/internal/models"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var endpoint = "wss://advanced-trade-ws.coinbase.com"

type PriceFeedRequest struct {
	Type       string   `json:"type"`
	ProductIds []string `json:"product_ids"`
	Channel    string   `json:"channel"`
}

type TickerResponse struct {
	Channel     string  `json:"channel"`
	ClientID    string  `json:"client_id"`
	Timestamp   string  `json:"timestamp"`
	SequenceNum int     `json:"sequence_num"`
	Events      []Event `json:"events"`
}

type Event struct {
	Type    string   `json:"type"`
	Tickers []Ticker `json:"tickers"`
}

type Ticker struct {
	Type               string `json:"type"`
	ProductID          string `json:"product_id"`
	Price              string `json:"price"`
	Volume24H          string `json:"volume_24_h"`
	Low24H             string `json:"low_24_h"`
	High24H            string `json:"high_24_h"`
	Low52W             string `json:"low_52_w"`
	High52W            string `json:"high_52_w"`
	PricePercentChg24H string `json:"price_percent_chg_24_h"`
	BestBid            string `json:"best_bid"`
	BestBidQuantity    string `json:"best_bid_quantity"`
	BestAsk            string `json:"best_ask"`
	BestAskQuantity    string `json:"best_ask_quantity"`
}

type PriceFeedService struct {
	wsConn   *websocket.Conn
	natsConn *NATSConn
	symbols  []string
}

func NewPriceFeedService(natsConn *NATSConn, symbols []string) *PriceFeedService {
	return &PriceFeedService{
		natsConn: natsConn,
		symbols:  symbols,
	}
}

func (priceFeedService *PriceFeedService) Start() error {
	conn, _, err := priceFeedService.dial(endpoint)
	if err != nil {
		return fmt.Errorf("failed to connect to Coinbase: %w", err)
	}

	priceFeedService.wsConn = conn
	log.Println("Connected to Coinbase WebSocket")

	var productIDs []models.ProductId

	for _, symbol := range priceFeedService.symbols {
		productIDs = append(productIDs, models.ProductId(symbol))
	}

	priceFeedService.subscribe(conn, "subscribe", productIDs, models.Ticker)
	log.Printf("Subscribed to ticker for %v", priceFeedService.symbols)

	go priceFeedService.startLivePricesPublishers()
	return nil
}

func (priceFeedService *PriceFeedService) dial(endpoint string) (*websocket.Conn, *http.Response, error) {
	conn, httpResponse, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		fmt.Println("ERROR: ", err)
	}

	return conn, httpResponse, err
}

func (priceFeedService *PriceFeedService) subscribe(conn *websocket.Conn, request_type models.WSRequestType, productIds []models.ProductId, channel models.Channel) {

	string_ids := make([]string, len(productIds))
	for i, id := range productIds {
		string_ids[i] = string(id)
	}

	subscribeMsg := PriceFeedRequest{
		Type:       string(request_type),
		ProductIds: string_ids,
		Channel:    string(channel),
	}
	err := conn.WriteJSON(subscribeMsg)

	if err != nil {
		fmt.Println("ERROR WHILE SUBSCRIBING: ", err)
	}
}

func (priceFeedService *PriceFeedService) ReconnectWithExponentialBackoff(numTries int) {

	priceFeedService.wsConn.Close()

	for count := 1; count <= numTries; count++ {
		backoffDelay := time.Duration(math.Pow(2, float64(count))) * time.Second
		fmt.Println("CONNECTION FAILED.. RECONNECT ATTEMPT ", count, " BACKOFF DELAY: ", backoffDelay)
		time.Sleep(backoffDelay)

		conn, _, err := priceFeedService.dial(endpoint)
		if err == nil {
			priceFeedService.wsConn = conn
			priceFeedService.subscribe(conn, "subscribe", []models.ProductId{models.BTC_USD, models.XRP_USD, models.ETH_USD}, models.Ticker)
		}
	}
	log.Fatalln("Could not reconnect, stopping the sim!")
}

func (priceFeedService *PriceFeedService) ReadMessages() <-chan models.PriceTick {
	priceTickChannel := make(chan models.PriceTick)

	go func() {
		defer close(priceTickChannel)

		for {
			var tickerResponse TickerResponse
			fmt.Println("TICKER REPONSE: ", tickerResponse)
			err := priceFeedService.wsConn.ReadJSON(&tickerResponse)
			if err != nil {
				fmt.Println("PRICE FEED JSON read error. RECONNECTING:", err)
				priceFeedService.ReconnectWithExponentialBackoff(3)
				continue
			}

			// Skip empty events or subscription confirmations
			if len(tickerResponse.Events) == 0 || len(tickerResponse.Events[0].Tickers) == 0 {
				continue
			}

			for _, ticker := range tickerResponse.Events[0].Tickers {
				bid, err := strconv.ParseFloat(ticker.BestBid, 64)
				if err != nil {
					fmt.Println("ERROR IN READING BID:", err)
					continue
				}

				ask, err := strconv.ParseFloat(ticker.BestAsk, 64)
				if err != nil {
					fmt.Println("ERROR IN READING ASK:", err)
					continue
				}

				priceTick := models.PriceTick{
					Symbol:    ticker.ProductID,
					Bid:       bid,
					Ask:       ask,
					Mid:       (bid + ask) / 2,
					Timestamp: time.Now().Unix(),
				}

				priceTickChannel <- priceTick
			}
		}
	}()

	return priceTickChannel
}

func (priceFeedService *PriceFeedService) startLivePricesPublishers() {
	readChannel := priceFeedService.ReadMessages()

	for priceTick := range readChannel {
		data, _ := json.Marshal(priceTick)

		if models.ProductId(priceTick.Symbol) == models.BTC_USD {
			priceFeedService.natsConn.nc.Publish(models.PriceBTCTopic, data)
		} else if models.ProductId(priceTick.Symbol) == models.ETH_USD {
			priceFeedService.natsConn.nc.Publish(models.PriceETHTopic, data)
		} else if models.ProductId(priceTick.Symbol) == models.XRP_USD {
			priceFeedService.natsConn.nc.Publish(models.PriceXRPTopic, data)
		}
	}
}

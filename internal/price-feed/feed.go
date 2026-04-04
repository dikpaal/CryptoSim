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

	"github.com/coinbase/cdp-sdk/go/auth"
	"github.com/gorilla/websocket"
)

var endpoint = "wss://advanced-trade-ws.coinbase.com"

type PriceFeedRequest struct {
	Type       string   `json:"type"`
	ProductIds []string `json:"product_ids"`
	Channel    string   `json:"channel"`
	JWT        string   `json:"jwt,omitempty"`
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
	wsConn    *websocket.Conn
	natsConn  *NATSConn
	symbols   []string
	keyName   string
	keySecret string
}

func NewPriceFeedService(natsConn *NATSConn, symbols []string, keyName, keySecret string) *PriceFeedService {
	return &PriceFeedService{
		natsConn:  natsConn,
		symbols:   symbols,
		keyName:   keyName,
		keySecret: keySecret,
	}
}

func (pfs *PriceFeedService) generateJWT() string {
	if pfs.keyName == "" || pfs.keySecret == "" {
		return ""
	}
	jwt, err := auth.GenerateJWT(auth.JwtOptions{
		KeyID:     pfs.keyName,
		KeySecret: pfs.keySecret,
		ExpiresIn: 120,
	})
	if err != nil {
		log.Printf("JWT generation failed: %v", err)
		return ""
	}
	return jwt
}

func (pfs *PriceFeedService) Start() error {
	conn, _, err := pfs.dial(endpoint)
	if err != nil {
		return fmt.Errorf("failed to connect to Coinbase: %w", err)
	}

	pfs.wsConn = conn
	log.Println("Connected to Coinbase WebSocket")

	var productIDs []models.ProductId
	for _, symbol := range pfs.symbols {
		productIDs = append(productIDs, models.ProductId(symbol))
	}

	pfs.subscribe(conn, "subscribe", productIDs, models.Ticker, pfs.generateJWT())
	log.Printf("Subscribed to ticker for %v", pfs.symbols)

	// Refresh JWT and resubscribe before the 2-minute expiry.
	if pfs.keyName != "" {
		go func() {
			ticker := time.NewTicker(110 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				pfs.subscribe(pfs.wsConn, "subscribe", productIDs, models.Ticker, pfs.generateJWT())
				log.Println("JWT refreshed, resubscribed to ticker")
			}
		}()
	}

	go pfs.startLivePricesPublishers()
	return nil
}

func (pfs *PriceFeedService) dial(endpoint string) (*websocket.Conn, *http.Response, error) {
	conn, httpResponse, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		fmt.Println("ERROR:", err)
	}
	return conn, httpResponse, err
}

func (pfs *PriceFeedService) subscribe(conn *websocket.Conn, requestType models.WSRequestType, productIds []models.ProductId, channel models.Channel, jwt string) {
	stringIDs := make([]string, len(productIds))
	for i, id := range productIds {
		stringIDs[i] = string(id)
	}

	msg := PriceFeedRequest{
		Type:       string(requestType),
		ProductIds: stringIDs,
		Channel:    string(channel),
		JWT:        jwt,
	}
	if err := conn.WriteJSON(msg); err != nil {
		fmt.Println("ERROR WHILE SUBSCRIBING:", err)
	}
}

func (pfs *PriceFeedService) ReconnectWithExponentialBackoff(numTries int) {
	pfs.wsConn.Close()

	for count := 1; count <= numTries; count++ {
		backoffDelay := time.Duration(math.Pow(2, float64(count))) * time.Second
		fmt.Println("CONNECTION FAILED.. RECONNECT ATTEMPT", count, "BACKOFF DELAY:", backoffDelay)
		time.Sleep(backoffDelay)

		conn, _, err := pfs.dial(endpoint)
		if err == nil {
			pfs.wsConn = conn
			pfs.subscribe(conn, "subscribe", []models.ProductId{models.BTC_USD, models.XRP_USD, models.ETH_USD}, models.Ticker, pfs.generateJWT())
			return
		}
	}
	log.Fatalln("Could not reconnect, stopping the sim!")
}

func (pfs *PriceFeedService) ReadMessages() <-chan models.PriceTick {
	priceTickChannel := make(chan models.PriceTick)

	go func() {
		defer close(priceTickChannel)

		for {
			var tickerResponse TickerResponse
			err := pfs.wsConn.ReadJSON(&tickerResponse)
			if err != nil {
				fmt.Println("PRICE FEED JSON read error. RECONNECTING:", err)
				pfs.ReconnectWithExponentialBackoff(3)
				continue
			}

			if len(tickerResponse.Events) == 0 || len(tickerResponse.Events[0].Tickers) == 0 {
				continue
			}

			for _, ticker := range tickerResponse.Events[0].Tickers {
				bid, err := strconv.ParseFloat(ticker.BestBid, 64)
				if err != nil {
					continue
				}
				ask, err := strconv.ParseFloat(ticker.BestAsk, 64)
				if err != nil {
					continue
				}

				priceTickChannel <- models.PriceTick{
					Symbol:    ticker.ProductID,
					Bid:       bid,
					Ask:       ask,
					Mid:       (bid + ask) / 2,
					Timestamp: time.Now().Unix(),
				}
			}
		}
	}()

	return priceTickChannel
}

func (pfs *PriceFeedService) startLivePricesPublishers() {
	for priceTick := range pfs.ReadMessages() {
		data, _ := json.Marshal(priceTick)
		switch models.ProductId(priceTick.Symbol) {
		case models.BTC_USD:
			pfs.natsConn.nc.Publish(models.PriceBTCTopic, data)
		case models.ETH_USD:
			pfs.natsConn.nc.Publish(models.PriceETHTopic, data)
		case models.XRP_USD:
			pfs.natsConn.nc.Publish(models.PriceXRPTopic, data)
		}
	}
}

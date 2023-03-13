package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"os"

	"github.com/joho/godotenv"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
)

var client *liteclient.ConnectionPool
var api *ton.APIClient
var w *wallet.Wallet

func sendTransactions(responseWriter http.ResponseWriter, req *http.Request) {
	// read query param "send_mode"
	sendModeString := req.URL.Query().Get("send_mode")

	// print send mode
	log.Println("send mode:", sendModeString)


	// parse send mode as uint8
	sendMode, err := strconv.ParseUint(sendModeString, 10, 8)
	if err != nil {
		// return json error
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(responseWriter).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	// print
	

	commentText := req.URL.Query().Get("comment")

	log.Println("comment:", commentText)

	var txs map[string]string
	// print request body
	log.Println("request body:", req.Body)
	err = json.NewDecoder(req.Body).Decode(&txs)


	log.Println("txs:", txs)

	if err != nil {
		log.Println("json decode err:", err.Error())

		// return json error
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(responseWriter).Encode(map[string]string{
			"error": err.Error(),
		})

		return
	}

	block, err := api.CurrentMasterchainInfo(context.Background())
	if err != nil {
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(responseWriter).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	balance, err := w.GetBalance(context.Background(), block)
	if err != nil {
		// return json error
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(responseWriter).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}


	// sum all transaction amounts and write to totalAmount
	var totalAmount float64
	for _, amtStr := range txs {

		// create float from string
		amtFloat, err := strconv.ParseFloat(amtStr, 64)

		if err != nil {
			// return json error
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(responseWriter).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}
		totalAmount += amtFloat
	}

	// make from float to uint64
	totalAmountUint64 := uint64(totalAmount * 1e9)


	
	if balance.NanoTON().Uint64() >= totalAmountUint64 {
		comment, err := wallet.CreateCommentCell(commentText)
		if err != nil {
			log.Fatalln("CreateComment err:", err.Error())
			return
		}

		var messages []*wallet.Message
		// generate message for each destination, in single transaction can be sent up to 254 messages
		for addrStr, amtStr := range txs {
			messages = append(messages, &wallet.Message{
				Mode: uint8(sendMode), // pay fee separately
				InternalMessage: &tlb.InternalMessage{
					Bounce:  false, // force send, even to uninitialized wallets
					DstAddr: address.MustParseAddr(addrStr),
					Amount:  tlb.MustFromTON(amtStr),
					Body:  comment,
				},
			})
		}

		log.Println("sending transaction and waiting for confirmation...")

		// send transaction that contains all our messages, and wait for confirmation
		txHash, err := w.SendManyWaitTxHash(context.Background(), messages)
		if err != nil {
			// return json error
			responseWriter.Header().Set("Content-Type", "application/json")
			responseWriter.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(responseWriter).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		log.Println("transaction sent, hash:", base64.StdEncoding.EncodeToString(txHash))
		log.Println("explorer link: https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash))
		// return json with transaction hash and explorer link
		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusOK)
		json.NewEncoder(responseWriter).Encode(map[string]string{
			"txHash": base64.StdEncoding.EncodeToString(txHash),
			"link":   "https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash),
		})
		return
	}

	// answer with json error not enough balance
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": "not enough balance",
	})
}

func main() {
	client = liteclient.NewConnectionPool()

	err := client.AddConnection(context.Background(), "135.181.140.212:13206", "K0t3+IWLOXHYMvMcrGZDPs+pn58a17LFbnXoQkKc2xw=")
	if err != nil {
		log.Fatalln("connection err: ", err.Error())
		return
	}

	api = ton.NewAPIClient(client)


	// read seed words from env
	err = godotenv.Load()
    if err != nil {
        log.Fatalln("Error loading .env file", err.Error())
    }
	seedWords := os.Getenv("SEED_WORDS")

	words := strings.Split(seedWords, " ")

	// initialize high-load wallet
	w, err = wallet.FromSeed(api, words, wallet.HighloadV2R2)
	if err != nil {
		log.Fatalln("FromSeed err:", err.Error())
		return
	}

	log.Println("wallet address:", w.Address())

	http.HandleFunc("/sendTransactions", sendTransactions)

	http.ListenAndServe(":8888", nil)
	log.Println("Server started on port 8888")
}

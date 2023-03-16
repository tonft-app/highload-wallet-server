package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

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


// get responseWriter and 

func respondError(responseWriter http.ResponseWriter, err error) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": err.Error(),
	})
}

func sendTransactions(responseWriter http.ResponseWriter, req *http.Request) {
	sendModeString := req.URL.Query().Get("send_mode")
	log.Println("Send mode:", sendModeString)

	sendMode, err := strconv.ParseUint(sendModeString, 10, 8)

	if err != nil {
		respondError(responseWriter, err)
		return
	}

	commentText := req.URL.Query().Get("comment")

	log.Println("comment:", commentText)

	var txs map[string]string

	err = json.NewDecoder(req.Body).Decode(&txs)

	log.Println("Transactions:", txs)

	if err != nil {
		log.Println("Json decode err:", err.Error())
		respondError(responseWriter, err)
		return
	}

	block, err := api.CurrentMasterchainInfo(context.Background())
	if err != nil {
		respondError(responseWriter, err)
		return
	}

	balance, err := w.GetBalance(context.Background(), block)
	if err != nil {
		respondError(responseWriter, err)
		return
	}

	var totalAmount float64
	for _, amtStr := range txs {
		amtFloat, err := strconv.ParseFloat(amtStr, 64)

		if err != nil {
			respondError(responseWriter, err)
			return
		}

		totalAmount += amtFloat
	}

	totalAmountUint64 := uint64(totalAmount * 1e9)

	if balance.NanoTON().Uint64() >= totalAmountUint64 {
		comment, err := wallet.CreateCommentCell(commentText)
		if err != nil {
			log.Fatalln("CreateComment err:", err.Error())
			return
		}

		var messages []*wallet.Message
		for addrStr, amtStr := range txs {
			messages = append(messages, &wallet.Message{
				Mode: uint8(sendMode),
				InternalMessage: &tlb.InternalMessage{
					Bounce:  false, 
					DstAddr: address.MustParseAddr(addrStr),
					Amount:  tlb.MustFromTON(amtStr),
					Body:  comment,
				},
			})
		}

		log.Println("Sending transaction and waiting for confirmation...")

		txHash, err := w.SendManyWaitTxHash(context.Background(), messages)
		if err != nil {
			respondError(responseWriter, err)
			return
		}

		log.Println("Transaction sent, hash:", base64.StdEncoding.EncodeToString(txHash))
		log.Println("Explorer link: https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash))

		responseWriter.Header().Set("Content-Type", "application/json")
		responseWriter.WriteHeader(http.StatusOK)
		json.NewEncoder(responseWriter).Encode(map[string]string{
			"txHash": base64.StdEncoding.EncodeToString(txHash),
			"link":   "https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash),
		})
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": "Not enough balance",
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

	err = godotenv.Load()
    if err != nil {
        log.Fatalln("Error loading .env file", err.Error())
    }

	seedWords := os.Getenv("SEED_PHRASE")
	var words []string;
	
	if seedWords == "" {
		log.Fatalln("SEED_PHRASE env is empty")
		words = wallet.NewSeed()
		log.Println("Generated seed words:", strings.Join(words, " "))
	} else{
		words = strings.Split(seedWords, " ")
	}

	w, err = wallet.FromSeed(api, words, wallet.HighloadV2R2)
	if err != nil {
		log.Fatalln("FromSeed err:", err.Error())
		return
	}

	log.Println("Wallet address:", w.Address())
	http.HandleFunc("/sendTransactions", sendTransactions)
	http.ListenAndServe(":8888", nil)
}
